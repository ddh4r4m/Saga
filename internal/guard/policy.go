package guard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/ddh4r4m/saga/internal/canon"
)

// PolicySchema is the schema id of .saga/policy.toml (guard-spec 2.5).
const PolicySchema = "saga.guard.policy/1"

// Policy is the [guard] policy of guard-spec section 2.5. Only the tables
// command validation reads are typed here; snapshot, mask, deps and mcp
// tables are accepted and carried through as opaque data so a policy file
// written for the full layer validates today.
type Policy struct {
	Schema string `toml:"schema" json:"schema"`
	Shell  string `toml:"shell" json:"shell"`

	Net         NetPolicy         `toml:"net" json:"net"`
	Scope       ScopePolicy       `toml:"scope" json:"scope"`
	Allow       AllowPolicy       `toml:"allow" json:"allow"`
	Deny        DenyPolicy        `toml:"deny" json:"deny"`
	Ask         AskPolicy         `toml:"ask" json:"ask"`
	Git         GitPolicy         `toml:"git" json:"git"`
	Credentials CredentialsPolicy `toml:"credentials" json:"credentials"`

	// Tables owned by later guard parts, accepted verbatim.
	Snapshot map[string]any `toml:"snapshot" json:"snapshot,omitempty"`
	Mask     map[string]any `toml:"mask" json:"mask,omitempty"`
	Deps     map[string]any `toml:"deps" json:"deps,omitempty"`
	MCP      map[string]any `toml:"mcp" json:"mcp,omitempty"`
}

// NetPolicy is [net].
type NetPolicy struct {
	Fetch *bool `toml:"fetch" json:"fetch"`
}

// ScopePolicy is [scope].
type ScopePolicy struct {
	Extra            []string `toml:"extra" json:"extra"`
	IgnoredIsInScope *bool    `toml:"ignored_is_in_scope" json:"ignored_is_in_scope"`
}

// AllowPolicy is [allow]: resolved-argv patterns, first token literal,
// the rest glob.
type AllowPolicy struct {
	Read             []string `toml:"read" json:"read"`
	MutateInScope    []string `toml:"mutate_in_scope" json:"mutate_in_scope"`
	MutateOutOfScope []string `toml:"mutate_out_of_scope" json:"mutate_out_of_scope"`
}

// DenyPolicy is [deny]: user additions to the hard-deny list.
type DenyPolicy struct {
	Extra []string `toml:"extra" json:"extra"`
}

// AskPolicy is [ask].
type AskPolicy struct {
	InterpreterExecUntracked *bool `toml:"interpreter_exec_untracked" json:"interpreter_exec_untracked"`
	Unresolvable             *bool `toml:"unresolvable" json:"unresolvable"`
	Sudo                     *bool `toml:"sudo" json:"sudo"`
}

// GitPolicy is [git].
type GitPolicy struct {
	Protected []string `toml:"protected" json:"protected"`
}

// CredentialsPolicy is [credentials].
type CredentialsPolicy struct {
	PathsExtra []string `toml:"paths_extra" json:"paths_extra"`
	AllowRead  []string `toml:"allow_read" json:"allow_read"`
}

// DefaultCredentialPaths is the credential_paths default of guard-spec
// section 2.4.1 (Unix rows; the Windows rows are M1 Windows work).
var DefaultCredentialPaths = []string{
	"~/.ssh/**", "~/.aws/**", "~/.config/gcloud/**", "~/.azure/**", "~/.kube/config",
	"~/.docker/config.json", "~/.netrc", "~/.npmrc", "~/.pypirc", "~/.gem/credentials",
	"~/.gitconfig", "~/.claude.json", "~/.claude/**/*.json", "~/.codex/auth.json",
	"~/.gemini/.env", "~/.gemini/oauth_creds.json",
	"**/.env", "**/.env.*", "**/*.pem", "**/*.p12", "**/*.key", "**/id_*", "**/secrets.*",
	"**/credentials*", "/proc/*/environ",
}

// DefaultProtectedBranches is the [git] protected default.
var DefaultProtectedBranches = []string{"main", "master", "develop", "release/*"}

func boolp(b bool) *bool { return &b }

