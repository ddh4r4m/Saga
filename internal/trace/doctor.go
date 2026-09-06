package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/schema"
	"github.com/ddh4r4m/saga/internal/store"
)

// DoctorSchema is the schema id of the doctor report.
const DoctorSchema = "saga.doctor/1"

// Check is one doctor line.
type Check struct {
	ID     string  `json:"id"`
	OK     bool    `json:"ok"`
	Detail string  `json:"detail"`
	Fix    *string `json:"fix"`
}

// DoctorReport is saga.doctor/1 (trace-spec section 7).
type DoctorReport struct {
	Schema    string         `json:"schema"`
	TS        string         `json:"ts"`
	OK        bool           `json:"ok"`
	Checks    []Check        `json:"checks"`
	Binary    map[string]any `json:"binary"`
	Pins      any            `json:"pins"`
	Cache     any            `json:"cache"`
	Drift7d   any            `json:"drift_7d"`
	Budget    any            `json:"budget"`
	Canary    any            `json:"canary"`
	Incidents []any          `json:"incidents"`
	Precision any            `json:"precision"`
}

// HookRegistration is one event's registration status, computed by the
// harness adapter from the settings files without modifying them.
type HookRegistration struct {
	Event      string
	Registered bool
	File       string
	Command    string
}

// DoctorInput is what the CLI gathers for Doctor.
type DoctorInput struct {
	Version string
	// Store may point at a directory without .saga; Doctor reports it.
	Store *store.Store
	// HarnessName and HarnessFound describe the harness binary on PATH;
	// InstallHarness is the `saga install --harness` id.
	HarnessName    string
	InstallHarness string
	HarnessFound   bool
	HarnessVersion string
	Hooks          []HookRegistration
	SettingsFiles  []string
	// Session names the recorded session the hooks_fire check reads;
	// empty means the newest, which is what a probe wants right after
	// driving one turn (trace-spec section 7, docs/12 row 10).
	Session string
}

func fixp(s string) *string { return &s }

// Doctor runs the M0 checks: go version, binary hash, .saga presence,
// config, hook registration, usage source, pins, price table, harness
// presence. Exit is 0 when all pass, 1 when a check fails, 6 when the
// harness is not found (trace-spec section 7).
func Doctor(in DoctorInput) (DoctorReport, cli.Code) {
	r := DoctorReport{Schema: DoctorSchema, TS: FormatTS(time.Now()), Incidents: []any{}}
	r.Binary = map[string]any{"version": in.Version, "go": runtime.Version(), "sha256": nil, "sha256_reason": nil, "path": nil}
	add := func(id string, ok bool, detail string, fix *string) {
		r.Checks = append(r.Checks, Check{ID: id, OK: ok, Detail: detail, Fix: fix})
	}
	add("go_version", true, runtime.Version()+" "+runtime.GOOS+"/"+runtime.GOARCH, nil)
	if exe, err := os.Executable(); err == nil {
		r.Binary["path"] = exe
		if h, err := fileSHA256(exe); err == nil {
			r.Binary["sha256"] = h
			add("binary", true, fmt.Sprintf("saga %s %s", in.Version, h[:19]), nil)
		} else {
			r.Binary["sha256_reason"] = err.Error()
			add("binary", false, "cannot hash binary: "+err.Error(), nil)
		}
	} else {
		r.Binary["sha256_reason"] = err.Error()
		add("binary", false, "cannot locate binary: "+err.Error(), nil)
	}

	code := cli.ExitOK
	if in.HarnessName != "" {
		if in.HarnessFound {
			add("harness_present", true, in.HarnessName+" "+in.HarnessVersion, nil)
		} else {
			add("harness_present", false, in.HarnessName+" not found on PATH", fixp("install "+in.HarnessName+" or run saga doctor from a shell where it is on PATH"))
			code = cli.ExitEnvironment
		}
	}

	st := in.Store
	if st == nil || !st.Exists() {
		add("saga_dir", false, ".saga/.gitignore missing", fixp("run saga init in the repository root"))
	} else {
		add("saga_dir", true, st.Dir(), nil)
		if _, err := st.Config(); err != nil {
			add("config", false, err.Error(), fixp("fix .saga/config.toml"))
		} else if st.HasConfig() {
			add("config", true, ".saga/config.toml parses (full mode)", nil)
		} else {
			add("config", true, ".saga/config.toml absent (minimal mode)", nil)
		}
	}

	var missing []string
	registered := 0
	for _, h := range in.Hooks {
		if h.Registered {
			registered++
		} else {
			missing = append(missing, h.Event)
		}
	}
	if len(in.Hooks) > 0 {
		if len(missing) == 0 {
			add("hooks_registered", true, fmt.Sprintf("%d/%d events bound in %s", registered, len(in.Hooks), strings.Join(in.SettingsFiles, ", ")), nil)
		} else {
			add("hooks_registered", false, fmt.Sprintf("%s hook missing in %s", strings.Join(missing, ", "), strings.Join(in.SettingsFiles, ", ")), fixp("run saga install --harness "+in.InstallHarness))
		}
	}
	// hooks_fire: a registered hook is not a firing hook. The check reads
	// what the session actually recorded, so one live turn proves the
	// chain end to end (docs/12 row 10).
	if st != nil && st.Exists() {
		ok, detail := hooksFire(st, in.Session)
		add("hooks_fire", ok, detail, fixp("drive one turn with the hooks installed, then rerun doctor in that workspace"))
	} else {
		add("hooks_fire", false, "no .saga store: nothing recorded to check", nil)
	}

	if st != nil && st.Exists() {
		sessions, _ := ListSessions(st)
		usageOK, usageDetail := usageSource(st, sessions)
		add("usage_source", usageOK, usageDetail, fixp("check transcript_path is readable; trace-spec section 9.3"))
		if pins, ok, err := ReadPins(st); err != nil {
			add("pins", false, err.Error(), nil)
		} else if !ok {
			add("pins", false, "pins/current.json absent: no session recorded yet", nil)
		} else {
			r.Pins = pins
			add("pins", true, fmt.Sprintf("pins/current.json present (saga %v)", pins.Saga["version"]), nil)
		}
		if t, err := LoadPrices(st); err != nil {
			add("price_table", false, err.Error(), nil)
		} else {
			add("price_table", true, fmt.Sprintf("%s observed %s (%d models)", t.Hash[:19], t.Observed, len(t.Models)), nil)
		}
		if cfg, err := st.Config(); err == nil {
			b := map[string]any{"session_limit": cfg.Trace.Budget.SessionUSD}
			if len(sessions) > 0 {
				rows, _ := ReadLedger(SessionDir(st, sessions[0]))
				if len(rows) > 0 {
					b["session_usd"] = rows[len(rows)-1].CumUSD
					b["session"] = sessions[0]
				}
			}
			r.Budget = b
		}
		add("retention", true, fmt.Sprintf("%d sessions under .saga/trace; prune ships in M0 step 2", len(sessions)), nil)
	}

	r.OK = true
	for _, c := range r.Checks {
		if !c.OK {
			r.OK = false
		}
	}
	if !r.OK && code == cli.ExitOK {
		code = cli.ExitFinding
	}
	return r, code
}

