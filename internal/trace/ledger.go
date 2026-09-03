package trace

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddh4r4m/saga/internal/schema"
)

// LedgerSchema is the schema id of a ledger row.
const LedgerSchema = "saga.trace.ledger/1"

// LedgerRow is one derived row of ledger.jsonl (trace-spec section 3.2),
// one per model_call.
type LedgerRow struct {
	Schema                 string             `json:"schema"`
	Session                string             `json:"session"`
	Turn                   int                `json:"turn"`
	Seq                    int                `json:"seq"`
	Model                  string             `json:"model"`
	Usage                  Usage              `json:"usage"`
	PriceTable             *string            `json:"price_table"`
	USD                    USD                `json:"usd"`
	USDReason              *string            `json:"usd_reason,omitempty"`
	ContextTokens          int                `json:"context_tokens"`
	ContextDelta           int                `json:"context_delta"`
	ToolOutputBytesTurn    int                `json:"tool_output_bytes_turn"`
	ToolOutputBytesRawTurn int                `json:"tool_output_bytes_raw_turn"`
	Attribution            map[string]float64 `json:"attribution"`
	ResidentTokens         map[string]int     `json:"resident_tokens"`
	CacheHitRatio          *float64           `json:"cache_hit_ratio"`
	TTLInferred            *string            `json:"ttl_inferred"`
	CumUSD                 float64            `json:"cum_usd"`
	MixedPriceTables       bool               `json:"mixed_price_tables,omitempty"`
}

// RowInput is what BuildRow needs beyond the usage itself.
type RowInput struct {
	Session             string
	Turn                int
	Seq                 int
	Model               string
	Usage               Usage
	Prices              *PriceTable
	PrevContextGrowth   int
	ToolOutputBytesTurn int
	// Estimated tokens added to this turn's context by origin, used for the
	// section 3.5 attribution estimate. Keys are attribution keys.
	OriginTokens map[string]int
	CumUSD       float64
}

// BuildRow derives a ledger row. It never turns an unpriced call into 0:
// usd fields stay null and cum_usd is unchanged.
func BuildRow(in RowInput) LedgerRow {
	usd, reason := in.Prices.Cost(in.Model, in.Usage)
	row := LedgerRow{
		Schema: LedgerSchema, Session: in.Session, Turn: in.Turn, Seq: in.Seq, Model: in.Model,
		Usage: in.Usage, USD: usd,
		ContextTokens:          in.Usage.ContextTokens(),
		ContextDelta:           in.Usage.Growth() - in.PrevContextGrowth,
		ToolOutputBytesTurn:    in.ToolOutputBytesTurn,
		ToolOutputBytesRawTurn: in.ToolOutputBytesTurn,
		Attribution:            Attribute(in.OriginTokens, in.Usage.Growth()-in.PrevContextGrowth),
		ResidentTokens:         map[string]int{"index": 0},
		CacheHitRatio:          in.Usage.CacheHitRatio(),
		CumUSD:                 in.CumUSD,
	}
	pt := in.Prices.Hash
	row.PriceTable = &pt
	if reason != "" {
		row.USDReason = &reason
	}
	if usd.Total != nil {
		row.CumUSD = round6(in.CumUSD + *usd.Total)
	}
	return row
}

func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }

// Attribute turns estimated tokens per origin into shares that sum to 1,
// scaled to the observed context delta; whatever the origins do not
// explain is the harness's share (trace-spec section 3.5). Every key of
// the closed set is present.
func Attribute(origin map[string]int, delta int) map[string]float64 {
	shares := ZeroAttribution()
	sum := 0
	for _, v := range origin {
		if v > 0 {
			sum += v
		}
	}
	if delta <= 0 || sum == 0 {
		shares["harness"] = 1
		return shares
	}
	total := float64(delta)
	if sum > delta {
		total = float64(sum)
	}
	explained := 0.0
	for k, v := range origin {
		if v <= 0 {
			continue
		}
		s := math.Round(float64(v)/total*1e4) / 1e4
		shares[k] = s
		explained += s
	}
	rest := 1 - explained
	if rest < 0 {
		rest = 0
	}
	shares["harness"] = math.Round((shares["harness"]+rest)*1e4) / 1e4
	return shares
}

