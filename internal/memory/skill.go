package memory

import (
	"context"
	"math"
	"strings"

	"github.com/shahab5191/openshin/internal/llm"
	"github.com/shahab5191/openshin/internal/store"
)

// skillExtractor generalises a subagent conversation into a procedure.
type skillExtractor interface {
	ExtractSkill(ctx context.Context, text string) (llm.Skill, error)
}

// skillStore is what the skill layer needs from the store.
type skillStore interface {
	ListUndistilledSubagentThreads(ctx context.Context) ([]store.ThreadRef, error)
	MarkThreadDistilled(ctx context.Context, id string) error
	UpsertSkill(ctx context.Context, s store.Skill) error
	MessagesBetween(ctx context.Context, threadID string, start, end int64) ([]store.Message, error)
}

// SkillMemory distils completed subagent threads into reusable procedures.
// Repeated patterns merge into one skill with a growing use count.
type SkillMemory struct {
	store   skillStore
	extract skillExtractor
	embed   embedder
}

func NewSkillMemory(store skillStore, extract skillExtractor, embed embedder) *SkillMemory {
	return &SkillMemory{store: store, extract: extract, embed: embed}
}

// Run distils every completed, not-yet-distilled subagent thread.
func (s *SkillMemory) Run(ctx context.Context) error {
	refs, err := s.store.ListUndistilledSubagentThreads(ctx)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if err := s.Promote(ctx, ref); err != nil {
			return err
		}
	}
	return nil
}

// Promote distils one subagent thread into a skill, if it contains a reusable
// procedure.
func (s *SkillMemory) Promote(ctx context.Context, ref store.ThreadRef) error {
	msgs, err := s.store.MessagesBetween(ctx, ref.ID, 0, math.MaxInt64)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return s.store.MarkThreadDistilled(ctx, ref.ID)
	}

	skill, err := s.extract.ExtractSkill(ctx, renderAll(msgs))
	if err != nil {
		return err
	}
	if strings.TrimSpace(skill.Name) == "" {
		return s.store.MarkThreadDistilled(ctx, ref.ID)
	}

	emb, err := s.embed.Embed(ctx, skill.Name+" "+skill.Trigger+" "+skill.Steps)
	if err != nil {
		return err
	}

	if err := s.store.UpsertSkill(ctx, store.Skill{
		SessionID:    ref.SessionID,
		Name:         skill.Name,
		Trigger:      skill.Trigger,
		Steps:        skill.Steps,
		Embedding:    store.EncodeEmbedding(emb),
		SourceThread: ref.ID,
		NameHash:     hashFact(skill.Name),
	}); err != nil {
		return err
	}

	return s.store.MarkThreadDistilled(ctx, ref.ID)
}
