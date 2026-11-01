// Package store is the repository layer over the raw SQLite schema. It maps the
// sqlc-generated row types onto domain types and owns the transactional
// invariants (a message's thread and session always exist before it is appended).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shahab5191/openshin/internal/db/sqlc"
)

// ThreadKind is the routing switch: user threads feed the fact path, subagent
// threads feed the skill path.
type ThreadKind string

const (
	KindUser     ThreadKind = "user"
	KindSubagent ThreadKind = "subagent"
)

// Message roles.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
	RoleSystem    = "system"
)

// Message is one raw conversation row.
type Message struct {
	Seq       int64  `json:"seq"`
	ThreadID  string `json:"thread_id"`
	ThreadSeq int64  `json:"thread_seq"`
	Actor     string `json:"actor"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	SourceRef string `json:"source_ref,omitempty"`
	CreatedAt string `json:"created_at"`
}

// Thread is one conversation thread.
type Thread struct {
	ID             string `json:"id"`
	SessionID      string `json:"session_id"`
	ParentThreadID string `json:"parent_thread_id,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	Kind           string `json:"kind"`
	Brief          string `json:"brief,omitempty"`
	Subject        string `json:"subject,omitempty"`
	Distilled      bool   `json:"distilled,omitempty"`
	CreatedAt      string `json:"created_at"`
	CompletedAt    string `json:"completed_at,omitempty"`
}

// IngestMessage is the agent-agnostic shape a message arrives in: HTTP ingest,
// the transcript tailer, and (later) MCP tools all produce one.
type IngestMessage struct {
	SessionID      string
	ThreadID       string // empty means the root thread, whose id == session id
	ParentThreadID string
	TaskID         string
	Kind           string // "user" or "subagent"; empty means "user"
	Brief          string
	Actor          string
	Role           string
	Content        string
	SourceRef      string // idempotency key from the ingest source
	CreatedAt      string // RFC3339; empty means now
}

// Conversations stores and reads the raw log.
type Conversations struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewConversations(db *sql.DB) *Conversations {
	return &Conversations{db: db, q: sqlc.New(db)}
}