// DefaultPolicy returns the documented defaults: reads and the common
// in-scope build and test commands run without a prompt, network fetches
// are allowed, every other class asks.
func DefaultPolicy() *Policy {
	return &Policy{
		Schema: PolicySchema,
		Shell:  "auto",
		Net:    NetPolicy{Fetch: boolp(true)},
		Scope:  ScopePolicy{Extra: []string{}, IgnoredIsInScope: boolp(true)},
		Allow: AllowPolicy{
			Read: []string{"git *", "ls *", "cat *", "rg *", "pytest --collect-only *"},
			MutateInScope: []string{
				"npm test", "npm run build", "npm ci", "npm install", "yarn *", "pnpm *",
				"pytest *", "cargo test *", "cargo build *", "swift test *", "swift build *",
				"go test *", "go build *", "go vet *", "make test", "make build", "make",
			},
			MutateOutOfScope: []string{"pip install -r requirements.txt", "git push origin HEAD"},
		},
		Deny: DenyPolicy{Extra: []string{}},
		Ask: AskPolicy{
			InterpreterExecUntracked: boolp(true), Unresolvable: boolp(true), Sudo: boolp(true),
		},
		Git:         GitPolicy{Protected: append([]string(nil), DefaultProtectedBranches...)},
		Credentials: CredentialsPolicy{PathsExtra: []string{}, AllowRead: []string{}},
	}
}

// NetFetch reports the effective net.fetch.
func (p *Policy) NetFetch() bool { return p.Net.Fetch == nil || *p.Net.Fetch }

// AskUnresolvable reports the effective ask.unresolvable.
func (p *Policy) AskUnresolvable() bool { return p.Ask.Unresolvable == nil || *p.Ask.Unresolvable }

// AskSudo reports the effective ask.sudo.
func (p *Policy) AskSudo() bool { return p.Ask.Sudo == nil || *p.Ask.Sudo }

// AskInterpreterUntracked reports the effective ask.interpreter_exec_untracked.
func (p *Policy) AskInterpreterUntracked() bool {
	return p.Ask.InterpreterExecUntracked == nil || *p.Ask.InterpreterExecUntracked
}

// CredentialPaths returns the defaults plus paths_extra.
func (p *Policy) CredentialPaths() []string {
	out := append([]string(nil), DefaultCredentialPaths...)
	return append(out, p.Credentials.PathsExtra...)
}

// Hash is the sha256 of the canonical JSON form of the effective policy.
func (p *Policy) Hash() string {
	h, err := canon.SHA256JSON(p)
	if err != nil {
		return canon.EmptyHash
	}
	return h
}

// Validate checks the schema id and the pattern syntax.
func (p *Policy) Validate() error {
	if p.Schema != "" && p.Schema != PolicySchema {
		return fmt.Errorf("schema %q, want %s", p.Schema, PolicySchema)
	}
	switch p.Shell {
	case "", "auto", "bash", "zsh", "sh", "pwsh", "cmd":
	default:
		return fmt.Errorf("shell %q: want auto, bash, zsh, sh, pwsh or cmd", p.Shell)
	}
	for name, list := range map[string][]string{
		"allow.read": p.Allow.Read, "allow.mutate_in_scope": p.Allow.MutateInScope,
		"allow.mutate_out_of_scope": p.Allow.MutateOutOfScope, "deny.extra": p.Deny.Extra,
	} {
		for _, pat := range list {
			if strings.TrimSpace(pat) == "" {
				return fmt.Errorf("%s: empty pattern", name)
			}
			if strings.ContainsAny(strings.Fields(pat)[0], "*?[") {
				return fmt.Errorf("%s: %q: the first token must be a literal command name", name, pat)
			}
		}
	}
	return nil
}

