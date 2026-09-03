// Command saga is the single Saga binary (ADR 0008). main stamps the
// version and hands off to internal/cli/cmd.
package main

import (
	"os"

	"github.com/ddh4r4m/saga/internal/cli/cmd"
)

// version is set by -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(int(cmd.Main(version, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)))
}
