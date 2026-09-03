package trace

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/store"
)

// Size caps of trace-spec section 2.5.
const (
	// InlineCap is the largest payload stored inline in an event.
	InlineCap = 4 * 1024
	// BlobCap is the largest payload stored whole in blobs/.
	BlobCap = 8 * 1024 * 1024
	// HeadTailCap is the head and tail kept of a payload above BlobCap.
	HeadTailCap = 64 * 1024
	// LineCap is the largest event line the writer accepts.
	LineCap = 16 * 1024
	// SegmentCap is the rotation size of events.<n>.jsonl.
	SegmentCap = 64 * 1024 * 1024
)

var segmentRe = regexp.MustCompile(`^events\.(\d{6})\.jsonl$`)

// SessionDir returns .saga/trace/sessions/<id> for a sanitised session id.
func SessionDir(s *store.Store, session string) string {
	return s.Path("trace", "sessions", SafeID(session))
}

// SafeID reduces a session id to a filesystem-safe name; anything outside
// [A-Za-z0-9._-] is replaced by the id's sha256 prefix.
func SafeID(id string) string {
	if id == "" {
		return "unknown"
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return "id-" + canon.SHA256([]byte(id))[7:7+24]
		}
	}
	return id
}

// Writer appends events to one session's hash-chained JSONL log. One
// writer per session holds the LOCK file for its lifetime (trace-spec
// section 2.4); a second process queues on the lock.
type Writer struct {
	Dir        string
	Session    string
	SegmentCap int64
	lock       *os.File
	seq        int
	prev       string
	segment    int
	size       int64
	start      time.Time
	masker     *Masker
}

// OpenWriter opens (creating if needed) the session directory, takes the
// lock and recovers the chain head from the last line of the last
// segment.
func OpenWriter(dir, session string, masker *Masker) (*Writer, error) {
	if err := store.CheckShape(dir); err != nil {
		return nil, err
	}
	for _, d := range []string{dir, filepath.Join(dir, "blobs"), filepath.Join(dir, "checkpoints")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	lock, err := acquireLock(filepath.Join(dir, "LOCK"))
	if err != nil {
		return nil, err
	}
	w := &Writer{Dir: dir, Session: session, SegmentCap: SegmentCap, lock: lock, prev: canon.Genesis, segment: 1, masker: masker}
	if w.masker == nil {
		w.masker = NewMasker(session)
	}
	if err := w.recover(); err != nil {
		lock.Close()
		return nil, err
	}
	return w, nil
}

// Close releases the lock.
func (w *Writer) Close() error {
	if w.lock == nil {
		return nil
	}
	err := releaseLock(w.lock)
	w.lock = nil
	return err
}

// Seq is the last seq written.
func (w *Writer) Seq() int { return w.seq }

// Prev is the hash of the last event written.
func (w *Writer) Prev() string { return w.prev }

// Masker returns the writer's masker.
func (w *Writer) Masker() *Masker { return w.masker }

// Segments lists the session's segment files in order.
func Segments(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if segmentRe.MatchString(e.Name()) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func segmentName(n int) string { return fmt.Sprintf("events.%06d.jsonl", n) }

func (w *Writer) recover() error {
	segs, err := Segments(w.Dir)
	if err != nil {
		return err
	}
	if len(segs) == 0 {
		w.start = time.Now()
		return nil
	}
	last := segs[len(segs)-1]
	m := segmentRe.FindStringSubmatch(filepath.Base(last))
	w.segment, _ = strconv.Atoi(m[1])
	fi, err := os.Stat(last)
	if err != nil {
		return err
	}
	w.size = fi.Size()
	line, err := lastLine(last)
	if err != nil {
		return err
	}
	if len(line) == 0 {
		// A fresh rotated segment: the head is the previous segment's tail.
		if len(segs) < 2 {
			w.start = time.Now()
			return nil
		}
		line, err = lastLine(segs[len(segs)-2])
		if err != nil {
			return err
		}
	}
	var ev Event
	if err := json.Unmarshal(line, &ev); err != nil {
		return fmt.Errorf("trace: last event unreadable in %s: %w", last, err)
	}
	w.seq = ev.Seq
	w.prev = ev.Hash
	// Rebuild the monotonic origin from the first event of the session.
	first, err := firstLine(segs[0])
	if err == nil && len(first) > 0 {
		var f Event
		if json.Unmarshal(first, &f) == nil {
			if t, err := time.Parse(TSFormat, f.TS); err == nil {
				w.start = t.Add(-time.Duration(f.MonoNS))
			}
		}
	}
	if w.start.IsZero() {
		w.start = time.Now()
	}
	return nil
}

func firstLine(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, LineCap+1)
	line, err := r.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return bytes.TrimRight(line, "\n"), nil
}

func lastLine(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := fi.Size()
	if size == 0 {
		return nil, nil
	}
	chunk := int64(LineCap + 2)
	if chunk > size {
		chunk = size
	}
	buf := make([]byte, chunk)
	if _, err := f.ReadAt(buf, size-chunk); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	buf = bytes.TrimRight(buf, "\n")
	if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
		buf = buf[i+1:]
	}
	return buf, nil
}

