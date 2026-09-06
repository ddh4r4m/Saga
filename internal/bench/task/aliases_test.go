package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAliasesAppearInAgentVisibleFiles: every alias of every group must
// exist verbatim in a file the agent can read (the prompt, the contract,
// or the repo it is given). An alias nobody can find is a term the task
// requires and never supplies.
func TestAliasesAppearInAgentVisibleFiles(t *testing.T) {
	dirs, err := Find(filepath.Join("..", "..", "..", "bench", "tasks", "*"))
	if err != nil || len(dirs) == 0 {
		t.Skip("no corpus")
	}
	for _, d := range dirs {
		tk, err := Load(d)
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		groups := tk.Terminal.MentionGroups()
		if len(groups) == 0 {
			continue
		}
		visible := agentVisible(t, d)
		for _, g := range groups {
			for _, alias := range g {
				if !strings.Contains(visible, strings.ToLower(alias)) {
					t.Errorf("%s: alias %q is in no agent-visible file", tk.ID, alias)
				}
			}
		}
	}
}

// agentVisible concatenates, lower-cased, everything the agent can read:
// prompt.md, contract.md and every file of repo/.
func agentVisible(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	for _, n := range []string{"prompt.md", "contract.md"} {
		if raw, err := os.ReadFile(filepath.Join(dir, n)); err == nil {
			b.Write(raw)
			b.WriteByte('\n')
		}
	}
	_ = filepath.Walk(filepath.Join(dir, "repo"), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if raw, rerr := os.ReadFile(p); rerr == nil {
			b.Write(raw)
			b.WriteByte('\n')
		}
		b.WriteString(p + "\n")
		return nil
	})
	return strings.ToLower(b.String())
}
