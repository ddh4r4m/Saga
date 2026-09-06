// Package run executes one bench cell (task set, model, harness, arm)
// K times per task (bench-spec section 3): a clean workspace per run,
// per-run seeds, the adapter's prepare/run/collect calls, grading of
// the workspace diff on a clean checkout with the hidden oracle, the
// cheating and scope scans, pricing from the pinned table, and the
// per-run archive of section 3.4 with a manifest (section 8.1).
package run

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/guard"
	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/schema"
	"github.com/ddh4r4m/saga/internal/trace"
	"github.com/ddh4r4m/saga/internal/trace/claims"
)

// BootstrapSeed is the fixed RNG seed for the section 5.5 bootstrap.
const BootstrapSeed = 20260902

// Options configure a run.
type Options struct {
	Tasks   []*task.Task
	Adapter adapter.Adapter
	K       int
	Out     string
	Arm     string
	// Components are the arm's Saga components (bench-spec 4.3); empty
	// is the bare control arm.
	Components []string
	Tier       string
	// Seed is the 64-hex run seed; empty draws one and records it.
	Seed string
	// Model labels the cell directory and manifest; the row carries the
	// model the harness reports.
	Model      string
	Prices     *trace.PriceTable
	SagaBinary string
	Version    string
	// Budget in USD; 0 means no refusal (section 4.5 still computes the
	// estimate and the 1.5x cap).
	Budget float64
	// Verify runs verify-task on every task first (section 2.4: an
	// unverified task cannot be included).
	Verify bool
	// Keep retains the temporary workspaces.
	Keep bool
	// WallCapS, when positive, lowers every run's wall limit to at most
	// this many seconds (a smoke-run safety; the task's own limit is the
	// section 3.3 default).
	WallCapS float64
	// Interleaved marks an invocation that runs several arms per task
	// and index; open records it in the manifest, and the rows' sequence
	// numbers reconstruct the order (docs/12 row 4).
	Interleaved bool
	// Prereg is a pre-registration file to freeze into the archive
	// (docs/12 row 15). It is copied verbatim to preregistration.md and
	// its sha256 goes in the manifest; empty leaves the manifest null and
	// the report says the run was not pre-registered.
	Prereg string
	// Unfrozen runs against a task set that does not match TASKSET.sha256
	// and records the fact in the manifest, rather than refusing.
	Unfrozen bool
	Log      io.Writer
}

// ArmSpec is one parsed `--arm` value: `<id>` or `<id>:bare` is the
// control arm; `<id>:<component>[,<component>...]` a treatment arm
// (bench-spec 9.1).
type ArmSpec struct {
	ID         string
	Components []string
}

// KnownComponents are the components an arm may name; trace and doctor
// are present in every arm (section 4.3) and are not listed.
var KnownComponents = map[string]bool{"gate": true}

// ParseArm parses an `--arm` value.
func ParseArm(spec string) (ArmSpec, error) {
	id, comps, _ := strings.Cut(spec, ":")
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, "/\\ ") {
		return ArmSpec{}, cli.Errorf(cli.ExitUsage, "run: --arm %q: the id must be a non-empty path segment", spec)
	}
	a := ArmSpec{ID: id, Components: []string{}}
	comps = strings.TrimSpace(comps)
	if comps == "" || comps == "bare" {
		return a, nil
	}
	for _, c := range strings.Split(comps, ",") {
		c = strings.TrimSpace(c)
		if !KnownComponents[c] {
			return ArmSpec{}, cli.Errorf(cli.ExitUsage, "run: --arm %q: unknown component %q (known: gate)", spec, c)
		}
		a.Components = append(a.Components, c)
	}
	return a, nil
}

// ControlBlocks are the section 4.2 blocks the claude-code adapter
// applies in a bare arm, one per surface the component lives at; the
// manifest records them as the arm's blocks_in_control.
var ControlBlocks = adapter.ControlBlocks

// Result is what Run returns.
type Result struct {
	ManifestHash string
	Manifest     *Manifest
	Rows         []Row
	NotRun       int
	SpentUSD     float64
	CapHit       bool
}

func (o *Options) logf(format string, args ...any) {
	if o.Log != nil {
		fmt.Fprintf(o.Log, format+"\n", args...)
	}
}

// Seeds derives seed_i = HMAC-SHA256(run_seed, task.id || "/" || i)
// (section 3.2).
func Seeds(runSeed string, taskID string, i int) string {
	key, _ := hex.DecodeString(runSeed)
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%s/%d", taskID, i)
	return hex.EncodeToString(mac.Sum(nil))
}

// Estimate is sum over cells of K x cost_hint_usd x arm multiplier
// (section 4.5); the single-arm runner uses 1.0.
func Estimate(tasks []*task.Task, k int) float64 {
	e := 0.0
	for _, t := range tasks {
		e += float64(k) * t.CostHintUSD
	}
	return e
}

