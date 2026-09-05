package claims

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/cli"
	"github.com/ddh4r4m/saga/internal/gate"
	"github.com/ddh4r4m/saga/internal/snapshot"
	"github.com/ddh4r4m/saga/internal/store"
	"github.com/ddh4r4m/saga/internal/trace"
)

// GateViewOf reduces gate's status report to the verdict's input.
func GateViewOf(ld *gate.Loaded, rep *gate.Report) *GateView {
	if ld == nil || rep == nil || ld.Contract == nil {
		return nil
	}
	g := &GateView{Exit: rep.Exit, TreeHash: ld.TreeHash}
	for _, gs := range rep.Gates {
		gi := GateInfo{ID: gs.ID, State: gs.State, Runnable: gs.Runnable}
		id := gs.ID
		if i := strings.LastIndexByte(id, ':'); i >= 0 {
			id = id[i+1:]
		}
		if rec, _, err := gate.LoadEvidence(ld.Store, ld.Contract.Slug, id); err == nil && rec != nil {
			gi.EvidenceOutcome = rec.Outcome
			if rec.Tree != nil && rec.Tree.WorktreeHash != nil {
				gi.EvidenceTree = *rec.Tree.WorktreeHash
				if rec.Tree.SnapshotID != nil && g.SnapshotID == "" && *rec.Tree.WorktreeHash == ld.TreeHash {
					g.SnapshotID = *rec.Tree.SnapshotID
				}
			}
		}
		if gs.Failure != nil && strings.HasPrefix(*gs.Failure, "stale") {
			gi.Stale = true
		}
		g.Gates = append(g.Gates, gi)
	}
	if len(ld.Contract.In) > 0 {
		g.InScopeDiffEmpty = gate.TrackedDiffEmpty(ld.Diff(), ld.Contract.In, gate.FoldCase())
		if g.InScopeDiffEmpty {
			for _, e := range ld.Diff() {
				if e.Untracked && gate.MatchAny(ld.Contract.In, e.Path, gate.FoldCase()) {
					g.InScopeDiffEmpty = false
					break
				}
			}
		}
	}
	return g
}

// DiffPathsOf lists the working-tree diff paths against base (HEAD when
// empty), nil when git cannot answer.
func DiffPathsOf(root, base string) ([]string, bool) {
	if !snapshot.IsRepo(root) {
		return nil, false
	}
	if base == "" {
		base = "HEAD"
		if snapshot.Head(root) == "" {
			base = snapshot.EmptyTree
		}
	}
	entries, err := gate.DiffPaths(root, base)
	if err != nil {
		return nil, false
	}
	var out []string
	for _, e := range entries {
		if e.Status != "D" && e.Path != "" {
			out = append(out, e.Path)
		}
	}
	return out, true
}

// ExistsIn returns an Exists function over root.
func ExistsIn(root string) func(string) bool {
	return func(rel string) bool {
		_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil
	}
}

// FinalMessageOf returns the final message of turn (the latest
// assistant_end turn event when turn is 0) from the session's events and
// blobs, with its hash; ok is false when none is recorded.
func FinalMessageOf(events []trace.Event, blobDir string, turn int) (msg, hash string, t int, ok bool) {
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Type != trace.TypeTurn || ev.Body["phase"] != "assistant_end" || ev.Agent != "main" {
			continue
		}
		if turn > 0 && ev.Turn != turn {
			continue
		}
		hash, _ = ev.Body["final_message_hash"].(string)
		if s, ok := ev.Body["final_message_inline"].(string); ok {
			return s, hash, ev.Turn, true
		}
		if ref, ok := ev.Body["final_message_ref"].(string); ok && ref != "" && blobDir != "" {
			if raw, err := os.ReadFile(filepath.Join(blobDir, filepath.Base(ref))); err == nil {
				return string(raw), hash, ev.Turn, true
			}
		}
		return "", hash, ev.Turn, false
	}
	return "", "", 0, false
}

// FromSession builds the offline input for a recorded session: the
// events, the final message of the turn, the working tree and gate's
// status when a contract exists.
func FromSession(s *store.Store, session string, turn int) (*Input, error) {
	dir := trace.SessionDir(s, session)
	if _, err := os.Stat(dir); err != nil {
		return nil, cli.Errorf(cli.ExitEnvironment, "session %s not found under %s", session, s.Path("trace", "sessions"))
	}
	events, err := trace.ReadAll(dir)
	if err != nil {
		return nil, cli.Wrap(cli.ExitIntegrity, "session", err)
	}
	blobs := filepath.Join(dir, "blobs")
	final, hash, t, ok := FinalMessageOf(events, blobs, turn)
	in := &Input{Final: final, FinalAvailable: ok, FinalHash: hash, Events: events, Turn: t, Agent: "main", Root: s.Root, BlobDir: blobs, Source: "cli", Trigger: "cli", Session: session, Exists: ExistsIn(s.Root)}
	if turn == 0 && !ok {
		// No turn end recorded at all: judge over the pending state.
		in.Turn = 0
	}
	fillTree(in, s)
	return in, nil
}

