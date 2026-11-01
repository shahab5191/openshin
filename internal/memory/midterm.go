package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/shahab5191/openshin/internal/store"
)

// Mid-term constants: the open window is forced into a segment when it exceeds
// midTermCeiling tokens; each fact expires after midTermTTL.
const (
	midTermCeiling  = 8000
	maxOpenMessages = 500
	midTermTTL      = 7 * 24 * time.Hour
)

// subjectExtractor returns the one-sentence subject of the open window.
type subjectExtractor interface {
	ExtractSubject(ctx context.Context, text string) (string, error)
}

// factExtractor distils durable facts from a finished segment.
type factExtractor interface {
	ExtractFacts(ctx context.Context, text string) ([]string, error)
}

// embedder computes embeddings for storage.
type embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// midTermStore is what the mid-term layer needs from the store.
type midTermStore interface {
	ListThreads(ctx context.Context) ([]store.Thread, error)
	MessagesAfter(ctx context.Context, threadID string, afterSeq int64, limit int) ([]store.Message, error)
	LastSegmentEnd(ctx context.Context, threadID string) (int64, error)
	CurrentSubject(ctx context.Context, threadID string) (string, error)
	CommitSegment(ctx context.Context, seg store.Segment, facts []store.Fact) error
	DeleteExpiredFacts(ctx context.Context, now string) (int64, error)
}

// MidTerm processes finished segments of the short-term window into decaying
// facts. It is the write side of the fact path; retrieval is a later phase.
type MidTerm struct {
	store   midTermStore
	subject subjectExtractor
	facts   factExtractor
	embed   embedder
	ceiling int
}

func NewMidTerm(store midTermStore, subject subjectExtractor, facts factExtractor, embed embedder) *MidTerm {
	return &MidTerm{store: store, subject: subject, facts: facts, embed: embed, ceiling: midTermCeiling}
}

// Run promotes every user thread and sweeps expired facts. It is the background
// maintenance pass the daemon runs on an interval.
func (m *MidTerm) Run(ctx context.Context) error {
	threads, err := m.store.ListThreads(ctx)
	if err != nil {
		return fmt.Errorf("mid-term: list threads: %w", err)
	}
	for _, t := range threads {
		if t.Kind != string(store.KindUser) {
			continue // subagent threads feed skill memory, not the fact path
		}
		if err := m.Promote(ctx, t); err != nil {
			return err
		}
	}
	if _, err := m.store.DeleteExpiredFacts(ctx, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("mid-term: sweep: %w", err)
	}
	return nil
}

// Promote turns a thread's open messages into a segment when the subject has
// changed or the open window exceeded the ceiling. A segment with no durable
// facts still commits — promotion (the window is settled) and extraction (it
// produced facts) are independent.
func (m *MidTerm) Promote(ctx context.Context, t store.Thread) error {
	last, err := m.store.LastSegmentEnd(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("mid-term: last segment: %w", err)
	}

	msgs, err := m.store.MessagesAfter(ctx, t.ID, last, maxOpenMessages)
	if err != nil {
		return fmt.Errorf("mid-term: open messages: %w", err)
	}
	if len(msgs) == 0 {
		return nil
	}

	subject, err := m.subject.ExtractSubject(ctx, renderAll(msgs))
	if err != nil {
		return fmt.Errorf("mid-term: subject: %w", err)
	}

	current, err := m.store.CurrentSubject(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("mid-term: current subject: %w", err)
	}

	if subject == current && totalTokens(msgs) <= m.ceiling {
		return nil // nothing settled yet, and under the ceiling
	}

	facts, err := m.extractFacts(ctx, t, msgs)
	if err != nil {
		return err
	}

	return m.store.CommitSegment(ctx, store.Segment{
		SessionID: t.SessionID,
		ThreadID:  t.ID,
		StartSeq:  msgs[0].Seq,
		EndSeq:    msgs[len(msgs)-1].Seq,
		Subject:   subject,
	}, facts)
}

func (m *MidTerm) extractFacts(ctx context.Context, t store.Thread, msgs []store.Message) ([]store.Fact, error) {
	raw, err := m.facts.ExtractFacts(ctx, renderAll(msgs))
	if err != nil {
		return nil, fmt.Errorf("mid-term: extract facts: %w", err)
	}

	expires := time.Now().Add(midTermTTL).UTC().Format(time.RFC3339)
	facts := make([]store.Fact, 0, len(raw))
	for _, f := range raw {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		emb, err := m.embed.Embed(ctx, f)
		if err != nil {
			return nil, fmt.Errorf("mid-term: embed: %w", err)
		}
		facts = append(facts, store.Fact{
			SessionID: t.SessionID,
			ThreadID:  t.ID,
			Fact:      f,
			Embedding: store.EncodeEmbedding(emb),
			StartSeq:  msgs[0].Seq,
			EndSeq:    msgs[len(msgs)-1].Seq,
			FactHash:  hashFact(f),
			ExpiresAt: expires,
		})
	}
	return facts, nil
}

func hashFact(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