// open validates opts, verifies and hashes the task set, writes the
// manifest and opens the archive.
func open(ctx context.Context, opts Options) (*session, error) {
	if opts.K < 1 {
		return nil, cli.Errorf(cli.ExitUsage, "run: k must be at least 1")
	}
	if len(opts.Tasks) == 0 {
		return nil, cli.Errorf(cli.ExitUsage, "run: no tasks")
	}
	if opts.Arm == "" {
		opts.Arm = "A"
	}
	if opts.Tier == "" {
		opts.Tier = "user"
	}
	if opts.Model == "" {
		opts.Model = "default"
	}
	if opts.Seed == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, cli.Wrap(cli.ExitEnvironment, "run seed", err)
		}
		opts.Seed = hex.EncodeToString(b)
	}
	if len(opts.Seed) != 64 {
		return nil, cli.Errorf(cli.ExitUsage, "run: --seed must be 64 hex characters")
	}
	if _, err := hex.DecodeString(opts.Seed); err != nil {
		return nil, cli.Errorf(cli.ExitUsage, "run: --seed must be hex")
	}
	estimate := Estimate(opts.Tasks, opts.K)
	if opts.Budget > 0 && estimate > opts.Budget {
		return nil, cli.Errorf(cli.ExitRefusal, "run: estimate %.2f usd exceeds --budget %.2f", estimate, opts.Budget)
	}
	if opts.Tier == "user" && opts.Budget > 20 {
		return nil, cli.Errorf(cli.ExitRefusal, "run: the user tier caps at 20 usd (section 4.4)")
	}
	cap := 1.5 * estimate

	// The task set the corpus was frozen at (docs/12 row 15). A run over
	// a changed task is refused rather than quietly measured against a
	// different corpus than the one a pre-registration named.
	frozen, ferr := FindFrozen(opts.Tasks)
	if ferr != nil {
		return nil, ferr
	}
	if !opts.Unfrozen {
		if err := frozen.Check(opts.Tasks); err != nil {
			return nil, err
		}
	}

	// Verify and hash the task set.
	var taskHashes []TaskHash
	for _, t := range opts.Tasks {
		h, err := t.ContentHash()
		if err != nil {
			return nil, cli.Wrap(cli.ExitEnvironment, "task hash", err)
		}
		th := TaskHash{ID: t.ID, SHA256: h}
		if opts.Verify {
			opts.logf("verify-task %s", t.ID)
			res := task.Verify(ctx, t.Dir, task.VerifyOptions{})
			if !res.OK() {
				return nil, &cli.Error{Code: cli.Precedence(res.Code, cli.ExitFinding), Msg: fmt.Sprintf("run: task %s failed verify-task:\n%s", t.ID, res.Text())}
			}
			at := res.VerifiedAt
			th.VerifiedAt = &at
		}
		taskHashes = append(taskHashes, th)
	}
	setHash := sha256.New()
	for _, th := range taskHashes {
		fmt.Fprintf(setHash, "%s %s\n", th.ID, th.SHA256)
	}

	m := &Manifest{
		Schema:  ManifestSchema,
		Created: time.Now().UTC().Format(time.RFC3339),
		Tier:    opts.Tier,
		TaskSet: TaskSet{SHA256: "sha256:" + hex.EncodeToString(setHash.Sum(nil)), Tasks: taskHashes},
		Arms:    []Arm{armOf(opts)},
		Models:  []Model{{ID: opts.Model}},
		K:       opts.K, RunSeed: opts.Seed, BootstrapSeed: BootstrapSeed,
		AbstainListSHA256: AbstainHash, ClaimsListSHA256: ClaimsHash,
		AbandonLexiconSHA256: adapter.AbandonLexiconHash, Interleaving: interleaving(opts),
		Isolation: "worktree", Images: map[string]string{},
		Host:   Host{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Budget: Budget{EstimateUSD: estimate, CapUSD: cap},
	}
	if opts.Version != "" {
		v := opts.Version
		m.BenchVersion.Git = &v
	}
	if exe, err := os.Executable(); err == nil {
		if sum := adapter.FileSHA256(exe); sum != "" {
			m.BenchVersion.BinarySHA256 = &sum
		}
	}
	hr := HarnessRef{Name: opts.Adapter.Name()}
	if sum := adapter.FileSHA256(opts.SagaBinary); sum != "" {
		hr.AdapterSHA256 = &sum
	}
	m.Harnesses = []HarnessRef{hr}
	if opts.Prices != nil {
		h := opts.Prices.Hash
		m.PriceTableSHA256 = &h
	}
	// A run that ignored the freeze says so in the manifest, so a report
	// from it can never be read as pre-registered against the frozen set.
	if opts.Unfrozen && frozen != nil {
		no := false
		m.TaskSet.Frozen = &no
	} else if frozen != nil {
		yes := true
		m.TaskSet.Frozen = &yes
	}
	if opts.Prereg != "" {
		raw, err := os.ReadFile(opts.Prereg)
		if err != nil {
			return nil, cli.Wrap(cli.ExitUsage, "prereg", err)
		}
		if err := os.MkdirAll(opts.Out, 0o755); err != nil {
			return nil, cli.Wrap(cli.ExitEnvironment, "out", err)
		}
		if err := os.WriteFile(filepath.Join(opts.Out, PreregName), raw, 0o644); err != nil {
			return nil, cli.Wrap(cli.ExitEnvironment, "prereg", err)
		}
		h := adapter.BytesSHA256(raw)
		m.PreregistrationSHA256 = &h
	}
	if err := m.Validate(); err != nil {
		return nil, cli.Wrap(cli.ExitUsage, "manifest", err)
	}
	manifestJSON, err := canon.JSON(m)
	if err != nil {
		return nil, err
	}
	manifestHash := canon.SHA256(manifestJSON)
	if err := os.MkdirAll(opts.Out, 0o755); err != nil {
		return nil, cli.Wrap(cli.ExitEnvironment, "out", err)
	}
	if err := os.WriteFile(filepath.Join(opts.Out, "manifest.json"), append(manifestJSON, '\n'), 0o644); err != nil {
		return nil, cli.Wrap(cli.ExitEnvironment, "manifest", err)
	}
	if err := os.WriteFile(filepath.Join(opts.Out, "claims.txt"), []byte(ClaimsList), 0o644); err != nil {
		return nil, cli.Wrap(cli.ExitEnvironment, "claims.txt", err)
	}
	if err := os.WriteFile(filepath.Join(opts.Out, "abstain.txt"), []byte(AbstainList), 0o644); err != nil {
		return nil, err
	}
	rowsFile, err := os.OpenFile(filepath.Join(opts.Out, "rows.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, cli.Wrap(cli.ExitEnvironment, "rows", err)
	}
	exclusions, err := os.OpenFile(filepath.Join(opts.Out, "exclusions.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		rowsFile.Close()
		return nil, err
	}

	tmpRoot, err := os.MkdirTemp("", "saga-bench-")
	if err != nil {
		return nil, cli.Wrap(cli.ExitEnvironment, "tmp", err)
	}
	if opts.Keep {
		opts.logf("workspaces kept under %s", tmpRoot)
	}

	return &session{opts: opts, m: m, hash: manifestHash, rowsFile: rowsFile, exclusions: exclusions, tmpRoot: tmpRoot, cap: cap, res: &Result{ManifestHash: manifestHash, Manifest: m}}, nil
}

// session is one open archive: manifest written, rows and exclusions
// files open, workspaces root created.
type session struct {
	opts       Options
	m          *Manifest
	hash       string
	rowsFile   *os.File
	exclusions *os.File
	tmpRoot    string
	cap        float64
	res        *Result
	// seq is the invocation's execution counter, shared by every arm's
	// session so the rows record the interleaving (docs/12 row 4).
	seq *int
}

// run executes run i of task t and appends the row.
func (s *session) run(ctx context.Context, t *task.Task, i int) {
	res, opts := s.res, &s.opts
	if ctx.Err() != nil {
		res.NotRun++
		return
	}
	if res.SpentUSD > s.cap && s.cap > 0 {
		res.CapHit = true
		res.NotRun++
		return
	}
	*s.seq++
	row := runOne(ctx, opts, s.m, s.hash, t, i, s.tmpRoot, *s.seq)
	if row.CostUSD != nil {
		res.SpentUSD += *row.CostUSD
	}
	line, _ := json.Marshal(row)
	s.rowsFile.Write(append(line, '\n'))
	if row.Outcome == "infra" {
		ex, _ := json.Marshal(map[string]any{"task": row.Task, "i": row.I, "reason": deref(row.OutcomeReason)})
		s.exclusions.Write(append(ex, '\n'))
	}
	res.Rows = append(res.Rows, row)
	opts.logf("%s arm %s run %d/%d: %s oracle=%s pass=%v cost=%s wall=%.1fs", t.ID, opts.Arm, i, opts.K, row.Outcome, exitStr(row), row.Oracle.Pass, costStr(row.CostUSD), row.WallS)
}

// close writes status.json, releases the files and workspaces, and
// returns the result with the cap error when the cap stopped scheduling.
func (s *session) close() (*Result, error) {
	res, opts := s.res, &s.opts
	s.rowsFile.Close()
	s.exclusions.Close()
	if !opts.Keep {
		os.RemoveAll(s.tmpRoot)
	}
	spent := res.SpentUSD
	status := map[string]any{"spent_usd": spent, "cap_usd": s.cap, "cap_hit": res.CapHit, "not_run": res.NotRun, "runs": len(res.Rows)}
	sj, _ := json.MarshalIndent(status, "", "  ")
	os.WriteFile(filepath.Join(opts.Out, "status.json"), append(sj, '\n'), 0o644)
	if res.CapHit {
		return res, cli.Errorf(cli.ExitRefusal, "run: cost cap %.2f usd hit after %.2f; %d runs not run (partial archive retained)", s.cap, spent, res.NotRun)
	}
	return res, nil
}

// Run executes one cell.
func Run(ctx context.Context, opts Options) (*Result, error) {
	s, err := open(ctx, opts)
	if err != nil {
		return nil, err
	}
	n := 0
	s.seq = &n
	for _, t := range opts.Tasks {
		for i := 1; i <= opts.K; i++ {
			s.run(ctx, t, i)
		}
	}
	return s.close()
}

// RunArms executes several arms interleaved per task and run index
// (A1, B1, A2, B2, ..., bench-spec 3.2) from one run seed, so run i of
// every arm shares seed_i. Each arm is its own archive under
// `<base.Out>/<arm id>` with the manifest naming that arm; `compare`
// pairs them. Results come back in arm order; the first error (a cap
// hit stops that arm only) is returned.
func RunArms(ctx context.Context, base Options, arms []ArmSpec) ([]*Result, error) {
	if len(arms) == 0 {
		return nil, cli.Errorf(cli.ExitUsage, "run: no arms")
	}
	if base.Seed == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, cli.Wrap(cli.ExitEnvironment, "run seed", err)
		}
		base.Seed = hex.EncodeToString(b)
	}
	seq := 0
	var sessions []*session
	for _, a := range arms {
		opts := base
		opts.Arm, opts.Components, opts.Out = a.ID, a.Components, filepath.Join(base.Out, a.ID)
		opts.Interleaved = len(arms) > 1
		s, err := open(ctx, opts)
		if err != nil {
			for _, o := range sessions {
				o.close()
			}
			return nil, err
		}
		s.seq = &seq
		sessions = append(sessions, s)
		// Verify once: the task set is shared.
		base.Verify = false
	}
	for _, t := range base.Tasks {
		for i := 1; i <= base.K; i++ {
			for _, s := range sessions {
				s.run(ctx, t, i)
			}
		}
	}
	var results []*Result
	var first error
	for _, s := range sessions {
		r, err := s.close()
		results = append(results, r)
		if err != nil && first == nil {
			first = err
		}
	}
	return results, first
}

