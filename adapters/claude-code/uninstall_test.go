package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUninstallIsTheInverseOfInstall: uninstall must be able to prove
// what it would undo, and must not touch a hook it did not write
// (trace-spec section 7).
func TestUninstallIsTheInverseOfInstall(t *testing.T) {
	project := t.TempDir()
	file := SettingsFile(project, false)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	// A pre-existing foreign hook on an event Saga also uses, plus one on
	// an event it does not, so both survival paths are covered.
	seed := map[string]any{"hooks": map[string]any{
		"PreToolUse":   []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "/usr/bin/other-tool"}}}},
		"Notification": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "/usr/bin/notify"}}}},
	}}
	raw, _ := json.MarshalIndent(seed, "", "  ")
	if err := os.WriteFile(file, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, entries, err := Install(project, "/opt/saga", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("install reported no entries")
	}

	// Dry run lists exactly what install wrote and changes nothing.
	before, _ := os.ReadFile(file)
	_, removed, _, err := Uninstall(project, false, true)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(file)
	if string(before) != string(after) {
		t.Error("dry run wrote to the settings file")
	}
	want := map[string]bool{}
	for _, e := range entries {
		want["hook "+e.Event+" "+e.Command] = true
	}
	if len(removed) != len(want) {
		t.Errorf("dry run listed %d entries, install wrote %d:\n%s", len(removed), len(want), strings.Join(removed, "\n"))
	}
	for _, l := range removed {
		if !want[l] {
			t.Errorf("dry run lists %q, which install did not write", l)
		}
	}

	// The real run removes ours and leaves the foreign hooks alone.
	if _, _, _, err := Uninstall(project, false, false); err != nil {
		t.Fatal(err)
	}
	rest := map[string]any{}
	raw, _ = os.ReadFile(file)
	if err := json.Unmarshal(raw, &rest); err != nil {
		t.Fatal(err)
	}
	hooks, _ := rest["hooks"].(map[string]any)
	if hooks == nil {
		t.Fatal("uninstall removed the foreign hooks too")
	}
	if _, ok := hooks["Notification"]; !ok {
		t.Error("a hook on an event Saga does not use was removed")
	}
	pre, _ := hooks["PreToolUse"].([]any)
	if len(pre) != 1 {
		t.Errorf("PreToolUse has %d entries, want the one foreign hook", len(pre))
	}
	if strings.Contains(string(raw), Marker) {
		t.Error("a saga-owned command survived uninstall")
	}
	// Idempotent.
	_, again, _, err := Uninstall(project, false, false)
	if err != nil || len(again) != 0 {
		t.Errorf("second uninstall: %v %v", again, err)
	}
}
