package claims

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/ddh4r4m/saga/internal/gate"
	"github.com/ddh4r4m/saga/internal/snapshot"
)

// Config is the [trace.claims] table of .saga/config.toml (trace-spec
// section 5.9), read from BASE: like [gate] (contracts section 2).
type Config struct {
	// Unverified is "block" (full-mode default) or "warn".
	Unverified string `toml:"unverified"`
	// Present reports a config file at the rev (full mode).
	Present bool `toml:"-"`
	// Mode is minimal or full.
	Mode string `toml:"-"`
	// MaxBlocks is gate's ceiling, shared by claim blocks (gate-spec
	// section 6: a claim block counts toward max_blocks).
	MaxBlocks int `toml:"-"`
}

// LoadConfig reads the table at rev ("HEAD" without a contract), the
// same rule as gate's mode (gate-spec section 5.4: never the working
// tree, so an agent cannot switch the mode for the run it is in).
// Outside a git repository the working-tree file is read.
func LoadConfig(root, rev string) Config {
	cfg := Config{Unverified: "block", Mode: ModeMinimal, MaxBlocks: 6}
	var raw []byte
	var ok bool
	if snapshot.IsRepo(root) {
		raw, ok = gate.ShowAt(root, rev, ".saga/config.toml")
	} else if b, err := os.ReadFile(filepath.Join(root, ".saga", "config.toml")); err == nil {
		raw, ok = b, true
	}
	if !ok {
		return cfg
	}
	cfg.Present, cfg.Mode = true, ModeFull
	var doc struct {
		Trace struct {
			Claims Config `toml:"claims"`
		} `toml:"trace"`
		Gate struct {
			MaxBlocks int `toml:"max_blocks"`
		} `toml:"gate"`
	}
	if _, err := toml.Decode(string(raw), &doc); err == nil {
		if doc.Trace.Claims.Unverified == "warn" {
			cfg.Unverified = "warn"
		}
		if doc.Gate.MaxBlocks > 0 && doc.Gate.MaxBlocks <= 8 {
			cfg.MaxBlocks = doc.Gate.MaxBlocks
		}
	}
	return cfg
}