// ParsePolicy decodes a policy file. Unknown keys are an error (contracts
// section 11: an unknown field is a validation failure).
func ParsePolicy(raw []byte) (*Policy, error) {
	var p Policy
	md, err := toml.Decode(string(raw), &p)
	if err != nil {
		return nil, err
	}
	if und := md.Undecoded(); len(und) > 0 {
		keys := make([]string, 0, len(und))
		for _, k := range und {
			keys = append(keys, k.String())
		}
		sort.Strings(keys)
		return nil, fmt.Errorf("unknown key %s", strings.Join(keys, ", "))
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Loosening is one field of a repo policy that would widen what the
// agent may do. A repo policy may only tighten (guard-spec 2.5).
type Loosening struct {
	Field  string
	Detail string
}

func (l Loosening) String() string { return l.Field + ": " + l.Detail }

// Loosenings lists every field of repo that is not a pure tightening of
// base. The fixed tables are [allow], [scope], [net] (only false is
// accepted), [ask] (only true), [credentials].allow_read and shell.
func Loosenings(repo *Policy) []Loosening {
	var out []Loosening
	add := func(f, d string) { out = append(out, Loosening{Field: f, Detail: d}) }
	if repo.Shell != "" && repo.Shell != "auto" {
		add("shell", "a repo policy cannot pick the shell")
	}
	if len(repo.Allow.Read)+len(repo.Allow.MutateInScope)+len(repo.Allow.MutateOutOfScope) > 0 {
		add("allow", "a repo policy cannot add allow rules")
	}
	if len(repo.Scope.Extra) > 0 {
		add("scope.extra", "a repo policy cannot widen the writable scope")
	}
	if repo.Scope.IgnoredIsInScope != nil && *repo.Scope.IgnoredIsInScope {
		add("scope.ignored_is_in_scope", "a repo policy may only set false")
	}
	if repo.Net.Fetch != nil && *repo.Net.Fetch {
		add("net.fetch", "a repo policy may only set false")
	}
	for name, v := range map[string]*bool{
		"ask.interpreter_exec_untracked": repo.Ask.InterpreterExecUntracked,
		"ask.unresolvable":               repo.Ask.Unresolvable,
		"ask.sudo":                       repo.Ask.Sudo,
	} {
		if v != nil && !*v {
			add(name, "a repo policy may only set true")
		}
	}
	if len(repo.Credentials.AllowRead) > 0 {
		add("credentials.allow_read", "a repo policy cannot permit credential reads")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// Merge applies a tightening-only repo policy on top of base and returns
// the result. Callers check Loosenings first; Merge ignores loosening
// fields.
func Merge(base, repo *Policy) *Policy {
	out := *base
	out.Deny.Extra = appendUnique(out.Deny.Extra, repo.Deny.Extra)
	out.Git.Protected = appendUnique(out.Git.Protected, repo.Git.Protected)
	out.Credentials.PathsExtra = appendUnique(out.Credentials.PathsExtra, repo.Credentials.PathsExtra)
	if repo.Net.Fetch != nil && !*repo.Net.Fetch {
		out.Net.Fetch = boolp(false)
	}
	if repo.Scope.IgnoredIsInScope != nil && !*repo.Scope.IgnoredIsInScope {
		out.Scope.IgnoredIsInScope = boolp(false)
	}
	for _, pair := range []struct {
		dst **bool
		src *bool
	}{
		{&out.Ask.InterpreterExecUntracked, repo.Ask.InterpreterExecUntracked},
		{&out.Ask.Unresolvable, repo.Ask.Unresolvable},
		{&out.Ask.Sudo, repo.Ask.Sudo},
	} {
		if pair.src != nil && *pair.src {
			*pair.dst = boolp(true)
		}
	}
	return &out
}

// Override applies the user's global policy: every field it sets replaces
// the default (the user owns the layer).
func Override(base, user *Policy) *Policy {
	out := *base
	if user.Shell != "" {
		out.Shell = user.Shell
	}
	if user.Net.Fetch != nil {
		out.Net.Fetch = user.Net.Fetch
	}
	if user.Scope.Extra != nil {
		out.Scope.Extra = user.Scope.Extra
	}
	if user.Scope.IgnoredIsInScope != nil {
		out.Scope.IgnoredIsInScope = user.Scope.IgnoredIsInScope
	}
	if user.Allow.Read != nil {
		out.Allow.Read = user.Allow.Read
	}
	if user.Allow.MutateInScope != nil {
		out.Allow.MutateInScope = user.Allow.MutateInScope
	}
	if user.Allow.MutateOutOfScope != nil {
		out.Allow.MutateOutOfScope = user.Allow.MutateOutOfScope
	}
	out.Deny.Extra = appendUnique(out.Deny.Extra, user.Deny.Extra)
	if user.Ask.InterpreterExecUntracked != nil {
		out.Ask.InterpreterExecUntracked = user.Ask.InterpreterExecUntracked
	}
	if user.Ask.Unresolvable != nil {
		out.Ask.Unresolvable = user.Ask.Unresolvable
	}
	if user.Ask.Sudo != nil {
		out.Ask.Sudo = user.Ask.Sudo
	}
	if user.Git.Protected != nil {
		out.Git.Protected = user.Git.Protected
	}
	out.Credentials.PathsExtra = appendUnique(out.Credentials.PathsExtra, user.Credentials.PathsExtra)
	if user.Credentials.AllowRead != nil {
		out.Credentials.AllowRead = user.Credentials.AllowRead
	}
	return &out
}

func appendUnique(dst, src []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(dst)+len(src))
	for _, list := range [][]string{dst, src} {
		for _, s := range list {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// PolicyFiles are the candidate repo policy paths, first match wins.
// guard-spec 2.5 and contracts section 2 name .saga/policy.toml; the
// guard.toml alias is accepted for the M1 task wording.
var PolicyFiles = []string{".saga/policy.toml", ".saga/guard.toml"}

// SagaHome returns the per-user directory (~/.saga, or $SAGA_HOME).
func SagaHome(env map[string]string) string {
	if h := env["SAGA_HOME"]; h != "" {
		return h
	}
	if h := os.Getenv("SAGA_HOME"); h != "" {
		return h
	}
	home := env["HOME"]
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".saga")
}

// TrustPath is the file recording the trusted hash of a repository's
// policy: <saga home>/approved/guard/<sha256(root)[:16]>.
func TrustPath(env map[string]string, root string) string {
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(SagaHome(env), "approved", "guard", hex.EncodeToString(sum[:])[:16])
}

// Trust records hash as the trusted repo policy hash for root. It is the
// human side of `saga guard policy trust` and is never reachable from
// an agent shell (contracts section 8).
func Trust(env map[string]string, root, hash string) error {
	p := TrustPath(env, root)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(hash+"\n"), 0o600)
}

// LoadedPolicy is the effective policy with how it was assembled.
type LoadedPolicy struct {
	Policy *Policy
	// UserFile is the global policy path when one was read.
	UserFile string
	// RepoFile is the repo policy path when one exists.
	RepoFile string
	// RepoHash is the sha256 of the repo policy file bytes.
	RepoHash string
	// RepoTrusted reports whether RepoHash matches the trust record.
	RepoTrusted bool
	// RepoApplied reports whether the repo policy was merged.
	RepoApplied bool
	// Loosenings lists the repo policy fields that were refused.
	Loosenings []Loosening
	// Notes are human-readable remarks for the decision record.
	Notes []string
}

// LoadPolicy assembles the effective policy for a repository root:
// defaults, then the user's ~/.saga/policy.toml, then the repo policy
// when it is trusted and tightens only. A malformed file is an error the
// CLI maps to exit 2; a trusted repo policy that loosens is also an error
// (contracts section 4: "repo policy touching a fixed table").
func LoadPolicy(env map[string]string, root string) (*LoadedPolicy, error) {
	lp := &LoadedPolicy{Policy: DefaultPolicy()}
	userFile := filepath.Join(SagaHome(env), "policy.toml")
	if raw, err := os.ReadFile(userFile); err == nil {
		up, err := ParsePolicy(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", userFile, err)
		}
		lp.Policy = Override(lp.Policy, up)
		lp.UserFile = userFile
	}
	for _, rel := range PolicyFiles {
		p := filepath.Join(root, filepath.FromSlash(rel))
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lp.RepoFile = p
		lp.RepoHash = canon.SHA256(raw)
		rp, err := ParsePolicy(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if want, err := os.ReadFile(TrustPath(env, root)); err == nil && strings.TrimSpace(string(want)) == lp.RepoHash {
			lp.RepoTrusted = true
		}
		lp.Loosenings = Loosenings(rp)
		if !lp.RepoTrusted {
			lp.Notes = append(lp.Notes, "repo policy "+rel+" ignored: hash "+lp.RepoHash+" not trusted (saga guard policy trust)")
			break
		}
		if len(lp.Loosenings) > 0 {
			return lp, fmt.Errorf("%s: repo policy touches a fixed table: %s", p, lp.Loosenings[0])
		}
		lp.Policy = Merge(lp.Policy, rp)
		lp.RepoApplied = true
		break
	}
	return lp, nil
}
