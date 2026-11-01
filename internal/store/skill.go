package store

import (
	"context"

	"github.com/shahab5191/openshin/internal/db/sqlc"
)

// ThreadRef is a subagent thread the skill layer has yet to distill.
type ThreadRef struct {
	ID        string
	SessionID string
}

// Skill is one reusable procedure.
type Skill struct {
	ID           int64  `json:"id,omitempty"`
	SessionID    string `json:"session_id"`
	Name         string `json:"name"`
	Trigger      string `json:"trigger"`
	Steps        string `json:"steps"`
	Embedding    []byte `json:"-"`
	SourceThread string `json:"source_thread,omitempty"`
	UseCount     int64  `json:"use_count"`
	NameHash     string `json:"-"`
}

// ListUndistilledSubagentThreads returns completed subagent threads the skill
// layer has not distilled yet.
func (m *Memory) ListUndistilledSubagentThreads(ctx context.Context) ([]ThreadRef, error) {
	rows, err := m.q.ListUndistilledSubagentThreads(ctx)
	if err != nil {
		return nil, err
	}
	refs := make([]ThreadRef, 0, len(rows))
	for _, r := range rows {
		refs = append(refs, ThreadRef{ID: r.ID, SessionID: r.SessionID})
	}
	return refs, nil
}

// MarkThreadDistilled records that the skill layer has processed a thread.
func (m *Memory) MarkThreadDistilled(ctx context.Context, id string) error {
	return m.q.MarkThreadDistilled(ctx, id)
}

// CompleteThread marks a thread finished, making it eligible for skill
// distillation.
func (m *Memory) CompleteThread(ctx context.Context, id string) error {
	return m.q.CompleteThread(ctx, id)
}

// UpsertSkill stores a skill, merging repeated patterns by name (incrementing
// use_count).
func (m *Memory) UpsertSkill(ctx context.Context, s Skill) error {
	return m.q.UpsertSkill(ctx, sqlc.UpsertSkillParams{
		SessionID:    s.SessionID,
		Name:         s.Name,
		Trigger:      s.Trigger,
		Steps:        s.Steps,
		Embedding:    s.Embedding,
		SourceThread: nullString(s.SourceThread),
		NameHash:     s.NameHash,
	})
}

// ListSkills returns a session's skills, most-used first.
func (m *Memory) ListSkills(ctx context.Context, sessionID string) ([]Skill, error) {
	rows, err := m.q.ListSkills(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	skills := make([]Skill, 0, len(rows))
	for _, r := range rows {
		skills = append(skills, Skill{
			ID:           r.ID,
			SessionID:    r.SessionID,
			Name:         r.Name,
			Trigger:      r.Trigger,
			Steps:        r.Steps,
			Embedding:    r.Embedding,
			SourceThread: r.SourceThread.String,
			UseCount:     r.UseCount,
			NameHash:     r.NameHash,
		})
	}
	return skills, nil
}
