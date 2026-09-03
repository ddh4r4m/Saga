package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddh4r4m/saga/internal/hookio"
)

func TestInstallDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	file, result, entries, err := Install(dir, "/usr/local/bin/saga", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("dry run wrote the settings file")
	}
	if len(entries) != len(hookio.Events) {
		t.Errorf("entries %d", len(entries))
	}
	var s map[string]any
	if err := json.Unmarshal(result, &s); err != nil {
		t.Fatal(err)
	}
	hooks := s["hooks"].(map[string]any)
	pre := hooks["PreToolUse"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if pre["command"] != "/usr/local/bin/saga hook claude-code PreToolUse" || pre["timeout"].(float64) != Timeout {
		t.Errorf("%v", pre)
	}
}

func TestInstallPreservesForeignHooksAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	file := SettingsFile(dir, false)
	os.MkdirAll(filepath.Dir(file), 0o755)
	existing := `{"permissions":{"allow":["Bash(ls)"]},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"afplay x.aiff"}]}]}}`
	os.WriteFile(file, []byte(existing), 0o644)
	for i := 0; i < 2; i++ {
		if _, _, _, err := Install(dir, "/bin/saga", false, false); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := os.ReadFile(file)
	var s map[string]any
	json.Unmarshal(raw, &s)
	if s["permissions"] == nil {
		t.Error("permissions dropped")
	}
	stop := s["hooks"].(map[string]any)["Stop"].([]any)
	if len(stop) != 2 {
		t.Errorf("Stop entries %d, want foreign + ours once", len(stop))
	}
	if !strings.Contains(string(raw), "afplay x.aiff") {
		t.Error("foreign hook lost")
	}
	regs := Registration([]string{file})
	for _, r := range regs {
		if !r.Registered || r.File != file {
			t.Errorf("%s not registered", r.Event)
		}
	}
	regs = Registration([]string{filepath.Join(dir, "missing.json")})
	for _, r := range regs {
		if r.Registered {
			t.Errorf("%s registered from a missing file", r.Event)
		}
	}
}
