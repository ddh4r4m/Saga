// Package claudecode generates and inspects the Claude Code settings
// fragment that binds every Saga event to `saga hook claude-code <event>`
// (contracts section 1, section 9). It writes only the hooks it owns and
// reads settings files without modifying them for doctor.
package claudecode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddh4r4m/saga/internal/hookio"
	"github.com/ddh4r4m/saga/internal/store"
)

// Harness is the id in `saga install --harness`.
const Harness = "claude-code"

// Timeout is the per-hook timeout written to settings, in seconds. The
// entry's own deadline (hook.deadline_ms, default 5 s) fires first, so a
// harness-side timeout, which fails open (harness-facts C23), is never
// the deciding one.
const Timeout = 60

// Command returns the hook command for event.
func Command(binary, event string) string {
	return fmt.Sprintf("%s hook %s %s", binary, Harness, event)
}

// Marker identifies Saga-owned hook entries in any settings file.
const Marker = " hook " + Harness + " "

// Fragment returns the {"hooks": {...}} object for binary. Matchers are
// omitted, which matches every tool (harness-facts C20).
func Fragment(binary string) map[string]any {
	hooks := map[string]any{}
	for _, ev := range hookio.Events {
		hooks[ev] = []any{map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": Command(binary, ev), "timeout": Timeout}},
		}}
	}
	return map[string]any{"hooks": hooks}
}

// SettingsFile returns the settings path `saga install` writes: the
// project's .claude/settings.local.json by default, settings.json with
// shared (gate-spec section 6.1).
func SettingsFile(project string, shared bool) string {
	name := "settings.local.json"
	if shared {
		name = "settings.json"
	}
	return filepath.Join(project, ".claude", name)
}

// UserSettingsFile is ~/.claude/settings.json.
func UserSettingsFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// Install merges the fragment into the settings file, replacing any
// earlier Saga-owned entries and leaving every other hook untouched. With
// dryRun nothing is written; the returned bytes are what would be
// written.
func Install(project, binary string, shared, dryRun bool) (file string, result []byte, entries []store.ManifestEntry, err error) {
	file = SettingsFile(project, shared)
	if err := store.CheckShape(file); err != nil {
		return file, nil, nil, err
	}
	settings := map[string]any{}
	raw, rerr := os.ReadFile(file)
	if rerr == nil {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return file, nil, nil, fmt.Errorf("%s: %w", file, err)
		}
	} else if !errors.Is(rerr, fs.ErrNotExist) {
		return file, nil, nil, rerr
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	ours := Fragment(binary)["hooks"].(map[string]any)
	for ev, mine := range ours {
		existing, _ := hooks[ev].([]any)
		kept := existing[:0:0]
		for _, e := range existing {
			if !ownsEntry(e) {
				kept = append(kept, e)
			}
		}
		hooks[ev] = append(kept, mine.([]any)...)
		entries = append(entries, store.ManifestEntry{Harness: Harness, Event: ev, File: file, Command: Command(binary, ev)})
	}
	settings["hooks"] = hooks
	result, err = json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return file, nil, nil, err
	}
	result = append(result, '\n')
	if dryRun {
		return file, result, entries, nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return file, nil, nil, err
	}
	return file, result, entries, store.WriteFileAtomic(file, result, 0o644)
}

func ownsEntry(e any) bool {
	m, ok := e.(map[string]any)
	if !ok {
		return false
	}
	inner, _ := m["hooks"].([]any)
	for _, h := range inner {
		hm, _ := h.(map[string]any)
		if cmd, _ := hm["command"].(string); strings.Contains(cmd, Marker) {
			return true
		}
	}
	return false
}

// Registration reports whether each event is bound to a Saga command in
// any of the files (read-only). A missing or unparsable file counts as no
// registrations from it.
func Registration(files []string) []struct {
	Event, File, Command string
	Registered           bool
} {
	type reg = struct {
		Event, File, Command string
		Registered           bool
	}
	out := make([]reg, 0, len(hookio.Events))
	found := map[string]reg{}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var settings map[string]any
		if json.Unmarshal(raw, &settings) != nil {
			continue
		}
		hooks, _ := settings["hooks"].(map[string]any)
		for ev, list := range hooks {
			items, _ := list.([]any)
			for _, e := range items {
				m, _ := e.(map[string]any)
				inner, _ := m["hooks"].([]any)
				for _, h := range inner {
					hm, _ := h.(map[string]any)
					cmd, _ := hm["command"].(string)
					if strings.Contains(cmd, Marker+ev) {
						if _, dup := found[ev]; !dup {
							found[ev] = reg{Event: ev, File: file, Command: cmd, Registered: true}
						}
					}
				}
			}
		}
	}
	for _, ev := range hookio.Events {
		if r, ok := found[ev]; ok {
			out = append(out, r)
		} else {
			out = append(out, reg{Event: ev})
		}
	}
	return out
}
