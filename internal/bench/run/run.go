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
	"fmt"
	"io"
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
// applies in a bare arm.
var ControlBlocks = []string{"path-shim:saga"}

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
		AbstainListSHA256: AbstainHash, ClaimsListSHA256: ClaimsHash, Isolation: "worktree", Images: map[string]string{},
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
	row := runOne(ctx, opts, s.m, s.hash, t, i, s.tmpRoot)
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
	var sessions []*session
	for _, a := range arms {
		opts := base
		opts.Arm, opts.Components, opts.Out = a.ID, a.Components, filepath.Join(base.Out, a.ID)
		s, err := open(ctx, opts)
		if err != nil {
			for _, o := range sessions {
				o.close()
			}
			return nil, err
		}
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
func runOne(ctx context.Context, opts *Options, m *Manifest, manifestHash string, t *task.Task, i int, tmpRoot string) Row {
	seed := Seeds(opts.Seed, t.ID, i)
	row := Row{
		Schema: RowSchema, Manifest: manifestHash, Task: t.ID, Model: opts.Model, Harness: opts.Adapter.Name(), Arm: opts.Arm, I: i, Seed: seed,
		Outcome: "completed", Oracle: OracleRow{Tests: map[string]string{}, Regressed: []string{}},
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
		writeArchive(runDir, &row, disclosure, nil, nil, nil, nil, "")
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
	prep, err := opts.Adapter.Prepare(ctx, &adapter.PrepareInput{Task: t, Workspace: ws, ConfigDir: cfg, Arm: opts.Arm, Components: opts.Components, Blocks: []string{}, Seed: seed, Limits: limits, SagaBinary: opts.SagaBinary, Prompt: prompt})
	if err != nil {
		return infra("prepare", err)
	}
	if prep.Disclosure != nil {
		disclosure.Merge(prep.Disclosure)
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

	// Cost from the pinned table; the harness figure is disclosed beside it.
	if opts.Prices != nil && row.Model != "" {
		usd, reason := opts.Prices.Cost(row.Model, col.Usage)
		if usd.Total != nil {
			row.CostUSD = usd.Total
		} else {
			row.CostUSDReason = strp(reason)
		}
	} else {
		row.CostUSDReason = strp("no price table")
	}
	if row.CostUSD == nil && col.HarnessCostUSD != nil {
		row.CostUSD = col.HarnessCostUSD
		row.CostUSDReason = strp("harness-reported; model unpriced in the pinned table")
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
		row.Oracle.Pass = oracle.Pass && row.Outcome == "completed"
	}
	if t.Impossible() {
		// The oracle fails every patch by construction; the run passes
		// only on a correctly classed ABANDON terminal that names what the
		// task requires (bench-spec 2.4 last row, docs/12 2.2); a
		// "completed" outcome fails whatever the message says.
		var must []string
		if t.Terminal != nil {
			must = t.Terminal.ReasonMustMention
		}
		row.Oracle.Pass = adapter.GradeImpossible(row.Outcome, col.Abandon, col.FinalMessage, t.ExpectedReasonClass(), must)
	}
	row.Scan = task.Scan(diff, t.ScanOptions())
	var oracleText []byte
	if oracle != nil {
		oracleText = oracle.Text()
	} else {
		oracleText = []byte(fmt.Sprintf("--- not graded: %s\n--- exit -1\n", deref(row.Oracle.ApplyError)))
	}
	writeArchive(runDir, &row, disclosure, col.StreamTraceJSONL, col.HookTraceJSONL, oracleText, diff, col.FinalMessage)
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
var artifactKeys = map[string]string{"trace.jsonl": "trace", "hook-trace.jsonl": "hook_trace", "harness.json": "harness", "workspace.diff": "diff", "oracle.txt": "oracle", "scan.json": "scan", "final_message.txt": "final_message"}

// writeArchive writes the section 3.4 files and SHA256SUMS, then run.json
// with the artifact hashes.
func writeArchive(dir string, row *Row, disclosure adapter.Disclosure, traceJSONL, hookTraceJSONL, oracleText, diff []byte, finalMessage string) {
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
	if v, err := schema.Normalize(disclosure); err == nil {
		if err := schema.ValidateID("saga.bench.harness/1", v); err != nil {
			row.OutcomeReason = strp("disclosure schema: " + err.Error())
		}
	}
	hj, _ := json.MarshalIndent(disclosure, "", "  ")
	write("harness.json", append(hj, '\n'))
	write("workspace.diff", diff)
	write("oracle.txt", oracleText)
	sj, _ := json.MarshalIndent(row.Scan, "", "  ")
	write("scan.json", append(sj, '\n'))
	write("final_message.txt", []byte(finalMessage))
	names := []string{"trace.jsonl", "hook-trace.jsonl", "harness.json", "workspace.diff", "oracle.txt", "scan.json", "final_message.txt"}
	if v, err := schema.Normalize(row); err == nil {
		if err := schema.ValidateID(RowSchema, v); err != nil {
			row.OutcomeReason = strp("row schema: " + err.Error())
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