func usageSource(st *store.Store, sessions []string) (bool, string) {
	if len(sessions) == 0 {
		return false, "no sessions recorded yet"
	}
	n := 0
	good := 0
	for _, s := range sessions {
		if n == 5 {
			break
		}
		rows, err := ReadLedger(SessionDir(st, s))
		if err != nil || len(rows) == 0 {
			continue
		}
		n++
		if rows[len(rows)-1].Usage.Source != "estimated" {
			good++
		}
	}
	if n == 0 {
		return false, "no ledger rows in the last sessions: transcript not read"
	}
	return good == n, fmt.Sprintf("%d/%d recent sessions have usage.source != estimated", good, n)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// Validate checks the report against saga.doctor/1.
func (r DoctorReport) Validate() error {
	v, err := schema.Normalize(r)
	if err != nil {
		return err
	}
	return schema.ValidateID(DoctorSchema, v)
}

// Text renders one line per check.
func (r DoctorReport) Text() string {
	var b strings.Builder
	for _, c := range r.Checks {
		mark := "ok  "
		if !c.OK {
			mark = "FAIL"
		}
		fmt.Fprintf(&b, "%s %-18s %s\n", mark, c.ID, c.Detail)
		if !c.OK && c.Fix != nil {
			fmt.Fprintf(&b, "     fix: %s\n", *c.Fix)
		}
	}
	return b.String()
}

// hooksFire reports whether a recorded session shows the chain working:
// at least one tool_call, its tool_result, and the Stop claim event. The
// session is the newest unless one is named, because the caller that
// needs this has just driven a single turn.
func hooksFire(st *store.Store, session string) (bool, string) {
	if session == "" {
		sessions, err := ListSessions(st)
		if err != nil || len(sessions) == 0 {
			return false, "no recorded session under .saga/trace/sessions"
		}
		session = sessions[0]
	}
	events, err := ReadAll(SessionDir(st, session))
	if err != nil {
		return false, "session " + session + ": " + err.Error()
	}
	calls, results, claim := 0, 0, ""
	for _, ev := range events {
		switch ev.Type {
		case TypeToolCall:
			calls++
		case TypeToolResult:
			results++
		case TypeGate:
			if ev.Body["kind"] == "claim" {
				if v, ok := ev.Body["verdict"].(string); ok {
					claim = v
				} else {
					claim = "no-claims"
				}
			}
		}
	}
	detail := fmt.Sprintf("session %s: %d tool_call, %d tool_result, stop claim %q", session, calls, results, claim)
	switch {
	case calls == 0:
		return false, detail + " (no tool_call recorded: the PreToolUse hook did not fire)"
	case results == 0:
		return false, detail + " (no tool_result recorded: the PostToolUse hook did not fire)"
	case claim == "":
		return false, detail + " (no Stop claim event: the Stop hook did not fire)"
	}
	return true, detail
}