// Now returns the wall clock and the monotonic offset since session start.
func (w *Writer) Now() (string, int64) {
	t := time.Now()
	mono := t.Sub(w.start).Nanoseconds()
	if mono < 0 {
		mono = 0
	}
	return FormatTS(t), mono
}

// Append assigns seq, ts, mono_ns, prev and hash to ev, validates it and
// writes it as one line, rotating the segment when the cap is reached. A
// line above LineCap is replaced by a tool_result event with
// error "event_oversize" (trace-spec section 2.5).
func (w *Writer) Append(ev *Event) error {
	if w.lock == nil {
		return errors.New("trace: writer closed")
	}
	ev.Schema = Schema
	ev.Session = w.Session
	if ev.Agent == "" {
		ev.Agent = "main"
	}
	ev.Seq = w.seq + 1
	ev.TS, ev.MonoNS = w.Now()
	ev.Prev = w.prev
	ev.Hash = ""
	line, err := w.encode(ev)
	if err != nil {
		return err
	}
	if len(line) > LineCap {
		over := &Event{
			Turn: ev.Turn, Agent: ev.Agent, Type: TypeToolResult, Source: ev.Source, Seq: ev.Seq, TS: ev.TS, MonoNS: ev.MonoNS, Prev: ev.Prev,
			Body: map[string]any{"for_seq": nil, "exit": nil, "error": "event_oversize", "result_hash": canon.SHA256(line), "result_bytes": len(line), "truncated": true, "wall_ms": nil, "served": "live"},
		}
		over.Schema, over.Session = Schema, w.Session
		line, err = w.encode(over)
		if err != nil {
			return err
		}
		*ev = *over
	}
	if err := ev.Validate(); err != nil {
		return err
	}
	if w.size > 0 && w.size+int64(len(line)) > w.SegmentCap {
		w.segment++
		w.size = 0
	}
	f, err := os.OpenFile(filepath.Join(w.Dir, segmentName(w.segment)), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(line)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	if cerr != nil {
		return cerr
	}
	w.size += int64(len(line))
	w.seq = ev.Seq
	w.prev = ev.Hash
	return nil
}

func (w *Writer) encode(ev *Event) ([]byte, error) {
	h, err := ev.ComputeHash()
	if err != nil {
		return nil, err
	}
	ev.Hash = h
	b, err := canon.JSON(ev)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Payload stores a masked payload per the section 2.5 caps and returns the
// fields to put in an event: an inline value (string) when small, else a
// blob ref; plus hash, byte count, truncated flag and masked count.
type Payload struct {
	Hash        string
	Bytes       int
	Inline      *string
	Ref         *string
	Truncated   bool
	MaskedCount int
}

// StorePayload masks b, hashes the full masked bytes and stores them
// inline, as a blob, or as head plus tail when above BlobCap.
func (w *Writer) StorePayload(b []byte) (Payload, error) {
	masked, n := w.masker.Mask(b)
	p := Payload{Hash: canon.SHA256(masked), Bytes: len(masked), MaskedCount: n}
	if len(masked) <= InlineCap {
		s := string(masked)
		p.Inline = &s
		return p, nil
	}
	data := masked
	if len(masked) > BlobCap {
		data = append(append([]byte{}, masked[:HeadTailCap]...), masked[len(masked)-HeadTailCap:]...)
		p.Truncated = true
	}
	name := p.Hash[len("sha256:"):]
	path := filepath.Join(w.Dir, "blobs", name)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		if err := store.WriteFileAtomic(path, data, 0o600); err != nil {
			return p, err
		}
	}
	ref := "blobs/" + name
	p.Ref = &ref
	return p, nil
}

// ReadAll returns every event of a session in seq order.
func ReadAll(dir string) ([]Event, error) {
	var out []Event
	err := Walk(dir, func(ev Event, _ []byte) error {
		out = append(out, ev)
		return nil
	})
	return out, err
}

// Walk calls fn for every event line of a session in order, with the raw
// line.
func Walk(dir string, fn func(ev Event, line []byte) error) error {
	segs, err := Segments(dir)
	if err != nil {
		return err
	}
	for _, seg := range segs {
		f, err := os.Open(seg)
		if err != nil {
			return err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, LineCap+1), LineCap*4)
		n := 0
		for sc.Scan() {
			n++
			line := sc.Bytes()
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var ev Event
			if err := json.Unmarshal(line, &ev); err != nil {
				f.Close()
				return fmt.Errorf("%s line %d: %w", filepath.Base(seg), n, err)
			}
			if err := fn(ev, append([]byte{}, line...)); err != nil {
				f.Close()
				return err
			}
		}
		if err := sc.Err(); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	return nil
}

// ListSessions returns session directory names, newest first by
// modification time.
func ListSessions(s *store.Store) ([]string, error) {
	dir := s.Path("trace", "sessions")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	type item struct {
		name string
		mod  time.Time
	}
	var items []item
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, item{e.Name(), fi.ModTime()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.name
	}
	return out, nil
}
