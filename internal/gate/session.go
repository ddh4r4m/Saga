package gate

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ddh4r4m/saga/internal/store"
)

// GateSessionSchema is the schema id of gate's per-session hook state.
const GateSessionSchema = "saga.gate.session/1"

// GateSession is .saga/observed/gate-<session>.json: the small amount of
// state gate's hook step records for the human-only commands. It is
// written by the hook, which runs in the harness's environment (not the
// agent's shell), so the agent cannot rewrite it through the Bash tool
// without the edit being a visible write into .saga/observed.
type GateSession struct {
	Schema  string `json:"schema"`
	Session string `json:"session"`
	// ToolInFlight is the tool_use_id of a shell tool call between its
	// PreToolUse and PostToolUse events, or "".
	ToolInFlight string `json:"tool_inflight"`
	ToolName     string `json:"tool_name"`
	// ApprovalDir is the canonical SAGA_APPROVAL_DIR the hook's own
	// environment carries ("" when unset); ApprovalDirSeen reports that
	// the hook has recorded it at least once.
	ApprovalDir     string `json:"approval_dir"`
	ApprovalDirSeen bool   `json:"approval_dir_seen"`
	UpdatedNS       int64  `json:"updated_ns"`
}

func gateSessionPath(s *store.Store, session string) string {
	return s.Path("observed", "gate-"+safeName(session)+".json")
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
}

// ReadGateSession loads the record for session, a fresh one when absent.
func ReadGateSession(s *store.Store, session string) (*GateSession, error) {
	g := &GateSession{Schema: GateSessionSchema, Session: session}
	p := gateSessionPath(s, session)
	if err := store.CheckShape(p); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return g, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, g); err != nil || g.Schema != GateSessionSchema {
		return &GateSession{Schema: GateSessionSchema, Session: session}, nil
	}
	return g, nil
}

// WriteGateSession stores the record atomically.
func WriteGateSession(s *store.Store, g *GateSession) error {
	if err := os.MkdirAll(s.Path("observed"), 0o700); err != nil {
		return err
	}
	g.Schema = GateSessionSchema
	g.UpdatedNS = time.Now().UnixNano()
	raw, err := json.Marshal(g)
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(gateSessionPath(s, g.Session), raw, 0o600)
}

// recentGateSessions returns the records updated within maxAge, newest
// first.
func recentGateSessions(s *store.Store, maxAge time.Duration) []*GateSession {
	entries, err := os.ReadDir(s.Path("observed"))
	if err != nil {
		return nil
	}
	cutoff := time.Now().Add(-maxAge).UnixNano()
	var out []*GateSession
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "gate-") || !strings.HasSuffix(name, ".json") || e.IsDir() {
			continue
		}
		p := filepath.Join(s.Path("observed"), name)
		if store.CheckShape(p) != nil {
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var g GateSession
		if json.Unmarshal(raw, &g) != nil || g.Schema != GateSessionSchema || g.UpdatedNS < cutoff {
			continue
		}
		out = append(out, &g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedNS > out[j].UpdatedNS })
	return out
}

// ToolInFlight reports a shell tool call the hook recorded as started and
// not yet finished for any session of this repository updated within
// maxAge.
func ToolInFlight(s *store.Store, maxAge time.Duration) (session, tool string, ok bool) {
	for _, g := range recentGateSessions(s, maxAge) {
		if g.ToolInFlight != "" {
			return g.Session, g.ToolName, true
		}
	}
	return "", "", false
}

// HookApprovalDir returns the approval store the most recent hook
// session (within maxAge) saw in its environment, and whether one was
// recorded at all.
func HookApprovalDir(s *store.Store, maxAge time.Duration) (dir string, seen bool) {
	for _, g := range recentGateSessions(s, maxAge) {
		if g.ApprovalDirSeen {
			return g.ApprovalDir, true
		}
	}
	return "", false
}
