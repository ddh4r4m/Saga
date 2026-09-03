package gate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/schema"
	"github.com/ddh4r4m/saga/internal/store"
)

// Schema ids of the gate records (contracts section 11).
const (
	EvidenceSchema = "saga.gate.evidence/1"
	RedSchema      = "saga.gate.red/1"
	ApprovalSchema = "saga.gate.approval/1"
	StatusSchema   = "saga.gate.status/1"
)

// Outcomes of an evidence record.
const (
	OutcomeMet      = "met"
	OutcomeUnmet    = "unmet"
	OutcomeAttested = "attested"
)

// Matched describes where EXPECT: matched; the output itself is never
// stored (section 4.4).
type Matched struct {
	Kind       string `json:"kind"`
	ExpectHash string `json:"expect_hash"`
	SpanBytes  [2]int `json:"span_bytes"`
}

// Resolved is the environment the oracle ran in.
type Resolved struct {
	Shell           string `json:"shell"`
	CWD             string `json:"cwd"`
	Platform        string `json:"platform"`
	PathFingerprint string `json:"path_fingerprint"`
	PathEntries     int    `json:"path_entries"`
	TimeoutS        int    `json:"timeout_s"`
	OutputCapBytes  int    `json:"output_cap_bytes"`
}

// TreeInfo binds the record to the tree it was observed on.
type TreeInfo struct {
	Base         string  `json:"base"`
	Head         string  `json:"head"`
	WorktreeHash *string `json:"worktree_hash"`
	SnapshotID   *string `json:"snapshot_id"`
	Dirty        bool    `json:"dirty"`
}

// GuardsSummary is the guard state at the time of the check.
type GuardsSummary struct {
	Clean   bool     `json:"clean"`
	Waivers []string `json:"waivers"`
}

// EvidenceRecord is .saga/evidence/<contract>/<gate>.json (section 4.3).
type EvidenceRecord struct {
	Schema       string        `json:"schema"`
	Gate         string        `json:"gate"`
	ContractHash string        `json:"contract_hash"`
	OracleHash   string        `json:"oracle_hash"`
	Outcome      string        `json:"outcome"`
	ExitStatus   *int          `json:"exit_status"`
	Matched      *Matched      `json:"matched"`
	OutputSHA256 *string       `json:"output_sha256"`
	OutputBytes  *int          `json:"output_bytes"`
	DurationMS   *int          `json:"duration_ms"`
	StartedAt    string        `json:"started_at"`
	Resolved     *Resolved     `json:"resolved"`
	Tree         *TreeInfo     `json:"tree"`
	RedProof     *string       `json:"red_proof"`
	Guards       GuardsSummary `json:"guards"`
	Approval     *string       `json:"approval"`
	// Failure is a short fixed label for an unmet outcome (timeout, exit,
	// no-match, output-cap, start-failure); never output text.
	Failure *string `json:"failure,omitempty"`
	// By and Note are set on attested records.
	By   string `json:"by,omitempty"`
	Note string `json:"note,omitempty"`
}

// RunSummary fingerprints one oracle run inside a red record.
type RunSummary struct {
	Exit         int    `json:"exit"`
	Matched      bool   `json:"matched"`
	OutputSHA256 string `json:"output_sha256"`
	OutputBytes  int    `json:"output_bytes"`
}

// RedRecord is .saga/red/<contract>/<gate>.json (section 3.4).
type RedRecord struct {
	Schema      string      `json:"schema"`
	Gate        string      `json:"gate"`
	Mode        string      `json:"mode"`
	OracleHash  string      `json:"oracle_hash"`
	WitnessHash string      `json:"witness_hash"`
	RequestHash *string     `json:"request_hash"`
	ProvedAt    string      `json:"proved_at"`
	Base        string      `json:"base"`
	Operator    any         `json:"operator"`
	Red         RunSummary  `json:"red"`
	Green       *RunSummary `json:"green"`
	Toolchain   Toolchain   `json:"toolchain"`
}

