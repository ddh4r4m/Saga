package gate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/snapshot"
	"github.com/ddh4r4m/saga/internal/store"
	"github.com/ddh4r4m/saga/internal/trace"
)

// Token ceilings of gate-spec section 9.
const (
	CeilingPreTool = 150
	CeilingPost    = 200
	CeilingStop    = 400
	SessionShare   = 1000
)

// ProtectedPaths are ledger paths the agent's editor tools may not touch:
// the checker writes them (contracts section 8; gate-spec section 2.1).
var ProtectedPaths = []string{".saga/evidence/", ".saga/red/", ".saga/request.md", ".saga/observed/"}

// forbiddenSubstrings are strings that name the approval store or its
// override; a shell command carrying one is denied outright (contracts
// section 8: the store is never the agent's to write or redirect).
var forbiddenSubstrings = []string{ApprovalEnv, ".saga/approved", ".saga/observed"}

// reApproveFlag matches the --approve and --ci flags in every spelling
// the flag package accepts.
var reApproveFlag = regexp.MustCompile(`^--?(approve|ci)(=.*)?$`)

// ForbiddenCommand classifies a shell command against gate's rows of the
// shared agent-forbidden list (contracts section 8): `saga gate approve`,
// `saga gate attest`, `saga gate check --approve`, `saga gate reverify
// --ci`, and any mention of the approval store. It tokenises on
// whitespace and command separators, strips quotes, and matches the
// `saga` word by basename, so flag order, `-approve`, quoting and a path
// to the binary do not evade it. A variable or an alias that expands to
// `saga` does: post-expansion classification is guard's (M1). It returns
// the row matched, or "".
func ForbiddenCommand(cmd string) string {
	for _, s := range forbiddenSubstrings {
		if strings.Contains(cmd, s) {
			return s
		}
	}
	// Split into simple commands on the shell operators, then into words.
	for _, seg := range regexp.MustCompile(`\|\||&&|[;|&\n]`).Split(cmd, -1) {
		words := strings.Fields(seg)
		for i := range words {
			words[i] = strings.Trim(words[i], `"'`+"`")
		}
		for i := 0; i+2 < len(words); i++ {
			if filepath.Base(words[i]) != "saga" || words[i+1] != "gate" {
				continue
			}
			switch words[i+2] {
			case "approve", "attest":
				return "saga gate " + words[i+2]
			case "check", "reverify":
				for _, w := range words[i+3:] {
					if reApproveFlag.MatchString(w) {
						return "saga gate " + words[i+2] + " " + w
					}
				}
			}
		}
	}
	return ""
}

// Layer is gate's step in the composed hook (section 6): translation of
// `status` and `guard-diff` into the harness envelope, never executing a
// CHECK: line.
type Layer struct {
	Store  *store.Store
	Stderr func(string)

	lastStop *StopOutcome
}

// StopOutcome is what gate's Stop step loaded and decided, kept for the
// claim step of the same entry (trace-spec section 5.9) so one Stop
// costs one write-tree. Decision is inactive, invalid, deleted, allow,
// block or release.
type StopOutcome struct {
	Loaded   *Loaded
	Report   *Report
	Decision string
	Blocks   int
}

// LastStop returns the outcome of the Stop step this process ran, or nil.
func (l *Layer) LastStop() *StopOutcome { return l.lastStop }

// Name implements hookio.Layer.
func (l *Layer) Name() string { return "gate" }

func (l *Layer) note(format string, args ...any) {
	if l.Stderr != nil {
		l.Stderr(fmt.Sprintf(format, args...))
	}
}

// Run implements hookio.Layer.
func (l *Layer) Run(ctx context.Context, in *hookio.Input) (*hookio.Output, error) {
	out := hookio.Allow("gate")
	if l.Store == nil || !l.Store.Exists() {
		return out, nil
	}
	l.session(in)
	switch in.Event {
	case hookio.EventPreToolUse:
		return l.preTool(in, out)
	case hookio.EventPostToolUse:
		return l.postTool(in, out)
	case hookio.EventStop:
		return l.stop(in, out)
	}
	return out, nil
}

