package store

import (
	"context"

	"github.com/shahab5191/openshin/internal/db/sqlc"
)

// SegmentRef is a segment long-term has yet to distill.
type SegmentRef struct {
	ID        int64
	SessionID string
	ThreadID  string
	StartSeq  int64
	EndSeq    int64
}

// LongTermItem is one role-tagged durable piece of knowledge.
type LongTermItem struct {
	ID          int64  `json:"id,omitempty"`
	SessionID   string `json:"session_id"`
	Content     string `json:"content"`
	RoleTag     string `json:"role_tag"`
	Embedding   []byte `json:"-"`
	StartSeq    int64  `json:"start_seq"`
	EndSeq      int64  `json:"end_seq"`
	ContentHash string `json:"-"`
}

// UndistilledSegments returns segments long-term has not distilled yet, oldest
// first.
func (m *Memory) UndistilledSegments(ctx context.Context) ([]SegmentRef, error) {
	rows, err := m.q.ListUndistilledSegments(ctx)
	if err != nil {
		return nil, err
	}
	refs := make([]SegmentRef, 0, len(rows))
	for _, r := range rows {
		refs = append(refs, SegmentRef{
			ID:        r.ID,
			SessionID: r.SessionID,
			ThreadID:  r.ThreadID,
			StartSeq:  r.StartSeq,
			EndSeq:    r.EndSeq,
		})
	}
	return refs, nil
}

// MarkDistilled records that long-term has processed a segment.
func (m *Memory) MarkDistilled(ctx context.Context, id int64) error {
	return m.q.MarkSegmentDistilled(ctx, id)
}

// MessagesBetween returns a thread's messages in [start, end], oldest first.
func (m *Memory) MessagesBetween(ctx context.Context, threadID string, start, end int64) ([]Message, error) {
	rows, err := m.q.MessagesBetween(ctx, sqlc.MessagesBetweenParams{
		ThreadID: threadID,
		Seq:      start,
		Seq_2:    end,
	})
	if err != nil {
		return nil, err
	}
	msgs := make([]Message, 0, len(rows))
	for _, r := range rows {
		msgs = append(msgs, toMessage(r))
	}
	return msgs, nil
}

// UpsertLongTerm stores a long-term item, deduped by content hash.
func (m *Memory) UpsertLongTerm(ctx context.Context, item LongTermItem) error {
	return m.q.UpsertLongTerm(ctx, sqlc.UpsertLongTermParams{
		SessionID:      item.SessionID,
		Content:        item.Content,
		RoleTag:        item.RoleTag,
		Embedding:      item.Embedding,
		SourceStartSeq: item.StartSeq,
		SourceEndSeq:   item.EndSeq,
		ContentHash:    item.ContentHash,
	})
}

// ListLongTerm returns a session's long-term items, oldest first.
func (m *Memory) ListLongTerm(ctx context.Context, sessionID string) ([]LongTermItem, error) {
	rows, err := m.q.ListLongTerm(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	items := make([]LongTermItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, LongTermItem{
			ID:          r.ID,
			SessionID:   r.SessionID,
			Content:     r.Content,
			RoleTag:     r.RoleTag,
			Embedding:   r.Embedding,
			StartSeq:    r.SourceStartSeq,
			EndSeq:      r.SourceEndSeq,
			ContentHash: r.ContentHash,
		})
	}
	return items, nil
}

// ListActiveFacts returns a session's non-expired mid-term facts.
func (m *Memory) ListActiveFacts(ctx context.Context, sessionID, now string) ([]Fact, error) {
	rows, err := m.q.ListActiveFacts(ctx, sqlc.ListActiveFactsParams{
		SessionID: sessionID,
		ExpiresAt: now,
	})
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