// fillTree adds gate's view, the diff and the mode from the working tree.
func fillTree(in *Input, s *store.Store) {
	rev := "HEAD"
	if ld, err := gate.Load(s.Root, s); err == nil && ld.Contract != nil {
		rev = ld.Base
		if rep, err := gate.Status(ld, gate.StatusOptions{SkipGuards: true}); err == nil {
			in.Gate = GateViewOf(ld, rep)
		}
		in.DiffPaths, in.DiffKnown = DiffPathsOf(s.Root, ld.Base)
	} else {
		in.DiffPaths, in.DiffKnown = DiffPathsOf(s.Root, "")
	}
	cfg := LoadConfig(s.Root, rev)
	in.Mode, in.Unverified = cfg.Mode, cfg.Unverified
}

var reDiffHeader = regexp.MustCompile(`(?m)^diff --git a/(.+?) b/(.+)$`)
var reDeleted = regexp.MustCompile(`(?m)^deleted file mode`)

// DiffPathsFromPatch lists the paths a unified diff names (renames by
// their new name, deletions excluded).
func DiffPathsFromPatch(patch []byte) []string {
	var out []string
	blocks := reDiffHeader.FindAllSubmatchIndex(patch, -1)
	for i, m := range blocks {
		end := len(patch)
		if i+1 < len(blocks) {
			end = blocks[i+1][0]
		}
		if reDeleted.Match(patch[m[0]:end]) {
			continue
		}
		out = append(out, string(patch[m[4]:m[5]]))
	}
	return out
}

// ReadTraceJSONL parses a portable trace.jsonl into events.
func ReadTraceJSONL(raw []byte) ([]trace.Event, error) {
	var out []trace.Event
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, trace.LineCap+1), trace.LineCap*4)
	n := 0
	for sc.Scan() {
		n++
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev trace.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return out, fmt.Errorf("trace.jsonl line %d: %w", n, err)
		}
		out = append(out, ev)
	}
	return out, sc.Err()
}

// RunDirInput is what a bench run directory provides.
type RunDirInput struct {
	FinalMessage string
	// FinalAvailable is false when final_message.txt is absent.
	FinalAvailable bool
	TraceJSONL     []byte
	WorkspaceDiff  []byte
	// Workspace is the agent's tree when it still exists ("" when only
	// the archive is available); gate's status is read from it.
	Workspace string
}

// FromRunDir builds the derived input for a bench archive directory
// (bench-spec section 3.4 layout).
func FromRunDir(dir string) (*Input, error) {
	final, err := os.ReadFile(filepath.Join(dir, "final_message.txt"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, cli.Wrap(cli.ExitEnvironment, "final_message.txt", err)
	}
	rd := RunDirInput{FinalMessage: string(final), FinalAvailable: err == nil}
	if raw, err := os.ReadFile(filepath.Join(dir, "trace.jsonl")); err == nil {
		rd.TraceJSONL = raw
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "workspace.diff")); err == nil {
		rd.WorkspaceDiff = raw
	}
	return FromRun(rd)
}

// FromRun builds the derived input from run-directory contents.
func FromRun(rd RunDirInput) (*Input, error) {
	events, err := ReadTraceJSONL(rd.TraceJSONL)
	if err != nil {
		return nil, cli.Wrap(cli.ExitUsage, "trace.jsonl", err)
	}
	turn := 0
	for _, ev := range events {
		if ev.Turn > turn {
			turn = ev.Turn
		}
	}
	in := &Input{Final: rd.FinalMessage, FinalAvailable: rd.FinalAvailable, FinalHash: canon.SHA256([]byte(rd.FinalMessage)), Events: events, Turn: turn, Agent: "main", Source: "derived", Trigger: "derived", Mode: ModeMinimal, Unverified: "block", Session: "run"}
	if rd.WorkspaceDiff != nil {
		in.DiffPaths, in.DiffKnown = DiffPathsFromPatch(rd.WorkspaceDiff), true
	}
	if rd.Workspace != "" {
		in.Root = rd.Workspace
		in.Exists = ExistsIn(rd.Workspace)
		if st := store.Open(rd.Workspace); st.Exists() {
			blobs := ""
			for _, ev := range events {
				if ev.Session != "" {
					blobs = filepath.Join(trace.SessionDir(st, ev.Session), "blobs")
					break
				}
			}
			if blobs != "" {
				if _, err := os.Stat(blobs); err == nil {
					in.BlobDir = blobs
				}
			}
			if ld, err := gate.Load(rd.Workspace, st); err == nil && ld.Contract != nil {
				if rep, err := gate.Status(ld, gate.StatusOptions{SkipGuards: true}); err == nil {
					in.Gate = GateViewOf(ld, rep)
				}
			}
		}
	}
	return in, nil
}