// Store paths.
func evidencePath(s *store.Store, slug, id string) string {
	return s.Path("evidence", slug, id+".json")
}

func redPath(s *store.Store, slug, id string) string { return s.Path("red", slug, id+".json") }

// writeRecord writes v as canonical JSON (so the file bytes hash to the
// record hash every reference carries) and returns that hash.
func writeRecord(path string, v any) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	raw, err := canon.JSON(v)
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	if err := store.WriteFileAtomic(path, raw, 0o600); err != nil {
		return "", err
	}
	return canon.SHA256(raw), nil
}

// readRecord reads a record file through the shape check and returns the
// bytes and their hash.
func readRecord(path string) ([]byte, string, error) {
	if err := store.CheckShape(path); err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return raw, canon.SHA256(raw), nil
}

// LoadEvidence reads the evidence record of a gate. Missing is (nil, "",
// nil).
func LoadEvidence(s *store.Store, slug, id string) (*EvidenceRecord, string, error) {
	raw, hash, err := readRecord(evidencePath(s, slug, id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	var rec EvidenceRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, hash, fmt.Errorf("%s: %w", evidencePath(s, slug, id), err)
	}
	if rec.Schema != EvidenceSchema {
		return &rec, hash, fmt.Errorf("%s: unknown schema %q", evidencePath(s, slug, id), rec.Schema)
	}
	return &rec, hash, nil
}

// WriteEvidence validates and writes the record, returning its hash.
func WriteEvidence(s *store.Store, slug, id string, rec *EvidenceRecord) (string, error) {
	rec.Schema = EvidenceSchema
	if rec.Guards.Waivers == nil {
		rec.Guards.Waivers = []string{}
	}
	v, err := schema.Normalize(rec)
	if err != nil {
		return "", err
	}
	if err := schema.ValidateID(EvidenceSchema, v); err != nil {
		return "", fmt.Errorf("evidence record: %w", err)
	}
	return writeRecord(evidencePath(s, slug, id), rec)
}

// LoadRed reads the red record of a gate. Missing is (nil, "", nil).
func LoadRed(s *store.Store, slug, id string) (*RedRecord, string, error) {
	raw, hash, err := readRecord(redPath(s, slug, id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	var rec RedRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, hash, fmt.Errorf("%s: %w", redPath(s, slug, id), err)
	}
	if rec.Schema != RedSchema {
		return &rec, hash, fmt.Errorf("%s: unknown schema %q", redPath(s, slug, id), rec.Schema)
	}
	return &rec, hash, nil
}

// WriteRed validates and writes the record, returning its hash.
func WriteRed(s *store.Store, slug, id string, rec *RedRecord) (string, error) {
	rec.Schema = RedSchema
	v, err := schema.Normalize(rec)
	if err != nil {
		return "", err
	}
	if err := schema.ValidateID(RedSchema, v); err != nil {
		return "", fmt.Errorf("red record: %w", err)
	}
	return writeRecord(redPath(s, slug, id), rec)
}

// RemoveRed drops a void red record.
func RemoveRed(s *store.Store, slug, id string) { _ = os.Remove(redPath(s, slug, id)) }

// Witnesses resolves the default witnesses of section 3.4 (every
// whitespace-separated token of CHECK: that names an existing regular
// file inside the repo) plus the explicit WITNESS: globs, and returns the
// sorted paths and the witness_hash.
//
// Interpretation recorded in IMPLEMENTATION-STATUS: a default witness
// that matches an IN: glob is the subject of the work, not a transitive
// input of the oracle, so it is left out; otherwise the work itself
// would void every red proof and approval whose CHECK: names the file it
// measures. An explicit WITNESS: glob always binds.
func Witnesses(root string, g *Gate) ([]string, string) {
	return witnesses(root, g, nil)
}

// WitnessesIn is Witnesses with the contract's IN: globs applied.
func WitnessesIn(root string, c *Contract, g *Gate) ([]string, string) {
	return witnesses(root, g, c.In)
}

func witnesses(root string, g *Gate, in []string) ([]string, string) {
	set := map[string]bool{}
	for _, tok := range strings.Fields(g.Check) {
		tok = strings.Trim(tok, `"'`+";&|()<>`")
		if tok == "" || strings.ContainsAny(tok, "*?[") || badPath(tok) != "" {
			continue
		}
		p := filepath.ToSlash(filepath.Clean(tok))
		if MatchAny(in, p, FoldCase()) {
			continue
		}
		if fi, err := os.Lstat(join(root, tok)); err == nil && fi.Mode().IsRegular() {
			set[p] = true
		}
	}
	if len(g.Witness) > 0 {
		for _, p := range listFiles(root) {
			if MatchAny(g.Witness, p, FoldCase()) {
				if fi, err := os.Lstat(join(root, p)); err == nil && fi.Mode().IsRegular() {
					set[p] = true
				}
			}
		}
	}
	paths := make([]string, 0, len(set))
	for p := range set {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		raw, err := os.ReadFile(join(root, p))
		if err != nil {
			continue
		}
		b.WriteString(p)
		b.WriteByte(0)
		b.WriteString(canon.SHA256(raw))
		b.WriteByte('\n')
	}
	return paths, canon.SHA256([]byte(b.String()))
}

// listFiles returns tracked plus untracked-not-ignored files.
func listFiles(root string) []string {
	out, err := gitOut(root, "ls-files", "-co", "--exclude-standard", "-z")
	if err != nil {
		return nil
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// RedReason values of saga.gate.status/1.
const (
	ReasonBaselineMissed = "baseline missed"
	ReasonMutationNotObs = "mutation not observed"
	ReasonNoOperator     = "no operator"
	ReasonWrongReason    = "wrong-reason red"
	ReasonInvalidated    = "invalidated"
	ReasonExpired        = "expired"
	ReasonDeclaredNone   = "declared none"
)

// RedBinding is everything a red proof is bound to (section 3.4).
type RedBinding struct {
	OracleHash  string
	WitnessHash string
	RequestHash string
	Toolchain   Toolchain
}

// RedValid checks a stored record against the current binding and the
// TTLs. The reason is one of the closed set when invalid.
func RedValid(root string, rec *RedRecord, b RedBinding, cfg Config) (bool, string) {
	if rec == nil {
		return false, ""
	}
	req := ""
	if rec.RequestHash != nil {
		req = *rec.RequestHash
	}
	if rec.OracleHash != b.OracleHash || rec.WitnessHash != b.WitnessHash || req != b.RequestHash ||
		rec.Toolchain.Platform != b.Toolchain.Platform || rec.Toolchain.PathFingerprint != b.Toolchain.PathFingerprint || rec.Toolchain.Shell != b.Toolchain.Shell {
		return false, ReasonInvalidated
	}
	if t, err := time.Parse(time.RFC3339, rec.ProvedAt); err == nil && time.Since(t) > time.Duration(cfg.RedTTLDays)*24*time.Hour {
		return false, ReasonExpired
	}
	if rec.Base != "" && CommitsBehind(root, rec.Base) > cfg.RedTTLCommits {
		return false, ReasonExpired
	}
	return true, ""
}

// RealRed applies section 3.3: a red observation is rejected when the
// failure is plausibly for the wrong reason.
func RealRed(r *Result, matched bool, cfg Config) (ok bool, why string) {
	if r.StartFail || r.Exit == 126 || r.Exit == 127 {
		return false, "command missing or not executable"
	}
	if r.TimedOut {
		return false, "timeout"
	}
	if r.Truncated {
		return false, "output cap breached"
	}
	if r.Exit == 0 && matched {
		return false, "green"
	}
	low := strings.ToLower(string(r.Output))
	for _, p := range cfg.RedRejectPatterns {
		if strings.Contains(low, strings.ToLower(p)) {
			return false, "output matches red_reject_pattern " + p
		}
	}
	return true, ""
}
