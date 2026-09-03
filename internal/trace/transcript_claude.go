package trace

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"

	"github.com/ddh4r4m/saga/internal/store"
)

// TranscriptCall is one assistant message with usage read from a Claude
// Code transcript (trace-spec section 9.3, harness-facts C29).
type TranscriptCall struct {
	MessageID  string
	Model      string
	StopReason string
	RequestID  string
	Timestamp  string
	Usage      map[string]any
}

// ReadClaudeTranscript reads the transcript from byte offset, returning
// the assistant messages that carry usage, deduplicated on message.id
// (the last line for an id wins within the read), and the new offset.
// Only complete lines are consumed; a partial trailing line is left for
// the next read. The file is opened read-only and never written.
func ReadClaudeTranscript(path string, offset int64) ([]TranscriptCall, int64, error) {
	if err := store.CheckShape(path); err != nil {
		return nil, offset, err
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, offset, err
	}
	if err != nil {
		return nil, offset, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	r := bufio.NewReaderSize(f, 256*1024)
	var order []string
	byID := map[string]*TranscriptCall{}
	pos := offset
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			// Partial trailing line: leave it for the next read.
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, pos, err
		}
		pos += int64(len(line))
		var rec struct {
			Type      string `json:"type"`
			RequestID string `json:"requestId"`
			Timestamp string `json:"timestamp"`
			Message   struct {
				ID         string         `json:"id"`
				Model      string         `json:"model"`
				StopReason string         `json:"stop_reason"`
				Usage      map[string]any `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &rec) != nil || rec.Type != "assistant" || rec.Message.Usage == nil {
			continue
		}
		c := &TranscriptCall{MessageID: rec.Message.ID, Model: rec.Message.Model, StopReason: rec.Message.StopReason, RequestID: rec.RequestID, Timestamp: rec.Timestamp, Usage: rec.Message.Usage}
		if c.MessageID == "" {
			c.MessageID = rec.RequestID
		}
		if _, seen := byID[c.MessageID]; !seen {
			order = append(order, c.MessageID)
		}
		byID[c.MessageID] = c
	}
	out := make([]TranscriptCall, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, pos, nil
}
