package ingest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/shahab5191/openshin/internal/store"
)

// Tailer reads a Claude Code transcript JSONL file incrementally, ingesting
// each new line into the raw log. It remembers how far it got in a sidecar file
// so restarts and re-runs are cheap and idempotent.
type Tailer struct {
	store *store.Conversations
}

func NewTailer(s *store.Conversations) *Tailer {
	return &Tailer{store: s}
}

// Result summarises one tail pass.
type Result struct {
	Scanned  int
	Ingested int
	Skipped  int
}

// IngestFile ingests every new line of path since the previous run.
func (t *Tailer) IngestFile(ctx context.Context, path string) (Result, error) {
	var res Result

	f, err := os.Open(path)
	if err != nil {
		return res, fmt.Errorf("tail: open transcript: %w", err)
	}
	defer f.Close()

	offset, err := readOffset(path)
	if err != nil {
		return res, err
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return res, fmt.Errorf("tail: seek: %w", err)
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		res.Scanned++
		offset += int64(len(scanner.Bytes())) + 1

		msg, ok, err := parseClaudeLine(scanner.Text())
		if err != nil {
			return res, fmt.Errorf("tail: parse line %d: %w", res.Scanned, err)
		}
		if !ok {
			res.Skipped++
			continue
		}
		if _, err := t.store.AppendMessage(ctx, msg); err != nil {
			return res, fmt.Errorf("tail: ingest line %d: %w", res.Scanned, err)
		}
		res.Ingested++
	}
	if err := scanner.Err(); err != nil {
		return res, fmt.Errorf("tail: scan: %w", err)
	}

	if err := writeOffset(path, offset); err != nil {
		return res, err
	}
	return res, nil
}

func offsetPath(path string) string {
	return path + ".offset"
}

func readOffset(path string) (int64, error) {
	raw, err := os.ReadFile(offsetPath(path))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("tail: read offset: %w", err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("tail: parse offset: %w", err)
	}
	return n, nil
}

func writeOffset(path string, offset int64) error {
	if err := os.WriteFile(offsetPath(path), []byte(strconv.FormatInt(offset, 10)), 0o644); err != nil {
		return fmt.Errorf("tail: write offset: %w", err)
	}
	return nil
}
