package trace

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/store"
)

// PricesSchema is the schema id of a price table (contracts section 11).
const PricesSchema = "saga.trace.prices/1"

//go:embed prices/default.toml
var defaultPricesTOML []byte

// PriceTable is saga.trace.prices/1: USD per million tokens per model id,
// addressed by the sha256 of its file bytes.
type PriceTable struct {
	Schema   string       `toml:"schema"`
	Observed string       `toml:"observed"`
	Models   []ModelPrice `toml:"model"`
	// CalibrationModel is the model the bench corpus's `cost_hint_usd`
	// values were measured on (docs/12 section 6). A bench estimate for
	// another model scales each hint by the price ratio to this one.
	CalibrationModel string `toml:"calibration_model"`
	// Hash is sha256 of the file bytes; pinned into every ledger row.
	Hash string `toml:"-"`
	raw  []byte
}

// ModelPrice is one row of the table.
type ModelPrice struct {
	ID             string       `toml:"id"`
	Vendor         string       `toml:"vendor"`
	TrainingCutoff string       `toml:"training_cutoff"`
	In             *float64     `toml:"in"`
	Out            *float64     `toml:"out"`
	CacheRead      *float64     `toml:"cache_read"`
	CacheWrite5m   *float64     `toml:"cache_write_5m"`
	CacheWrite1h   *float64     `toml:"cache_write_1h"`
	LongContext    *LongContext `toml:"long_context"`
	OpenWeight     bool         `toml:"open_weight"`
	Source         string       `toml:"source"`
	Note           string       `toml:"note"`
	// CacheWrite5mReason records why a 5m cache-write price is an
	// assumption rather than a measurement: Claude Code writes 1h cache,
	// so the component is zero in every observation and its price is not
	// identified by a fit.
	CacheWrite5mReason string `toml:"cache_write_5m_reason"`
}

// LongContext is the tiered rate above a context size (Gemini style).
type LongContext struct {
	Over int      `toml:"over"`
	In   *float64 `toml:"in"`
	Out  *float64 `toml:"out"`
}

// ParsePrices parses a price table file and computes its hash.
func ParsePrices(raw []byte) (*PriceTable, error) {
	var t PriceTable
	if _, err := toml.Decode(string(raw), &t); err != nil {
		return nil, fmt.Errorf("price table: %w", err)
	}
	if t.Schema != PricesSchema {
		return nil, fmt.Errorf("price table: schema %q, want %s", t.Schema, PricesSchema)
	}
	t.Hash = canon.SHA256(raw)
	t.raw = raw
	return &t, nil
}

// DefaultPrices returns the table shipped in the binary.
func DefaultPrices() *PriceTable {
	t, err := ParsePrices(defaultPricesTOML)
	if err != nil {
		panic("embedded price table invalid: " + err.Error())
	}
	return t
}

// Lookup returns the row for a model id, or nil when the model is
// unpriced.
func (t *PriceTable) Lookup(model string) *ModelPrice {
	for i := range t.Models {
		if t.Models[i].ID == model {
			return &t.Models[i]
		}
	}
	return nil
}

// perMTok is the headline price of a model, input plus output per
// million tokens. It is the figure a cost hint scales with: the cache
// components move with it and a bench estimate is an order-of-magnitude
// guard, not an invoice.
func (m *ModelPrice) perMTok() (float64, bool) {
	if m == nil || m.In == nil || m.Out == nil {
		return 0, false
	}
	return *m.In + *m.Out, true
}

// CostRatio is how much a run on model costs relative to the model the
// corpus's cost hints were calibrated on. It returns 1.0 and false when
// either model is absent from the table or carries no price, which the
// caller reports rather than hides: an unpriced model keeps the
// Opus-calibrated hint.
//
// The bench needed this because its estimate is the only thing standing
// between an operator and a spend, and it was model-blind: a Haiku cell
// over the twenty pilot tasks at K=5 priced at 48.50 usd, the Opus
// figure, and the user tier's 20 usd cap refused a run that would have
// cost about a tenth of that (2026-09-13).
func (t *PriceTable) CostRatio(model string) (float64, bool) {
	if t == nil || model == "" {
		return 1, false
	}
	cal := t.CalibrationModel
	if cal == "" || model == cal {
		return 1, cal != "" && model == cal
	}
	m, ok := t.Lookup(model).perMTok()
	c, okc := t.Lookup(cal).perMTok()
	if !ok || !okc || c == 0 {
		return 1, false
	}
	return m / c, true
}

