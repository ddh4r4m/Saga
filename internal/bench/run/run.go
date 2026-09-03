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
	Tier    string
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
	Log  io.Writer
}

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

// Run executes the cell.
func Run(ctx context.Context, opts Options) (*Result, error) {
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
		Arms:    []Arm{{ID: opts.Arm, Components: []string{}}},
		Models:  []Model{{ID: opts.Model}},
		K:       opts.K, RunSeed: opts.Seed, BootstrapSeed: BootstrapSeed,
		AbstainListSHA256: AbstainHash, Isolation: "worktree", Images: map[string]string{},
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
	if err := os.WriteFile(filepath.Join(opts.Out, "abstain.txt"), []byte(AbstainList), 0o644); err != nil {
		return nil, err
	}
	rowsFile, err := os.OpenFile(filepath.Join(opts.Out, "rows.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, cli.Wrap(cli.ExitEnvironment, "rows", err)
	}
	defer rowsFile.Close()
	exclusions, err := os.OpenFile(filepath.Join(opts.Out, "exclusions.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	defer exclusions.Close()

	tmpRoot, err := os.MkdirTemp("", "saga-bench-")
	if err != nil {
		return nil, cli.Wrap(cli.ExitEnvironment, "tmp", err)
	}
	if !opts.Keep {
		defer os.RemoveAll(tmpRoot)
	} else {
		opts.logf("workspaces kept under %s", tmpRoot)
	}

	res := &Result{ManifestHash: manifestHash, Manifest: m}
	for _, t := range opts.Tasks {
		for i := 1; i <= opts.K; i++ {
			if ctx.Err() != nil {
				res.NotRun++
				continue
			}
			if res.SpentUSD > cap && cap > 0 {
				res.CapHit = true
				res.NotRun++
				continue
			}
			row := runOne(ctx, &opts, m, manifestHash, t, i, tmpRoot)
			if row.CostUSD != nil {
				res.SpentUSD += *row.CostUSD
			}
			line, _ := json.Marshal(row)
			rowsFile.Write(append(line, '\n'))
			if row.Outcome == "infra" {
				ex, _ := json.Marshal(map[string]any{"task": row.Task, "i": row.I, "reason": deref(row.OutcomeReason)})
				exclusions.Write(append(ex, '\n'))
			}
			res.Rows = append(res.Rows, row)
			opts.logf("%s run %d/%d: %s oracle=%s pass=%v cost=%s wall=%.1fs", t.ID, i, opts.K, row.Outcome, exitStr(row), row.Oracle.Pass, costStr(row.CostUSD), row.WallS)
		}
	}
	spent := res.SpentUSD
	status := map[string]any{"spent_usd": spent, "cap_usd": cap, "cap_hit": res.CapHit, "not_run": res.NotRun, "runs": len(res.Rows)}
	sj, _ := json.MarshalIndent(status, "", "  ")
	os.WriteFile(filepath.Join(opts.Out, "status.json"), append(sj, '\n'), 0o644)
	if res.CapHit {
		return res, cli.Errorf(cli.ExitRefusal, "run: cost cap %.2f usd hit after %.2f; %d runs not run (partial archive retained)", cap, spent, res.NotRun)
	}
	return res, nil
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
	limits := adapter.Limits{WallS: t.WallLimit().Seconds(), MaxTurns: t.MaxTurns(), USD: t.CostCap()}
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
		writeArchive(runDir, &row, disclosure, nil, nil, nil, "")
		return row
	}
	if err := task.Stage(ctx, t, ws); err != nil {
		return infra("stage", err)
	}
	if err := task.Setup(ctx, t, ws); err != nil {
		return infra("setup", err)
	}
	promptPath := filepath.Join(cfg, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(t.Prompt()), 0o644); err != nil {
		return infra("prompt", err)
	}
	prep, err := opts.Adapter.Prepare(ctx, &adapter.PrepareInput{Task: t, Workspace: ws, ConfigDir: cfg, Arm: opts.Arm, Blocks: []string{}, Seed: seed, Limits: limits, SagaBinary: opts.SagaBinary})
	if err != nil {
		return infra("prepare", err)
	}
	if prep.Disclosure != nil {
		disclosure.Merge(prep.Disclosure)
	}

	runCtx, cancel := context.WithTimeout(ctx, t.WallLimit())
	start := time.Now()
	ro, err := opts.Adapter.Run(runCtx, &adapter.RunInput{Task: t, Workspace: ws, ConfigDir: cfg, PromptPath: promptPath, Seed: seed, Index: i, Limits: limits, Log: opts.Log})
	row.WallS = time.Since(start).Seconds()
	cancel()
	if err != nil {
		return infra("harness", err)
	}
	col, err := opts.Adapter.Collect(ctx, &adapter.CollectInput{Task: t, Workspace: ws, ConfigDir: cfg, NativeLogPath: ro.NativeLogPath, Seed: seed, Run: ro})
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
	if col.ToolSequence != nil {
		row.ToolSequence = col.ToolSequence
	}
	row.Drift.Repeat = repeats(col.ToolSequence)
	row.Outcome = col.Outcome
	if col.OutcomeReason != "" {
		row.OutcomeReason = strp(col.OutcomeReason)
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
	claimed, why := ClaimedDone(row.Outcome, col.FinalMessage)
	row.ClaimedDone, row.ClaimedDoneReason = &claimed, &why

	// Diff, grade on a clean checkout, scan.
	diff, err := task.Diff(ctx, ws)
	if err != nil {
		return infra("diff", err)
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
		// The oracle fails every patch by construction; the run passes when
		// the harness reached the expected terminal.
		row.Oracle.Pass = row.Outcome == "abandon" && (t.Terminal == nil || mentionsAll(col.FinalMessage, t.Terminal.ReasonMustMention))
	}
	row.Scan = task.Scan(diff, t.ScanOptions())
	var oracleText []byte
	if oracle != nil {
		oracleText = oracle.Text()
	} else {
		oracleText = []byte(fmt.Sprintf("--- not graded: %s\n--- exit -1\n", deref(row.Oracle.ApplyError)))
	}
	writeArchive(runDir, &row, disclosure, col.TraceJSONL, oracleText, diff, col.FinalMessage)
	return row
}

func mentionsAll(msg string, needles []string) bool {
	for _, n := range needles {
		if !strings.Contains(msg, n) {
			return false
		}
	}
	return true
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
var artifactKeys = map[string]string{"trace.jsonl": "trace", "harness.json": "harness", "workspace.diff": "diff", "oracle.txt": "oracle", "scan.json": "scan", "final_message.txt": "final_message"}

// writeArchive writes the section 3.4 files and SHA256SUMS, then run.json
// with the artifact hashes.
func writeArchive(dir string, row *Row, disclosure adapter.Disclosure, traceJSONL, oracleText, diff []byte, finalMessage string) {
	write := func(name string, b []byte) {
		if b == nil {
			b = []byte{}
		}
		os.WriteFile(filepath.Join(dir, name), b, 0o644)
		row.Artifacts[artifactKeys[name]] = adapter.BytesSHA256(b)
	}
	write("trace.jsonl", traceJSONL)
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
	names := []string{"trace.jsonl", "harness.json", "workspace.diff", "oracle.txt", "scan.json", "final_message.txt"}
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
