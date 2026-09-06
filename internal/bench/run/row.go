package run

import (
	"encoding/json"

	"github.com/ddh4r4m/saga/internal/bench/adapter"
	"github.com/ddh4r4m/saga/internal/bench/task"
	"github.com/ddh4r4m/saga/internal/schema"
	"github.com/ddh4r4m/saga/internal/trace"
)

// RowSchema is the run row schema id (bench-spec section 9.3).
const RowSchema = "saga.bench.run/1"

// ManifestSchema is the manifest schema id (section 8.1).
const ManifestSchema = "saga.bench.manifest/1"

// Usage is the contracts section 7.2 object as stored in a row.
type Usage struct {
	InputFresh      int     `json:"input_fresh"`
	CacheRead       int     `json:"cache_read"`
	CacheWrite5m    int     `json:"cache_write_5m"`
	CacheWrite1h    int     `json:"cache_write_1h"`
	Output          int     `json:"output"`
	Reasoning       *int    `json:"reasoning"`
	ReasoningReason *string `json:"reasoning_reason,omitempty"`
	Source          string  `json:"source"`
}

// UsageFrom converts the trace usage object.
func UsageFrom(u trace.Usage) Usage {
	src := u.Source
	if src == "" {
		src = "unknown"
	}
	return Usage{InputFresh: u.InputFresh, CacheRead: u.CacheRead, CacheWrite5m: u.CacheWrite5m, CacheWrite1h: u.CacheWrite1h, Output: u.Output, Reasoning: u.Reasoning, ReasoningReason: u.ReasoningReason, Source: src}
}

// Total is every token the run consumed.
func (u Usage) Total() int {
	t := u.InputFresh + u.CacheRead + u.CacheWrite5m + u.CacheWrite1h + u.Output
	if u.Reasoning != nil {
		t += *u.Reasoning
	}
	return t
}

// OracleRow is the graded result.
type OracleRow struct {
	Exit        int               `json:"exit"`
	Tests       map[string]string `json:"tests"`
	Pass        bool              `json:"pass"`
	Regressed   []string          `json:"regressed"`
	CeilingBand []float64         `json:"ceiling_band"`
	ApplyError  *string           `json:"apply_error"`
	// Integrity is the section 5.8 probe verdict; Pass requires it to be
	// "ok", because the oracle shares its interpreter with the agent's code.
	Integrity       string `json:"integrity"`
	IntegrityReason string `json:"integrity_reason,omitempty"`
}

// Drift is the section 5.9 event counts.
type Drift struct {
	Repeat             int `json:"repeat"`
	EditFailStreak     int `json:"edit_fail_streak"`
	Oscillation        int `json:"oscillation"`
	OutOfScopeRead     int `json:"out_of_scope_read"`
	LateScopeExpansion int `json:"late_scope_expansion"`
}

// Row is one saga.bench.run/1 record.
type Row struct {
	Schema   string `json:"schema"`
	Manifest string `json:"manifest"`
	Task     string `json:"task"`
	Model    string `json:"model"`
	Harness  string `json:"harness"`
	Arm      string `json:"arm"`
	I        int    `json:"i"`
	// Sequence is this run's 1-based position in the invocation's
	// execution order across arms and tasks, so the interleaving is a
	// recorded fact rather than an inference (docs/12 row 4).
	Sequence      int     `json:"sequence"`
	Seed          string  `json:"seed"`
	Outcome       string  `json:"outcome"`
	OutcomeReason *string `json:"outcome_reason"`
	// AbandonReasonClass is the reason class of an ABANDON terminal
	// (adapter.ReasonClasses or "unclassified"); null otherwise.
	AbandonReasonClass *string `json:"abandon_reason_class"`
	// Pins is the trace-spec section 4.1 record observed for this run;
	// NonComparable marks a run whose served model differs from the
	// requested one (trace-spec section 4.2, gemini-cli #28859 class).
	Pins                  *trace.Pins        `json:"pins"`
	NonComparable         bool               `json:"non_comparable"`
	NonComparableReason   *string            `json:"non_comparable_reason"`
	ClaimedDone           *bool              `json:"claimed_done"`
	ClaimedDoneReason     *string            `json:"claimed_done_reason"`
	ClaimedDoneStructural *bool              `json:"claimed_done_structural"`
	ClaimVerdict          *string            `json:"claim_verdict"`
	Claims                int                `json:"claims"`
	Oracle                OracleRow          `json:"oracle"`
	Scan                  task.ScanResult    `json:"scan"`
	Usage                 Usage              `json:"usage"`
	CostUSD               *float64           `json:"cost_usd"`
	CostUSDReason         *string            `json:"cost_usd_reason"`
	WallS                 float64            `json:"wall_s"`
	Turns                 int                `json:"turns"`
	ToolCalls             int                `json:"tool_calls"`
	Drift                 Drift              `json:"drift"`
	Compliance            []any              `json:"compliance"`
	BlockedReachAttempts  int                `json:"blocked_reach_attempts"`
	ComponentUsed         *bool              `json:"component_used"`
	ToolSequence          []adapter.ToolCall `json:"tool_sequence"`
	Artifacts             map[string]string  `json:"artifacts"`
}