// Persist writes the table to .saga/trace/prices/<sha256 hex>.toml if it is
// not already there, so every table ever used is kept (trace-spec
// section 2.7), and returns the path.
func (t *PriceTable) Persist(s *store.Store) (string, error) {
	dir := s.Path("trace", "prices")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, t.Hash[len("sha256:"):]+".toml")
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	return p, store.WriteFileAtomic(p, t.raw, 0o600)
}

// LoadPrices returns the table pinned for this store: the file named by
// .saga/trace/prices/current, else the shipped default. The chosen table
// is persisted under its hash.
func LoadPrices(s *store.Store) (*PriceTable, error) {
	cur := s.Path("trace", "prices", "current")
	if name, err := os.ReadFile(cur); err == nil {
		p := s.Path("trace", "prices", string(name))
		if err := store.CheckShape(p); err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("pinned price table: %w", err)
		}
		return ParsePrices(raw)
	}
	t := DefaultPrices()
	if s != nil {
		if _, err := t.Persist(s); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// UsePrices installs a price table file as the pinned table
// (`saga trace prices use <file>`).
func UsePrices(s *store.Store, file string) (*PriceTable, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	t, err := ParsePrices(raw)
	if err != nil {
		return nil, err
	}
	p, err := t.Persist(s)
	if err != nil {
		return nil, err
	}
	return t, store.WriteFileAtomic(s.Path("trace", "prices", "current"), []byte(filepath.Base(p)), 0o600)
}

// USD is the priced breakdown of one call. A nil component means unpriced.
type USD struct {
	Input      *float64 `json:"input"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
	Output     *float64 `json:"output"`
	Total      *float64 `json:"total"`
}

func perM(tokens int, rate *float64) *float64 {
	if rate == nil {
		if tokens == 0 {
			z := 0.0
			return &z
		}
		return nil
	}
	v := float64(tokens) * *rate / 1e6
	return &v
}

// Cost prices a usage object against the table. The returned reason is
// non-empty when any component is unpriced; total is then nil, never 0
// (trace-spec section 3.3).
func (t *PriceTable) Cost(model string, u Usage) (USD, string) {
	row := t.Lookup(model)
	if row == nil {
		return USD{}, "model " + model + " not in price table " + t.Hash
	}
	in, out := row.In, row.Out
	ctx := u.InputFresh + u.CacheRead + u.CacheWrite5m + u.CacheWrite1h
	if row.LongContext != nil && ctx > row.LongContext.Over {
		if row.LongContext.In != nil {
			in = row.LongContext.In
		}
		if row.LongContext.Out != nil {
			out = row.LongContext.Out
		}
	}
	usd := USD{
		Input:     perM(u.InputFresh, in),
		CacheRead: perM(u.CacheRead, row.CacheRead),
		Output:    perM(u.Output, out),
	}
	w5 := perM(u.CacheWrite5m, row.CacheWrite5m)
	w1 := perM(u.CacheWrite1h, row.CacheWrite1h)
	if w5 != nil && w1 != nil {
		v := *w5 + *w1
		usd.CacheWrite = &v
	}
	var reason string
	switch {
	case usd.Input == nil:
		reason = "input unpriced"
	case usd.CacheRead == nil:
		reason = "cache_read unpriced"
	case usd.CacheWrite == nil:
		reason = "cache_write unpriced"
	case usd.Output == nil:
		reason = "output unpriced"
	default:
		v := *usd.Input + *usd.CacheRead + *usd.CacheWrite + *usd.Output
		usd.Total = &v
	}
	if reason != "" {
		reason += " for " + model
	}
	return usd, reason
}
