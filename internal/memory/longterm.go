package memory

import (
	"context"
	"strings"

	"github.com/shahab5191/openshin/internal/llm"
	"github.com/shahab5191/openshin/internal/store"
)

// longTermExtractor distils role-tagged knowledge from a segment's raw messages.
type longTermExtractor interface {
	ExtractLongTerm(ctx context.Context, text string) ([]llm.TaggedItem, error)
}

// longTermStore is what the long-term layer needs from the store.
type longTermStore interface {
	UndistilledSegments(ctx context.Context) ([]store.SegmentRef, error)
	MarkDistilled(ctx context.Context, id int64) error
	MessagesBetween(ctx context.Context, threadID string, start, end int64) ([]store.Message, error)
	UpsertLongTerm(ctx context.Context, item store.LongTermItem) error
}

// LongTerm distils durable, role-tagged knowledge from the full log, framed by
// the segments mid-term produced. It reads raw messages, not mid-term's facts.
type LongTerm struct {
	store   longTermStore
	extract longTermExtractor
	embed   embedder
}

func NewLongTerm(store longTermStore, extract longTermExtractor, embed embedder) *LongTerm {
	return &LongTerm{store: store, extract: extract, embed: embed}
}

// Run distils every not-yet-distilled segment. Idempotent: content hashes make
// re-running safe.
func (l *LongTerm) Run(ctx context.Context) error {
	refs, err := l.store.UndistilledSegments(ctx)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if err := l.Promote(ctx, ref); err != nil {
			return err
		}
	}
	return nil
}

// Promote distils one segment into role-tagged long-term items.
func (l *LongTerm) Promote(ctx context.Context, ref store.SegmentRef) error {
	msgs, err := l.store.MessagesBetween(ctx, ref.ThreadID, ref.StartSeq, ref.EndSeq)
	if err != nil {
		return err
	}

	items, err := l.extract.ExtractLongTerm(ctx, renderAll(msgs))
	if err != nil {
		return err
	}

	for _, it := range items {
		content := strings.TrimSpace(it.Content)
		if content == "" {
			continue
		}
		emb, err := l.embed.Embed(ctx, content)
		if err != nil {
			return err
		}
		if err := l.store.UpsertLongTerm(ctx, store.LongTermItem{
			SessionID:   ref.SessionID,
			Content:     content,
			RoleTag:     normalizeRoleTag(it.RoleTag),
			Embedding:   store.EncodeEmbedding(emb),
			StartSeq:    ref.StartSeq,
			EndSeq:      ref.EndSeq,
			ContentHash: hashFact(content),
		}); err != nil {
			return err
		}
	}

	return l.store.MarkDistilled(ctx, ref.ID)
}

// normalizeRoleTag coerces an unexpected tag to "fact" so the DB constraint is
// never violated by a model's odd output.
func normalizeRoleTag(s string) string {
	switch s {
	case "decision", "constraint":
		return s
	default:
		return "fact"
	}
}