// Validate checks the row against the embedded schema.
func (r *Row) Validate() error {
	v, err := schema.Normalize(r)
	if err != nil {
		return err
	}
	return schema.ValidateID(RowSchema, v)
}

// Manifest is the saga.bench.manifest/1 record.
type Manifest struct {
	Schema                string       `json:"schema"`
	Created               string       `json:"created"`
	Tier                  string       `json:"tier"`
	BenchVersion          BenchVersion `json:"bench_version"`
	PreregistrationSHA256 *string      `json:"preregistration_sha256"`
	TaskSet               TaskSet      `json:"task_set"`
	Arms                  []Arm        `json:"arms"`
	Models                []Model      `json:"models"`
	Harnesses             []HarnessRef `json:"harnesses"`
	K                     int          `json:"k"`
	RunSeed               string       `json:"run_seed"`
	BootstrapSeed         int64        `json:"bootstrap_seed"`
	PriceTableSHA256      *string      `json:"price_table_sha256"`
	AbstainListSHA256     string       `json:"abstain_list_sha256"`
	ClaimsListSHA256      string       `json:"claims_list_sha256"`
	// AbandonLexiconSHA256 pins the closed reason lexicon that classed
	// this run's ABANDON terminals (docs/12 row 16).
	AbandonLexiconSHA256 string `json:"abandon_lexicon_sha256"`
	// Interleaving names the execution order across arms; the rows'
	// sequence numbers reconstruct it exactly (docs/12 row 4).
	Interleaving string            `json:"interleaving"`
	Isolation    string            `json:"isolation"`
	Images       map[string]string `json:"images"`
	Host         Host              `json:"host"`
	Budget       Budget            `json:"budget"`
}

// BenchVersion identifies the bench binary.
type BenchVersion struct {
	Git          *string `json:"git"`
	BinarySHA256 *string `json:"binary_sha256"`
}

// TaskSet lists the tasks by content hash.
type TaskSet struct {
	SHA256 string     `json:"sha256"`
	Tasks  []TaskHash `json:"tasks"`
}

// TaskHash is one task's identity.
type TaskHash struct {
	ID         string  `json:"id"`
	SHA256     string  `json:"sha256"`
	VerifiedAt *string `json:"verified_at"`
}

// Arm is one arm of the design.
type Arm struct {
	ID              string   `json:"id"`
	Components      []string `json:"components"`
	BlocksInControl []string `json:"blocks_in_control,omitempty"`
}

// Model is a model under test.
type Model struct {
	ID             string  `json:"id"`
	Snapshot       *string `json:"snapshot"`
	TrainingCutoff *string `json:"training_cutoff"`
}

// HarnessRef is a harness under test.
type HarnessRef struct {
	Name          string  `json:"name"`
	Version       *string `json:"version"`
	AdapterSHA256 *string `json:"adapter_sha256"`
}

// Host is where the bench ran.
type Host struct {
	OS     string  `json:"os"`
	Arch   string  `json:"arch"`
	Kernel *string `json:"kernel"`
}

// Budget is the section 4.5 estimate and cap.
type Budget struct {
	EstimateUSD float64  `json:"estimate_usd"`
	CapUSD      float64  `json:"cap_usd"`
	SpentUSD    *float64 `json:"spent_usd"`
}

// Validate checks the manifest against the embedded schema.
func (m *Manifest) Validate() error {
	v, err := schema.Normalize(m)
	if err != nil {
		return err
	}
	return schema.ValidateID(ManifestSchema, v)
}

// ReadRows parses rows.jsonl.
func ReadRows(raw []byte) ([]Row, error) {
	var rows []Row
	dec := json.NewDecoder(bytesReader(raw))
	for dec.More() {
		var r Row
		if err := dec.Decode(&r); err != nil {
			return rows, err
		}
		rows = append(rows, r)
	}
	return rows, nil
}
