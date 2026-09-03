package schema

import (
	"strings"
	"testing"
)

func validEnvelope() map[string]any {
	return map[string]any{
		"schema": "saga.trace/1", "seq": 1.0, "ts": "2026-09-03T00:00:00.000Z", "mono_ns": 0.0,
		"session": "s", "turn": 0.0, "agent": "main", "type": "session", "source": "hook:SessionStart",
		"body": map[string]any{}, "prev": "sha256:genesis",
		"hash": "sha256:" + strings.Repeat("0", 64),
	}
}

func TestEnvelopeValidation(t *testing.T) {
	if err := ValidateID("saga.trace/1", validEnvelope()); err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}
	bad := validEnvelope()
	bad["extra"] = 1
	if err := ValidateID("saga.trace/1", bad); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("unknown field accepted: %v", err)
	}
	bad = validEnvelope()
	bad["type"] = "banana"
	if err := ValidateID("saga.trace/1", bad); err == nil {
		t.Error("bad type accepted")
	}
	bad = validEnvelope()
	bad["seq"] = 0.0
	if err := ValidateID("saga.trace/1", bad); err == nil {
		t.Error("seq 0 accepted")
	}
	bad = validEnvelope()
	bad["prev"] = "sha256:xyz"
	if err := ValidateID("saga.trace/1", bad); err == nil {
		t.Error("bad prev accepted")
	}
	bad = validEnvelope()
	delete(bad, "hash")
	if err := ValidateID("saga.trace/1", bad); err == nil {
		t.Error("missing hash accepted")
	}
	bad = validEnvelope()
	bad["source"] = "hook:"
	if err := ValidateID("saga.trace/1", bad); err == nil {
		t.Error("bad source accepted")
	}
}

func TestSplitID(t *testing.T) {
	name, major, err := SplitID("saga.trace.ledger/1")
	if err != nil || name != "saga.trace.ledger" || major != 1 {
		t.Errorf("%s %d %v", name, major, err)
	}
	if _, _, err := SplitID("saga.trace"); err == nil {
		t.Error("missing major accepted")
	}
}

func TestEveryRegisteredSchemaLoads(t *testing.T) {
	for id, file := range Registry {
		if _, err := Load(file); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
	for _, typ := range []string{"session", "turn", "model_call", "tool_call", "tool_result", "edit", "gate", "guard", "mem_inject", "compaction", "subagent", "budget", "drift", "checkpoint", "canary", "route_decision"} {
		if _, err := Load(BodyFile(typ)); err != nil {
			t.Errorf("body %s: %v", typ, err)
		}
	}
}
