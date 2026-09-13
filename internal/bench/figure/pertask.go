package figure

import "github.com/ddh4r4m/saga/internal/cli"

// RenderPerTask is the d2 figure; not built yet.
func RenderPerTask(archive, out string) (string, error) {
	return "", cli.Errorf(cli.ExitUsage, "figure: --per-task is not implemented yet")
}
