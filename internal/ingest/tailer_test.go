package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shahab5191/openshin/internal/db/testdb"
	"github.com/shahab5191/openshin/internal/store"
)

func TestTailerIngestIncrementalAndIdempotent(t *testing.T) {
	c := store.NewConversations(testdb.Open(t))
	tailer := NewTailer(c)
	ctx := context.Background()

	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	write := func(s string) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer f.Close()
		if _, err := f.WriteString(s); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	write(`{"type":"user","uuid":"u1","sessionId":"s1","message":{"content":[{"type":"text","text":"hello"}]}}` + "\n")

	res, err := tailer.IngestFile(ctx, path)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if res.Ingested != 1 {
		t.Fatalf("expected 1 ingested, got %+v", res)
	}

	// Re-run without appending: nothing new.
	res, err = tailer.IngestFile(ctx, path)
	if err != nil {
		t.Fatalf("re-ingest: %v", err)
	}
	if res.Ingested != 0 {
		t.Fatalf("expected 0 ingested on re-run, got %+v", res)
	}

	// Append a second line: only the new line is ingested.
	write(`{"type":"assistant","uuid":"u2","sessionId":"s1","message":{"content":[{"type":"text","text":"hi back"}]}}` + "\n")
	res, err = tailer.IngestFile(ctx, path)
	if err != nil {
		t.Fatalf("incremental ingest: %v", err)
	}
	if res.Ingested != 1 {
		t.Fatalf("expected 1 newly ingested, got %+v", res)
	}

	msgs, err := c.ReadRange(ctx, "s1", 1, 1<<62)
	if err != nil {
		t.Fatalf("read range: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 total messages, got %d", len(msgs))
	}
}