// AppendMessage ensures the session and thread exist, then appends the message
// at the next per-thread position. It is atomic and idempotent: a replayed
// message carrying the same SourceRef upserts to the same row instead of
// duplicating (see AppendMessageIdempotent).
func (c *Conversations) AppendMessage(ctx context.Context, in IngestMessage) (Message, error) {
	if in.SessionID == "" {
		return Message{}, fmt.Errorf("append message: empty session id")
	}
	if in.ThreadID == "" {
		in.ThreadID = in.SessionID // root thread id == session id
	}
	if in.Kind == "" {
		in.Kind = string(KindUser)
	}
	if in.Actor == "" {
		return Message{}, fmt.Errorf("append message: empty actor")
	}
	if in.Role == "" {
		return Message{}, fmt.Errorf("append message: empty role")
	}
	if in.Content == "" {
		return Message{}, fmt.Errorf("append message: empty content")
	}
	if in.CreatedAt == "" {
		in.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, fmt.Errorf("append message: begin: %w", err)
	}
	defer tx.Rollback()
	q := c.q.WithTx(tx)

	// Idempotency: a message already recorded under this source_ref is returned
	// unchanged. The single-writer connection makes the check-then-insert safe.
	if in.SourceRef != "" {
		if existing, err := q.GetMessageBySourceRef(ctx, nullString(in.SourceRef)); err == nil {
			return toMessage(existing), nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return Message{}, fmt.Errorf("append message: dedup lookup: %w", err)
		}
	}

	if err := q.UpsertSession(ctx, in.SessionID); err != nil {
		return Message{}, fmt.Errorf("append message: upsert session: %w", err)
	}
	if err := q.UpsertThread(ctx, sqlc.UpsertThreadParams{
		ID:             in.ThreadID,
		SessionID:      in.SessionID,
		ParentThreadID: nullString(in.ParentThreadID),
		TaskID:         nullString(in.TaskID),
		Kind:           in.Kind,
		Brief:          nullString(in.Brief),
	}); err != nil {
		return Message{}, fmt.Errorf("append message: upsert thread: %w", err)
	}

	seq, err := q.NextThreadSeq(ctx, in.ThreadID)
	if err != nil {
		return Message{}, fmt.Errorf("append message: next thread seq: %w", err)
	}

	row, err := q.AppendMessage(ctx, sqlc.AppendMessageParams{
		ThreadID:  in.ThreadID,
		ThreadSeq: seq,
		Actor:     in.Actor,
		Role:      in.Role,
		Content:   in.Content,
		SourceRef: nullString(in.SourceRef),
		CreatedAt: in.CreatedAt,
	})
	if err != nil {
		return Message{}, fmt.Errorf("append message: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Message{}, fmt.Errorf("append message: commit: %w", err)
	}

	return toMessage(row), nil
}

// ReadRange returns the messages of a thread whose global seq falls in
// [startSeq, endSeq], oldest first. startSeq/endSeq are inclusive.
func (c *Conversations) ReadRange(ctx context.Context, threadID string, startSeq, endSeq int64) ([]Message, error) {
	if threadID == "" {
		return nil, fmt.Errorf("read range: empty thread id")
	}
	if startSeq > endSeq {
		return nil, fmt.Errorf("read range: start %d after end %d", startSeq, endSeq)
	}

	rows, err := c.q.ReadRange(ctx, sqlc.ReadRangeParams{
		ThreadID: threadID,
		Seq:      startSeq,
		Seq_2:    endSeq,
	})
	if err != nil {
		return nil, fmt.Errorf("read range: %w", err)
	}

	msgs := make([]Message, 0, len(rows))
	for _, r := range rows {
		msgs = append(msgs, toMessage(r))
	}
	return msgs, nil
}

// GetThread returns a thread by id, or sql.ErrNoRows.
func (c *Conversations) GetThread(ctx context.Context, id string) (Thread, error) {
	t, err := c.q.GetThread(ctx, id)
	if err != nil {
		return Thread{}, err
	}
	return toThread(t), nil
}

// ListThreads returns all threads in creation order.
func (c *Conversations) ListThreads(ctx context.Context) ([]Thread, error) {
	rows, err := c.q.ListThreads(ctx)
	if err != nil {
		return nil, err
	}
	threads := make([]Thread, 0, len(rows))
	for _, r := range rows {
		threads = append(threads, toThread(r))
	}
	return threads, nil
}

// RecentMessages returns the most recent limit messages of a thread, oldest
// first. limit is a safety cap on the read, not a correctness bound.
func (c *Conversations) RecentMessages(ctx context.Context, threadID string, limit int) ([]Message, error) {
	if threadID == "" {
		return nil, fmt.Errorf("recent messages: empty thread id")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("recent messages: non-positive limit %d", limit)
	}

	rows, err := c.q.LatestMessages(ctx, sqlc.LatestMessagesParams{
		ThreadID: threadID,
		Limit:    int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("recent messages: %w", err)
	}

	msgs := make([]Message, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		msgs = append(msgs, toMessage(rows[i]))
	}
	return msgs, nil
}

func toMessage(r sqlc.Conversation) Message {
	return Message{
		Seq:       r.Seq,
		ThreadID:  r.ThreadID,
		ThreadSeq: r.ThreadSeq,
		Actor:     r.Actor,
		Role:      r.Role,
		Content:   r.Content,
		SourceRef: r.SourceRef.String,
		CreatedAt: r.CreatedAt,
	}
}

func toThread(r sqlc.Thread) Thread {
	return Thread{
		ID:             r.ID,
		SessionID:      r.SessionID,
		ParentThreadID: r.ParentThreadID.String,
		TaskID:         r.TaskID.String,
		Kind:           r.Kind,
		Brief:          r.Brief.String,
		Subject:        r.Subject.String,
		Distilled:      r.Distilled != 0,
		CreatedAt:      r.CreatedAt,
		CompletedAt:    r.CompletedAt.String,
	}
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