// Validate checks the row against saga.trace.ledger/1.
func (r *LedgerRow) Validate() error {
	v, err := schema.Normalize(r)
	if err != nil {
		return err
	}
	return schema.ValidateID(LedgerSchema, v)
}

// AppendLedger appends a validated row to <dir>/ledger.jsonl.
func AppendLedger(dir string, row LedgerRow) error {
	if err := row.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(row)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "ledger.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ReadLedger returns every row of <dir>/ledger.jsonl.
func ReadLedger(dir string) ([]LedgerRow, error) {
	f, err := os.Open(filepath.Join(dir, "ledger.jsonl"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rows []LedgerRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var r LedgerRow
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return rows, err
		}
		rows = append(rows, r)
	}
	return rows, sc.Err()
}

// Report is the saga.trace.report/1 summary of a session's ledger.
type Report struct {
	Schema        string             `json:"schema"`
	Session       string             `json:"session"`
	Calls         int                `json:"calls"`
	Turns         int                `json:"turns"`
	Models        []string           `json:"models"`
	Tokens        map[string]int     `json:"tokens"`
	USD           map[string]float64 `json:"usd"`
	TotalUSD      float64            `json:"total_usd"`
	Unpriced      int                `json:"unpriced_calls"`
	Estimated     int                `json:"estimated_calls"`
	PriceTables   []string           `json:"price_tables"`
	CacheHitRatio *float64           `json:"cache_hit_ratio"`
	ContextMin    int                `json:"context_min"`
	ContextMedian int                `json:"context_median"`
	ContextMax    int                `json:"context_max"`
	LargestDelta  int                `json:"largest_delta"`
	LargestTurn   int                `json:"largest_delta_turn"`
	Attribution   map[string]float64 `json:"attribution_usd"`
	Budget        *float64           `json:"budget_session_usd"`
}

// ReportSchema is the schema id of the report.
const ReportSchema = "saga.trace.report/1"

// Summarize builds the report from ledger rows.
func Summarize(session string, rows []LedgerRow, budget *float64) Report {
	r := Report{Schema: ReportSchema, Session: session, Tokens: map[string]int{}, USD: map[string]float64{}, Attribution: map[string]float64{}, Budget: budget}
	models := map[string]bool{}
	tables := map[string]bool{}
	turns := map[int]bool{}
	var ctxs []int
	readSum, inSum := 0, 0
	for _, row := range rows {
		r.Calls++
		turns[row.Turn] = true
		models[row.Model] = true
		if row.PriceTable != nil {
			tables[*row.PriceTable] = true
		}
		r.Tokens["input_fresh"] += row.Usage.InputFresh
		r.Tokens["cache_read"] += row.Usage.CacheRead
		r.Tokens["cache_write_5m"] += row.Usage.CacheWrite5m
		r.Tokens["cache_write_1h"] += row.Usage.CacheWrite1h
		r.Tokens["output"] += row.Usage.Output
		if row.Usage.Reasoning != nil {
			r.Tokens["reasoning"] += *row.Usage.Reasoning
		}
		if row.Usage.Source == "estimated" {
			r.Estimated++
		}
		if row.USD.Total == nil {
			r.Unpriced++
		} else {
			r.USD["input"] += deref(row.USD.Input)
			r.USD["cache_read"] += deref(row.USD.CacheRead)
			r.USD["cache_write"] += deref(row.USD.CacheWrite)
			r.USD["output"] += deref(row.USD.Output)
			r.TotalUSD += *row.USD.Total
			for k, s := range row.Attribution {
				r.Attribution[k] += s * *row.USD.Total
			}
		}
		ctxs = append(ctxs, row.ContextTokens)
		readSum += row.Usage.CacheRead
		inSum += row.ContextTokens
		if row.ContextDelta > r.LargestDelta {
			r.LargestDelta, r.LargestTurn = row.ContextDelta, row.Turn
		}
	}
	r.Turns = len(turns)
	for m := range models {
		r.Models = append(r.Models, m)
	}
	sort.Strings(r.Models)
	for t := range tables {
		r.PriceTables = append(r.PriceTables, t)
	}
	sort.Strings(r.PriceTables)
	for k, v := range r.USD {
		r.USD[k] = round6(v)
	}
	for k, v := range r.Attribution {
		r.Attribution[k] = round6(v)
	}
	r.TotalUSD = round6(r.TotalUSD)
	if inSum > 0 {
		ratio := round6(float64(readSum) / float64(inSum))
		r.CacheHitRatio = &ratio
	}
	if len(ctxs) > 0 {
		sort.Ints(ctxs)
		r.ContextMin, r.ContextMedian, r.ContextMax = ctxs[0], ctxs[len(ctxs)/2], ctxs[len(ctxs)-1]
	}
	return r
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// Text renders the report in the trace-spec section 3.7 layout.
func (r Report) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "session %s  %s  %d turns  %d model calls\n", r.Session, strings.Join(r.Models, ","), r.Turns, r.Calls)
	fmt.Fprintf(&b, "%-20s %14s %10s %8s\n", "", "tokens", "usd", "share")
	rowf := func(label, tok string, usd float64) {
		share := 0.0
		if r.TotalUSD > 0 {
			share = usd / r.TotalUSD * 100
		}
		fmt.Fprintf(&b, "%-20s %14s %10.4f %7.1f%%\n", label, tok, usd, share)
	}
	rowf("input (fresh)", commas(r.Tokens["input_fresh"]), r.USD["input"])
	rowf("cache read", commas(r.Tokens["cache_read"]), r.USD["cache_read"])
	rowf("cache write", commas(r.Tokens["cache_write_5m"]+r.Tokens["cache_write_1h"]), r.USD["cache_write"])
	rowf("output", commas(r.Tokens["output"]), r.USD["output"])
	if v, ok := r.Tokens["reasoning"]; ok {
		fmt.Fprintf(&b, "%-20s %14s %10s\n", "reasoning", commas(v), "n/a")
	}
	budget := ""
	if r.Budget != nil && *r.Budget > 0 {
		budget = fmt.Sprintf("  budget %.2f (%.0f%%)", *r.Budget, r.TotalUSD / *r.Budget * 100)
	}
	fmt.Fprintf(&b, "%-20s %14s %10.4f%s\n", "total", "", r.TotalUSD, budget)
	if r.Unpriced > 0 {
		fmt.Fprintf(&b, "unpriced calls: %d (not included in total)\n", r.Unpriced)
	}
	if r.Estimated > 0 {
		fmt.Fprintf(&b, "~ estimated calls: %d (no usage source)\n", r.Estimated)
	}
	if r.CacheHitRatio != nil {
		fmt.Fprintf(&b, "cache hit ratio  %.3f\n", *r.CacheHitRatio)
	}
	if r.Calls > 0 {
		fmt.Fprintf(&b, "per-call context  min %s  median %s  max %s  largest delta +%s (turn %d)\n", commas(r.ContextMin), commas(r.ContextMedian), commas(r.ContextMax), commas(r.LargestDelta), r.LargestTurn)
	}
	if len(r.PriceTables) > 0 {
		fmt.Fprintf(&b, "price table %s\n", strings.Join(r.PriceTables, ", "))
	}
	if len(r.Attribution) > 0 {
		b.WriteString("attribution (estimated)\n")
		keys := make([]string, 0, len(r.Attribution))
		for k := range r.Attribution {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if r.Attribution[k] > 0 {
				fmt.Fprintf(&b, "  %-18s %10.4f\n", k, r.Attribution[k])
			}
		}
	}
	return b.String()
}

func commas(n int) string {
	s := fmt.Sprint(n)
	if n < 0 {
		return "-" + commas(-n)
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
