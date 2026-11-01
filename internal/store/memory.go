package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/shahab5191/openshin/internal/db/sqlc"
)

// Segment is a contiguous, settled run of messages of one thread.
type Segment struct {
	SessionID string `json:"session_id"`
	ThreadID  string `json:"thread_id"`
	StartSeq  int64  `json:"start_seq"`
	EndSeq    int64  `json:"end_seq"`
	Subject   string `json:"subject,omitempty"`
}

// Fact is one durable fact extracted from a segment.
type Fact struct {
	SessionID string `json:"session_id"`
	ThreadID  string `json:"thread_id"`
	Fact      string `json:"fact"`
	Embedding []byte `json:"-"`
	StartSeq  int64  `json:"start_seq"`
	EndSeq    int64  `json:"end_seq"`
	FactHash  string `json:"fact_hash"`
	ExpiresAt string `json:"expires_at"`
}

// Memory stores the mid-term layer: segments and the facts distilled from them.
type Memory struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewMemory(db *sql.DB) *Memory {
	return &Memory{db: db, q: sqlc.New(db)}
}

// ListThreads returns all threads in creation order.
func (m *Memory) ListThreads(ctx context.Context) ([]Thread, error) {
	rows, err := m.q.ListThreads(ctx)
	if err != nil {
		return nil, err
	}
	threads := make([]Thread, 0, len(rows))
	for _, r := range rows {
		threads = append(threads, toThread(r))
	}
	return threads, nil
}

// MessagesAfter returns messages of a thread with seq > afterSeq, oldest first.
func (m *Memory) MessagesAfter(ctx context.Context, threadID string, afterSeq int64, limit int) ([]Message, error) {
	rows, err := m.q.MessagesAfter(ctx, sqlc.MessagesAfterParams{
		ThreadID: threadID,
		Seq:      afterSeq,
		Limit:    int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("messages after: %w", err)
	}
	msgs := make([]Message, 0, len(rows))
	for _, r := range rows {
		msgs = append(msgs, toMessage(r))
	}
	return msgs, nil
}

// LastSegmentEnd returns the highest end_seq of any segment of the thread, or 0.
func (m *Memory) LastSegmentEnd(ctx context.Context, threadID string) (int64, error) {
	return m.q.LastSegmentEnd(ctx, threadID)
}

// CurrentSubject returns the thread's current subject, or "".
func (m *Memory) CurrentSubject(ctx context.Context, threadID string) (string, error) {
	s, err := m.q.CurrentSubject(ctx, threadID)
	if err != nil {
		return "", err
	}
	return s.String, nil
}

// CommitSegment atomically records a segment, its facts, and the new subject.
// The segment is only marked done once its facts are ready, so a crash mid-way
// simply re-runs the (idempotent) extraction next pass.
func (m *Memory) CommitSegment(ctx context.Context, seg Segment, facts []Fact) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("commit segment: begin: %w", err)
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)

	if _, err := q.CreateSegment(ctx, sqlc.CreateSegmentParams{
		SessionID: seg.SessionID,
		ThreadID:  seg.ThreadID,
		StartSeq:  seg.StartSeq,
		EndSeq:    seg.EndSeq,
		Subject:   nullString(seg.Subject),
	}); err != nil {
		return fmt.Errorf("commit segment: create: %w", err)
	}

	for _, f := range facts {
		if err := q.UpsertFact(ctx, sqlc.UpsertFactParams{
			SessionID: f.SessionID,
			ThreadID:  f.ThreadID,
			Fact:      f.Fact,
			Embedding: f.Embedding,
			StartSeq:  f.StartSeq,
			EndSeq:    f.EndSeq,
			FactHash:  f.FactHash,
			ExpiresAt: f.ExpiresAt,
		}); err != nil {
			return fmt.Errorf("commit segment: upsert fact: %w", err)
		}
	}

	if err := q.SetSubject(ctx, sqlc.SetSubjectParams{
		Subject: nullString(seg.Subject),
		ID:      seg.ThreadID,
	}); err != nil {
		return fmt.Errorf("commit segment: set subject: %w", err)
	}

	return tx.Commit()
}

// DeleteExpiredFacts removes facts whose expiry has passed, returning how many.
func (m *Memory) DeleteExpiredFacts(ctx context.Context, now string) (int64, error) {
	return m.q.DeleteExpiredFacts(ctx, now)
}

// ListSegments returns a thread's segments, oldest first.
func (m *Memory) ListSegments(ctx context.Context, threadID string) ([]Segment, error) {
	rows, err := m.q.ListSegments(ctx, threadID)
	if err != nil {
		return nil, err
	}
	segs := make([]Segment, 0, len(rows))
	for _, r := range rows {
		segs = append(segs, Segment{
			SessionID: r.SessionID,
			ThreadID:  r.ThreadID,
			StartSeq:  r.StartSeq,
			EndSeq:    r.EndSeq,
			Subject:   r.Subject.String,
		})
	}
	return segs, nil
}

// ListFacts returns a session's facts, oldest first.
func (m *Memory) ListFacts(ctx context.Context, sessionID string) ([]Fact, error) {
	rows, err := m.q.ListFacts(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	facts := make([]Fact, 0, len(rows))
	for _, r := range rows {
		facts = append(facts, Fact{
			SessionID: r.SessionID,
			ThreadID:  r.ThreadID,
			Fact:      r.Fact,
			Embedding: r.Embedding,
			StartSeq:  r.StartSeq,
			EndSeq:    r.EndSeq,
			FactHash:  r.FactHash,
			ExpiresAt: r.ExpiresAt,
		})
	}
	return facts, nil
}

// EncodeEmbedding serialises a float32 vector to little-endian bytes.
func EncodeEmbedding(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// DecodeEmbedding reverses EncodeEmbedding.
func DecodeEmbedding(b []byte) []float32 {
	n := len(b) / 4
	v := make([]float32, n)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}