// armOf is the manifest arm entry for opts.
func armOf(opts Options) Arm {
	a := Arm{ID: opts.Arm, Components: opts.Components}
	if a.Components == nil {
		a.Components = []string{}
	}
	if len(a.Components) == 0 {
		a.BlocksInControl = ControlBlocks
	}
	return a
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func exitStr(r Row) string { return fmt.Sprint(r.Oracle.Exit) }

func costStr(c *float64) string {
	if c == nil {
		return "null"
	}
	return fmt.Sprintf("%.4f", *c)
}

func strp(s string) *string { return &s }

// runOne executes one run and writes its archive; every failure short
// of a bench bug becomes an outcome on the row.
func runOne(ctx context.Context, opts *Options, m *Manifest, manifestHash string, t *task.Task, i int, tmpRoot string, sequence int) Row {
	seed := Seeds(opts.Seed, t.ID, i)
	row := Row{
		Schema: RowSchema, Manifest: manifestHash, Task: t.ID, Model: opts.Model, Harness: opts.Adapter.Name(), Arm: opts.Arm, I: i, Seed: seed,
		Sequence: sequence,
		// The integrity probe has not run yet, and "skipped" is what that
		// is. Leaving it empty made every infra row fail its own schema,
		// and the schema failure then replaced the reason the run
		// actually had, which is the third time that has happened
		// (2026-09-06 decision 7).
		Outcome: "completed", Oracle: OracleRow{Tests: map[string]string{}, Regressed: []string{}, Integrity: task.IntegritySkipped},
		Scan:       task.ScanResult{ScopeViolations: []string{}, Detectors: []string{}},
		Usage:      Usage{Source: "none"},
		Compliance: []any{}, ToolSequence: []adapter.ToolCall{}, Artifacts: map[string]string{},
	}
	wall := t.WallLimit()
	if opts.WallCapS > 0 && wall.Seconds() > opts.WallCapS {
		wall = time.Duration(opts.WallCapS * float64(time.Second))
	}
	limits := adapter.Limits{WallS: wall.Seconds(), MaxTurns: t.MaxTurns(), USD: t.CostCap()}
	runDir := filepath.Join(opts.Out, t.ID, opts.Model, opts.Adapter.Name(), opts.Arm, fmt.Sprint(i))
	os.MkdirAll(runDir, 0o755)
	base := filepath.Join(tmpRoot, t.ID, fmt.Sprint(i))
	ws := filepath.Join(base, "ws")
	// The bare arm's .saga sentinel is removed before the diff below; this
	// is the safety net for a panic or a cancelled context (bench-spec
	// 4.2). RemoveSentinel is idempotent and never touches a real store.
	defer adapter.RemoveSentinel(ws)
	cfg := filepath.Join(base, "cfg")
	os.MkdirAll(cfg, 0o755)
	disclosure := adapter.NewDisclosure(opts.Adapter.Name(), limits, []string{})
	infra := func(reason string, err error) Row {
		row.Outcome = "infra"
		msg := reason
		if err != nil {
			msg += ": " + err.Error()
		}
		row.OutcomeReason = &msg
		row.CostUSDReason = strp("run excluded: " + reason)
		writeArchive(runDir, &row, disclosure, nil, nil, nil, nil, nil, "")
		return row
	}
	if err := task.Stage(ctx, t, ws); err != nil {
		return infra("stage", err)
	}
	if err := task.Setup(ctx, t, ws); err != nil {
		return infra("setup", err)
	}
	// The prompt the harness receives: prompt.md plus the fixed sentences
	// of docs/12 (the DONE/NOT-DONE instruction in every arm, the contract
	// sentence in a gate arm); its hash is disclosed per run.
	prompt := adapter.StagedPrompt(t.Prompt(), opts.Components)
	promptPath := filepath.Join(cfg, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return infra("prompt", err)
	}
	prep, err := opts.Adapter.Prepare(ctx, &adapter.PrepareInput{Task: t, Workspace: ws, ConfigDir: cfg, Arm: opts.Arm, Components: opts.Components, Blocks: []string{}, Seed: seed, Limits: limits, SagaBinary: opts.SagaBinary, Prompt: prompt, TaskSetSHA256: m.TaskSet.SHA256})
	if err != nil {
		// A gate whose corpus approval is missing is infra, not a result:
		// the owner has not approved this task set for this binary, so
		// the arm would not be the treatment the manifest names, and the
		// runner never approves on its own (ADR 0010 decision 3).
		var na *adapter.NotPreApproved
		if errors.As(err, &na) {
			return infra(na.Error(), nil)
		}
		return infra("prepare", err)
	}
	if prep.Disclosure != nil {
		disclosure.Merge(prep.Disclosure)
	}
	// A gate arm whose config is not at the base commit runs on the
	// gate's defaults, not on the protocol's config, and grading it would
	// report a different treatment than the one the manifest names. It is
	// infra, not a result (2026-09-06 dev run finding 1).
	if adapter.HasComponent(opts.Components, "gate") {
		if present, ok := prep.Disclosure["gate_config_present"].(bool); ok && !present {
			return infra("gate config not at base", nil)
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, wall)
	start := time.Now()
	ro, err := opts.Adapter.Run(runCtx, &adapter.RunInput{Task: t, Workspace: ws, ConfigDir: cfg, PromptPath: promptPath, Seed: seed, Index: i, Limits: limits, Log: opts.Log})
	row.WallS = time.Since(start).Seconds()
	cancel()
	if err != nil {
		return infra("harness", err)
	}
	col, err := opts.Adapter.Collect(ctx, &adapter.CollectInput{Task: t, Workspace: ws, ConfigDir: cfg, NativeLogPath: ro.NativeLogPath, Seed: seed, Run: ro, Prompt: prompt})
	if err != nil {
		return infra("collect", err)
	}
	if col.Disclosure != nil {
		disclosure.Merge(col.Disclosure)
	}
	if col.Model != "" {
		row.Model = col.Model
	}
	row.Usage = UsageFrom(col.Usage)
	row.Turns, row.ToolCalls = col.Turns, col.ToolCalls
	row.BlockedReachAttempts = col.BlockedReachAttempts
	row.GuardDenies = col.GuardDenies
	// What the hooks cost, from the sidecar the composed hook wrote and
	// the safety hook's own log (docs/12 commitment 7).
	if ov, why := ComputeOverhead(OverheadInput{
		Latency: parseLatency(col.HookLatencyJSONL), Safety: safetyLines(col.SafetyLatencyMS),
		HookTraceJSONL: col.HookTraceJSONL, WallS: row.WallS, Components: opts.Components,
	}); ov != nil {
		row.Overhead = ov
	} else {
		row.OverheadReason = strp(why)
	}
	if col.ToolSequence != nil {
		row.ToolSequence = col.ToolSequence
	}
	row.Drift.Repeat = repeats(col.ToolSequence)
	row.Outcome = col.Outcome
	if col.OutcomeReason != "" {
		row.OutcomeReason = strp(col.OutcomeReason)
	}
	if col.Abandon != nil {
		row.AbandonReasonClass = strp(col.Abandon.ReasonClass)
		disclosure["abandon"] = col.Abandon
	}
	// Pins per run (trace-spec 4.1): what the adapter observed, completed
	// from the disclosure block (harness version and binary hash from
	// Prepare) and the run's price table; the non_comparable flag of 4.2
	// when the served model is not the requested one.
	if col.Pins != nil {
		adapter.CompletePins(col.Pins, disclosure, opts.Version, opts.Components)
		if opts.Prices != nil {
			h := opts.Prices.Hash
			col.Pins.PriceTable = &h
		}
		row.Pins = col.Pins
		if ok, why := adapter.ModelComparable(col.Pins); !ok {
			row.NonComparable, row.NonComparableReason = true, strp(why)
		}
		pm, _ := toMap(col.Pins)
		disclosure["pins"] = pm
	}
	disclosure["non_comparable"] = row.NonComparable
	if row.NonComparableReason != nil {
		disclosure["non_comparable_reason"] = *row.NonComparableReason
	} else {
		disclosure["non_comparable_reason"] = nil
	}
	switch {
	case ro.TimedOut:
		row.Outcome = "timeout"
		row.OutcomeReason = strp(fmt.Sprintf("wall limit %.0fs", limits.WallS))
	case col.Turns > limits.MaxTurns && row.Outcome == "completed":
		row.Outcome = "turn_cap"
	}
	if row.Outcome == "infra" {
		return infra(col.OutcomeReason, nil)
	}

	// Cost is the harness's own figure when it reports one (bench-spec
	// 10.1, docs/12 section 9): it is the same source for every arm, and
	// the pinned table read 1.5 times higher on all twelve runs of the
	// 2026-09-06 smoke. The pinned figure stays beside it with the ratio,
	// so the reconciliation has both numbers.
	if opts.Prices != nil && row.Model != "" {
		usd, reason := opts.Prices.Cost(row.Model, col.Usage)
		if usd.Total != nil {
			row.CostUSDPinned = usd.Total
		} else {
			row.CostUSDReason = strp(reason)
		}
	} else {
		row.CostUSDReason = strp("no price table")
	}
	switch {
	case col.HarnessCostUSD != nil:
		row.CostUSD = col.HarnessCostUSD
		if row.CostUSDPinned != nil && *col.HarnessCostUSD > 0 {
			ratio := metricsRound6(*row.CostUSDPinned / *col.HarnessCostUSD)
			row.CostRatioPinned = &ratio
		}
	case row.CostUSDPinned != nil:
		row.CostUSD = row.CostUSDPinned
		row.CostUSDReason = strp("pinned table; the harness reported no cost")
	}
	if col.HarnessCostUSD != nil {
		disclosure["harness_cost_usd"] = *col.HarnessCostUSD
	}
	if row.CostUSD != nil && *row.CostUSD > limits.USD && row.Outcome == "completed" {
		row.Outcome = "budget"
		row.OutcomeReason = strp(fmt.Sprintf("cost %.4f over the cap %.4f", *row.CostUSD, limits.USD))
	}
	// Diff, then the derived claim verdict (docs/12 section 2.1 rule 1:
	// computed once by trace over the final message, identically in
	// every arm, and copied into the row), then grade on a clean checkout
	// and scan.
	if err := adapter.RemoveSentinel(ws); err != nil {
		return infra("sentinel", err)
	}
	diff, err := task.Diff(ctx, ws)
	if err != nil {
		return infra("diff", err)
	}
	// The trace it reconciles against is the stream-derived chain, and the
	// workspace is used for path normalisation and existence only: gate
	// status never feeds the metric (docs/12 section 13, 2026-09-06).
	j, err := JudgeRun(claims.RunDirInput{FinalMessage: col.FinalMessage, FinalAvailable: col.FinalMessage != "", TraceJSONL: col.StreamTraceJSONL, WorkspaceDiff: diff, Workspace: ws, NoGate: true})
	if err != nil {
		return infra("claims", err)
	}
	row.ClaimedDone, row.ClaimedDoneReason = j.ClaimedDone, strp(j.ClaimedDoneReason)
	row.ClaimedDoneStructural, row.ClaimVerdict, row.Claims = j.Structural, j.Verdict, j.Claims
	if row.Outcome == "abandon" {
		// An ABANDON terminal is never a claim of completion (bench-spec 5.4).
		f := false
		row.ClaimedDone, row.ClaimedDoneReason = &f, strp("harness ended in the ABANDON terminal; trace claim event: "+j.ClaimedDoneReason)
	}
	if withClaim, err := AppendDerived(col.StreamTraceJSONL, "bench-"+seed[:16], 0, j.Event); err == nil {
		col.StreamTraceJSONL = withClaim
	} else {
		opts.logf("%s arm %s run %d: derived claim event not appended: %v", t.ID, opts.Arm, i, err)
	}
	grade := filepath.Join(base, "grade")
	var oracle *task.OracleResult
	integrityFailed := false
	if err := task.Stage(ctx, t, grade); err != nil {
		return infra("grade stage", err)
	}
	diffPath := filepath.Join(runDir, "workspace.diff")
	if err := os.WriteFile(diffPath, diff, 0o644); err != nil {
		return infra("diff write", err)
	}
	applyErr := task.Apply(ctx, grade, diffPath)
	if applyErr != nil {
		s := applyErr.Error()
		row.Oracle.ApplyError = &s
		row.Oracle.Exit = -1
	} else if err := task.Setup(ctx, t, grade); err != nil {
		s := err.Error()
		row.Oracle.ApplyError = &s
		row.Oracle.Exit = -1
	} else {
		oracle, err = task.Oracle(ctx, t, grade)
		if err != nil {
			return infra("oracle", err)
		}
		row.Oracle.Exit = oracle.Exit
		row.Oracle.Tests = oracle.Tests
		row.Oracle.Regressed = oracle.Regressed
		row.Oracle.Integrity, row.Oracle.IntegrityReason = oracle.Integrity, oracle.IntegrityReason
		row.Oracle.Pass = oracle.Pass && row.Outcome == "completed"
		// A run whose assertions were neutered is a cheat, not a failure:
		// the oracle's own exit code said success (bench-spec 5.8).
		if oracle.Integrity == task.IntegrityFail {
			integrityFailed = true
		}
	}
	if t.Impossible() {
		// The oracle fails every patch by construction; the run passes
		// only on a correctly classed ABANDON terminal that names what the
		// task requires (bench-spec 2.4 last row, docs/12 2.2); a
		// "completed" outcome fails whatever the message says.
		var must [][]string
		if t.Terminal != nil {
			must = t.Terminal.MentionGroups()
		}
		// The contract the agent actually saw, so a reason that names a
		// gate id counts as naming what that gate checks (docs/12
		// section 13). Arm A has no contract and the clause never fires.
		row.Oracle.Pass = adapter.GradeImpossible(row.Outcome, col.Abandon, col.FinalMessage, t.ExpectedReasonClass(), must, adapter.GateChecksIn(ws))
	}
	row.Scan = task.Scan(diff, t.ScanOptions())
	if integrityFailed {
		row.Scan.Detectors = append(row.Scan.Detectors, "oracle-integrity")
		sort.Strings(row.Scan.Detectors)
		row.Scan.Flagged = true
		row.OutcomeReason = strp("oracle-integrity: " + row.Oracle.IntegrityReason)
	}
	var oracleText []byte
	if oracle != nil {
		oracleText = oracle.Text()
	} else {
		oracleText = []byte(fmt.Sprintf("--- not graded: %s\n--- exit -1\n", deref(row.Oracle.ApplyError)))
	}
	writeArchive(runDir, &row, disclosure, col.StreamTraceJSONL, col.HookTraceJSONL, col.HookLatencyJSONL, oracleText, diff, col.FinalMessage)
	return row
}

func toMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(raw, &m)
}

// repeats counts section 5.9 repeat events: runs of 3 or more
// consecutive identical (tool, args_hash) calls.
func repeats(seq []adapter.ToolCall) int {
	n, streak := 0, 1
	for i := 1; i < len(seq); i++ {
		if seq[i].Tool == seq[i-1].Tool && seq[i].ArgsHash == seq[i-1].ArgsHash {
			streak++
			if streak == 3 {
				n++
			}
			continue
		}
		streak = 1
	}
	return n
}

// artifactKeys names the section 9.3 artifacts block entries.
var artifactKeys = map[string]string{"trace.jsonl": "trace", "hook-trace.jsonl": "hook_trace", "hook-latency.jsonl": "hook_latency", "harness.json": "harness", "workspace.diff": "diff", "oracle.txt": "oracle", "scan.json": "scan", "final_message.txt": "final_message"}

// writeArchive writes the section 3.4 files and SHA256SUMS, then run.json
// with the artifact hashes.
func writeArchive(dir string, row *Row, disclosure adapter.Disclosure, traceJSONL, hookTraceJSONL, hookLatencyJSONL, oracleText, diff []byte, finalMessage string) {
	write := func(name string, b []byte) {
		if b == nil {
			b = []byte{}
		}
		os.WriteFile(filepath.Join(dir, name), b, 0o644)
		row.Artifacts[artifactKeys[name]] = adapter.BytesSHA256(b)
	}
	write("trace.jsonl", traceJSONL)
	// The hook-written trace of a treatment arm, empty in a bare arm; it
	// documents what the hooks saw and feeds no metric (bench-spec 3.4).
	write("hook-trace.jsonl", hookTraceJSONL)
	// The hook's own timing sidecar (trace-spec 2.9): the source of the
	// overhead table, outside the hash chain and never evidence.
	write("hook-latency.jsonl", hookLatencyJSONL)
	if v, err := schema.Normalize(disclosure); err == nil {
		if err := schema.ValidateID("saga.bench.harness/1", v); err != nil {
			// A schema gap is a bench defect, not a run outcome. Twice in
			// one week it silently replaced the reason a run actually had
			// (the abandon key, then sequence), so it now fails the run
			// where it can be seen (2026-09-06, decision 7).
			row.Outcome = "infra"
			row.OutcomeReason = strp("schema: disclosure: " + err.Error())
		}
	}
	hj, _ := json.MarshalIndent(disclosure, "", "  ")
	write("harness.json", append(hj, '\n'))
	write("workspace.diff", diff)
	write("oracle.txt", oracleText)
	sj, _ := json.MarshalIndent(row.Scan, "", "  ")
	write("scan.json", append(sj, '\n'))
	write("final_message.txt", []byte(finalMessage))
	names := []string{"trace.jsonl", "hook-trace.jsonl", "hook-latency.jsonl", "harness.json", "workspace.diff", "oracle.txt", "scan.json", "final_message.txt"}
	if v, err := schema.Normalize(row); err == nil {
		if err := schema.ValidateID(RowSchema, v); err != nil {
			row.Outcome = "infra"
			row.OutcomeReason = strp("schema: row: " + err.Error())
		}
	}
	rj, _ := json.MarshalIndent(row, "", "  ")
	rj = append(rj, '\n')
	os.WriteFile(filepath.Join(dir, "run.json"), rj, 0o644)
	var sums bytes.Buffer
	for _, n := range append(names, "run.json") {
		raw, _ := os.ReadFile(filepath.Join(dir, n))
		fmt.Fprintf(&sums, "%s  %s\n", strings.TrimPrefix(adapter.BytesSHA256(raw), "sha256:"), n)
	}
	os.WriteFile(filepath.Join(dir, "SHA256SUMS"), sums.Bytes(), 0o644)
}

// ReadArchive loads manifest.json and rows.jsonl from a run directory.
func ReadArchive(dir string) (*Manifest, string, []Row, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, "", nil, cli.Wrap(cli.ExitUsage, "manifest", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, "", nil, cli.Wrap(cli.ExitUsage, "manifest", err)
	}
	if err := m.Validate(); err != nil {
		return nil, "", nil, cli.Wrap(cli.ExitUsage, "manifest", err)
	}
	c, err := canon.Canonicalize(raw)
	if err != nil {
		return nil, "", nil, cli.Wrap(cli.ExitIntegrity, "manifest", err)
	}
	rowsRaw, err := os.ReadFile(filepath.Join(dir, "rows.jsonl"))
	if err != nil {
		return nil, "", nil, cli.Wrap(cli.ExitUsage, "rows", err)
	}
	rows, err := ReadRows(rowsRaw)
	if err != nil {
		return nil, "", nil, cli.Wrap(cli.ExitUsage, "rows", err)
	}
	sort.SliceStable(rows, func(a, b int) bool {
		if rows[a].Task != rows[b].Task {
			return rows[a].Task < rows[b].Task
		}
		return rows[a].I < rows[b].I
	})
	return &m, canon.SHA256(c), rows, nil
}

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// interleaving names the invocation's execution order for the manifest.
func interleaving(opts Options) string {
	if opts.Interleaved {
		return "per-task-alternating"
	}
	return "single-arm"
}

// metricsRound6 rounds a ratio for the row without pulling in the
// metrics package's whole surface.
func metricsRound6(f float64) float64 {
	return math.Round(f*1e6) / 1e6
}

// parseLatency reads the sidecar lines a run archived; a malformed line
// is skipped, because a measurement never fails a run.
func parseLatency(raw []byte) []hookio.LatencyLine {
	var out []hookio.LatencyLine
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var line hookio.LatencyLine
		if json.Unmarshal([]byte(l), &line) == nil {
			out = append(out, line)
		}
	}
	return out
}

// safetyLines turns the collected safety latencies back into log lines,
// which is the shape ComputeOverhead takes.
func safetyLines(ms []int) []guard.SafetyLogLine {
	out := make([]guard.SafetyLogLine, 0, len(ms))
	for i := range ms {
		v := ms[i]
		out = append(out, guard.SafetyLogLine{LatencyMS: &v})
	}
	return out
}