// session records what the human-only commands consult: the approval
// store this (harness-inherited) environment names, and the shell tool
// call in flight between PreToolUse and PostToolUse. Turn boundaries
// clear a stale in-flight marker.
func (l *Layer) session(in *hookio.Input) {
	if in.SessionID == "" {
		return
	}
	g, err := ReadGateSession(l.Store, in.SessionID)
	if err != nil {
		return
	}
	dir := os.Getenv(ApprovalEnv)
	if dir != "" {
		if c, err := filepath.EvalSymlinks(dir); err == nil {
			dir = c
		}
	}
	g.ApprovalDir, g.ApprovalDirSeen = dir, true
	shell := in.ToolName == "Bash" || in.ToolName == "PowerShell"
	switch in.Event {
	case hookio.EventPreToolUse:
		if shell {
			g.ToolInFlight, g.ToolName = firstNonEmpty(in.ToolUseID, "unknown"), in.ToolName
		}
	case hookio.EventPostToolUse, hookio.EventPostToolUseFailure:
		if shell || g.ToolInFlight == in.ToolUseID {
			g.ToolInFlight, g.ToolName = "", ""
		}
	case hookio.EventStop, hookio.EventUserPromptSubmit, hookio.EventSessionStart, hookio.EventSessionEnd:
		g.ToolInFlight, g.ToolName = "", ""
	}
	if err := WriteGateSession(l.Store, g); err != nil {
		l.note("saga gate: session: %v", err)
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (l *Layer) preTool(in *hookio.Input, out *hookio.Output) (*hookio.Output, error) {
	switch in.ToolName {
	case "Bash", "PowerShell":
		var ti struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(in.ToolInput, &ti)
		if f := ForbiddenCommand(ti.Command); f != "" {
			out.Decision, out.Reason, out.Exit = hookio.DecisionDeny, "saga gate: "+canon.CleanText(f)+" is a human act; ask the user to run it", int(cli.ExitRefusal)
			return out, nil
		}
	case "Edit", "Write", "NotebookEdit", "MultiEdit":
		var ti struct {
			FilePath string `json:"file_path"`
		}
		_ = json.Unmarshal(in.ToolInput, &ti)
		if why := l.protectedWrite(ti.FilePath); why != "" {
			out.Decision, out.Reason, out.Exit = hookio.DecisionDeny, "saga gate: "+why, int(cli.ExitRefusal)
			return out, nil
		}
		ld, err := LoadLite(l.Store.Root, l.Store)
		if err != nil || ld.Contract == nil {
			return out, nil
		}
		if f := PredictScope(GuardInput{Root: ld.Root, Contract: ld.Contract, Config: ld.Config}, ti.FilePath); f != nil {
			msg := fmt.Sprintf("saga gate: G-SCOPE %s %s", clip(f.Path, 200), f.Rule)
			if f.Detail != "" {
				msg += " (" + clip(f.Detail, 200) + ")"
			}
			out.Decision, out.Reason, out.Exit = hookio.DecisionDeny, l.cap(in, msg, CeilingPreTool), int(cli.ExitRefusal)
		}
	}
	return out, nil
}

// protectedWrite names the reason an editor write to path is denied: the
// approval store (or anything under ~/.saga), and the ledger paths the
// checker owns. Symlinks in the existing prefix are resolved first.
func (l *Layer) protectedWrite(path string) string {
	if path == "" {
		return ""
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(l.Store.Root, abs)
	}
	abs = resolveExisting(abs)
	if dir, _, err := ApprovalDirPath(); err == nil {
		if under(resolveExisting(dir), abs) {
			return "the approval store is written by saga gate approve, never by the agent"
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if under(resolveExisting(filepath.Join(home, ".saga")), abs) {
			return "~/.saga is the user's store, never the agent's"
		}
	}
	r, ok := rel(resolveExisting(l.Store.Root), abs)
	if !ok {
		r, ok = rel(l.Store.Root, abs)
	}
	if ok {
		for _, p := range ProtectedPaths {
			if r == strings.TrimSuffix(p, "/") || strings.HasPrefix(r, p) {
				return clip(r, 200) + " is written by the checker, not by the agent"
			}
		}
	}
	return ""
}

// resolveExisting resolves symlinks in the longest existing prefix of p
// (a dangling link included: the agent may plant the link before the
// checker creates the target) and re-attaches the rest.
func resolveExisting(p string) string { return resolveDepth(filepath.Clean(p), 0) }

func resolveDepth(p string, depth int) string {
	if depth > 16 {
		return p
	}
	rest := ""
	for cur := p; ; {
		if c, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(c, rest)
		}
		if fi, err := os.Lstat(cur); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Readlink(cur); err == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(cur), target)
				}
				return resolveDepth(filepath.Join(target, rest), depth+1)
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

func under(dir, p string) bool {
	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

// clip bounds and cleans a text fragment bound for a hook message
// (section 4.4: control- and bidi-stripped, capped per field).
func clip(s string, n int) string {
	s = canon.CleanText(s)
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " ")
	if len(s) > n {
		s = s[:n] + "..."
	}
	return s
}

func (l *Layer) postTool(in *hookio.Input, out *hookio.Output) (*hookio.Output, error) {
	switch in.ToolName {
	case "Edit", "Write", "NotebookEdit", "MultiEdit", "Bash":
	default:
		return out, nil
	}
	ld, err := LoadLite(l.Store.Root, l.Store)
	if err != nil || ld.Contract == nil {
		return out, nil
	}
	findings, err := GuardDiff(GuardInput{Root: ld.Root, Store: ld.Store, Base: ld.Base, Contract: ld.Contract, Config: ld.Config})
	if err != nil {
		l.note("saga gate: guard-diff: %v", err)
		return out, nil
	}
	var parts []string
	for _, f := range findings {
		if f.Blocks() {
			parts = append(parts, fmt.Sprintf("%s %s %s %s pre=%d post=%d", f.ID, clip(f.Path, 120), f.Hunk, f.Rule, f.Pre, f.Post))
		}
	}
	if len(parts) == 0 {
		return out, nil
	}
	msg := "saga gate: " + strings.Join(parts, "; ") + "; waive with `WAIVE: <guard> <path> <hunk> <reason>` in .saga/contract.md (G-LEDGER cannot be waived)"
	out.Decision, out.Reason, out.Exit = hookio.DecisionBlock, l.cap(in, msg, CeilingPost), int(cli.ExitRefusal)
	l.record(in, "guard_diff", map[string]any{"findings": len(parts), "decision": out.Decision})
	return out, nil
}

func (l *Layer) stop(in *hookio.Input, out *hookio.Output) (*hookio.Output, error) {
	so := &StopOutcome{Decision: "inactive"}
	l.lastStop = so
	// No contract in the working tree and no repository to track one at
	// HEAD: the layer is inactive (section 6, first row).
	if _, err := os.Stat(join(l.Store.Root, ContractPath)); errors.Is(err, fs.ErrNotExist) && !snapshot.IsRepo(l.Store.Root) {
		return out, nil
	}
	ld, err := Load(l.Store.Root, l.Store)
	if ld != nil && ld.Missing {
		if ld.TrackedAtHead {
			so.Decision = "deleted"
			out.Decision, out.Reason, out.Exit = hookio.DecisionBlock, l.cap(in, "saga gate: contract deleted; restore .saga/contract.md", CeilingStop), int(cli.ExitRefusal)
			l.record(in, "stop", map[string]any{"decision": "block", "reason": "contract deleted"})
		}
		return out, nil
	}
	if err != nil {
		so.Decision = "invalid"
		out.Decision, out.Reason, out.Exit = hookio.DecisionBlock, l.cap(in, "saga gate: contract invalid: run saga gate lint", CeilingStop), int(cli.CodeOf(err))
		l.record(in, "stop", map[string]any{"decision": "block", "reason": "contract invalid", "exit": int(cli.CodeOf(err))})
		return out, nil
	}
	rep, err := Status(ld, StatusOptions{})
	if err != nil {
		so.Decision = "invalid"
		out.Decision, out.Reason, out.Exit = hookio.DecisionBlock, l.cap(in, "saga gate: contract invalid: run saga gate lint", CeilingStop), int(cli.CodeOf(err))
		return out, nil
	}
	so.Loaded, so.Report = ld, rep
	obs, oerr := l.Store.ReadObserved(in.SessionID)
	if oerr != nil {
		return out, cli.Wrap(cli.ExitEnvironment, "observed", oerr)
	}
	body := map[string]any{
		"for_turn": obs.Turn, "progress_hash": rep.ProgressHash, "tree_hash": rep.TreeHash, "mode": rep.Mode, "exit": rep.Exit,
		"ids": ids(rep), "states": states(rep),
		// The claim verdict is the kind: claim event trace writes after
		// this step (trace-spec section 5.9).
		"claims": nil, "claim_verdict": nil, "claim_reason": "see the kind: claim event of this turn",
	}
	if rep.Exit == 0 {
		obs.GateBlocks, obs.GateProgress = 0, rep.ProgressHash
		body["decision"] = "allow"
		so.Decision = "allow"
		l.record(in, "stop", body)
		return out, l.Store.WriteObserved(obs)
	}
	if in.StopHookActive && obs.GateProgress == rep.ProgressHash {
		obs.GateBlocks++
	} else {
		obs.GateBlocks = 1
	}
	obs.GateProgress = rep.ProgressHash
	so.Blocks = obs.GateBlocks
	if obs.GateBlocks > ld.Config.MaxBlocks {
		so.Decision = "release"
		body["decision"], body["blocks"] = "release", obs.GateBlocks
		out.AdditionalContext = append(out.AdditionalContext, "HANDOFF REQUIRED: "+fmt.Sprint(ld.Config.MaxBlocks)+" Stop blocks without progress")
		l.record(in, "stop", body)
		return out, l.Store.WriteObserved(obs)
	}
	left := SessionShare - obs.Tokens["gate"]
	msg := rep.StopReason(left)
	if canon.TokensEstString(msg) > left {
		msg = fmt.Sprintf("saga gate: %d unmet; run saga gate status", rep.Summary.Unmet+rep.Summary.Unproven+rep.Summary.Manual)
	}
	obs.Tokens["gate"] += canon.TokensEstString(msg)
	rep.Budget = Budget{BytesEmitted: len(msg), TokensEst: canon.TokensEstString(msg), Ceiling: CeilingStop}
	out.Decision, out.Reason, out.Exit = hookio.DecisionBlock, msg, rep.Exit
	so.Decision = "block"
	body["decision"], body["blocks"] = "block", obs.GateBlocks
	l.record(in, "stop", body)
	return out, l.Store.WriteObserved(obs)
}

// cap enforces the per-event ceiling and the session share; the decision
// is unchanged, the text collapses.
func (l *Layer) cap(in *hookio.Input, msg string, ceiling int) string {
	obs, err := l.Store.ReadObserved(in.SessionID)
	if err != nil {
		return msg
	}
	left := SessionShare - obs.Tokens["gate"]
	if canon.TokensEstString(msg) > ceiling || canon.TokensEstString(msg) > left {
		msg = "saga gate: blocked; run saga gate status"
	}
	obs.Tokens["gate"] += canon.TokensEstString(msg)
	_ = l.Store.WriteObserved(obs)
	return msg
}

func ids(r *Report) []string {
	var out []string
	for _, g := range r.Gates {
		out = append(out, g.ID)
	}
	return out
}

func states(r *Report) []string {
	var out []string
	for _, g := range r.Gates {
		out = append(out, g.State)
	}
	return out
}

// record writes one `gate` trace event (contracts section 6).
func (l *Layer) record(in *hookio.Input, kind string, body map[string]any) {
	if in.SessionID == "" {
		return
	}
	obs, err := l.Store.ReadObserved(in.SessionID)
	if err != nil || obs.MaskSalt == "" {
		return
	}
	dir := trace.SessionDir(l.Store, in.SessionID)
	w, err := trace.OpenWriter(dir, in.SessionID, trace.NewMasker(obs.MaskSalt))
	if err != nil {
		return
	}
	defer w.Close()
	body["kind"] = kind
	if err := w.Append(&trace.Event{Type: trace.TypeGate, Source: "hook:" + in.Event, Turn: obs.Turn, Body: body}); err != nil {
		l.note("saga gate: trace: %v", err)
	}
}
