package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/shahab5191/openshin/internal/db/testdb"
	"github.com/shahab5191/openshin/internal/store"
)

// fakeExtractors implements all three extraction interfaces with fixed answers,
// so a test can control what the "model" returns.
type fakeExtractors struct {
	subject string
	facts   []string
	embed   []float32
}

func (f *fakeExtractors) ExtractSubject(ctx context.Context, text string) (string, error) {
	return f.subject, nil
}

func (f *fakeExtractors) ExtractFacts(ctx context.Context, text string) ([]string, error) {
	return f.facts, nil
}

func (f *fakeExtractors) Embed(ctx context.Context, text string) ([]float32, error) {
	return f.embed, nil
}

func newMidTerm(t *testing.T, f *fakeExtractors) (*store.Memory, *store.Conversations, *MidTerm) {
	t.Helper()
	db := testdb.Open(t)
	mem := store.NewMemory(db)
	conv := store.NewConversations(db)
	return mem, conv, NewMidTerm(mem, f, f, f)
}

func seedThread(t *testing.T, c *store.Conversations, threadID, sessionID string, n int, content string) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		if _, err := c.AppendMessage(ctx, store.IngestMessage{
			SessionID: sessionID, ThreadID: threadID, Actor: "user", Role: store.RoleUser, Content: content,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func TestPromoteCreatesSegmentAndFacts(t *testing.T) {
	mem, conv, mid := newMidTerm(t, &fakeExtractors{
		subject: "building a CLI",
		facts:   []string{"fact one", "fact two"},
		embed:   []float32{1, 2, 3},
	})
	seedThread(t, conv, "s1", "s1", 3, "some message")

	thr := store.Thread{ID: "s1", SessionID: "s1", Kind: "user"}
	if err := mid.Promote(context.Background(), thr); err != nil {
		t.Fatalf("promote: %v", err)
	}

	segs, err := mem.ListSegments(context.Background(), "s1")
	if err != nil {
		t.Fatalf("list segments: %v", err)
	}
	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segs))
	}
	if segs[0].StartSeq != 1 || segs[0].EndSeq != 3 {
		t.Fatalf("expected segment [1,3], got [%d,%d]", segs[0].StartSeq, segs[0].EndSeq)
	}

	facts, err := mem.ListFacts(context.Background(), "s1")
	if err != nil {
		t.Fatalf("list facts: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}

	subject, err := mem.CurrentSubject(context.Background(), "s1")
	if err != nil {
		t.Fatalf("current subject: %v", err)
	}
	if subject != "building a CLI" {
		t.Fatalf("expected subject recorded, got %q", subject)
	}
}

func TestPromoteNoBoundaryWhenUnchangedAndUnderCeiling(t *testing.T) {
	mem, conv, mid := newMidTerm(t, &fakeExtractors{subject: "same", facts: []string{"f"}, embed: []float32{1}})
	seedThread(t, conv, "s1", "s1", 3, "message")
	thr := store.Thread{ID: "s1", SessionID: "s1", Kind: "user"}

	if err := mid.Promote(context.Background(), thr); err != nil {
		t.Fatalf("first promote: %v", err)
	}

	seedThread(t, conv, "s1", "s1", 2, "message")
	if err := mid.Promote(context.Background(), thr); err != nil {
		t.Fatalf("second promote: %v", err)
	}

	segs, _ := mem.ListSegments(context.Background(), "s1")
	if len(segs) != 1 {
		t.Fatalf("expected still 1 segment, got %d", len(segs))
	}
}

func TestPromoteCeilingForcesSegment(t *testing.T) {
	mem, conv, mid := newMidTerm(t, &fakeExtractors{subject: "same", facts: []string{"f"}, embed: []float32{1}})
	mid.ceiling = 40 // tiny ceiling so a few messages exceed it

	seedThread(t, conv, "s1", "s1", 3, "message")
	thr := store.Thread{ID: "s1", SessionID: "s1", Kind: "user"}
	if err := mid.Promote(context.Background(), thr); err != nil {
		t.Fatalf("first promote: %v", err)
	}

	seedThread(t, conv, "s1", "s1", 3, strings.Repeat("x", 100))
	if err := mid.Promote(context.Background(), thr); err != nil {
		t.Fatalf("second promote: %v", err)
	}

	segs, _ := mem.ListSegments(context.Background(), "s1")
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments (ceiling forced), got %d", len(segs))
	}
}

func TestPromoteEmptyFactsStillCommitsSegment(t *testing.T) {
	mem, conv, mid := newMidTerm(t, &fakeExtractors{subject: "small talk", facts: []string{}, embed: []float32{1}})
	seedThread(t, conv, "s1", "s1", 2, "thanks, bye")

	if err := mid.Promote(context.Background(), store.Thread{ID: "s1", SessionID: "s1", Kind: "user"}); err != nil {
		t.Fatalf("promote: %v", err)
	}

	segs, _ := mem.ListSegments(context.Background(), "s1")
	if len(segs) != 1 {
		t.Fatalf("expected a segment even with no facts, got %d", len(segs))
	}
	facts, _ := mem.ListFacts(context.Background(), "s1")
	if len(facts) != 0 {
		t.Fatalf("expected no facts, got %d", len(facts))
	}
}

func TestRunSkipsSubagentThreads(t *testing.T) {
	mem, conv, mid := newMidTerm(t, &fakeExtractors{subject: "topic", facts: []string{"f"}, embed: []float32{1}})
	seedThread(t, conv, "root", "s1", 2, "user conversation")

	// A subagent thread in the same session.
	if _, err := conv.AppendMessage(context.Background(), store.IngestMessage{
		SessionID: "s1", ThreadID: "sub", ParentThreadID: "root", Kind: "subagent",
		Actor: "subagent", Role: store.RoleAssistant, Content: "worker output",
	}); err != nil {
		t.Fatalf("seed subagent: %v", err)
	}

	if err := mid.Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	if segs, _ := mem.ListSegments(context.Background(), "root"); len(segs) != 1 {
		t.Fatalf("expected root segment, got %d", len(segs))
	}
	if segs, _ := mem.ListSegments(context.Background(), "sub"); len(segs) != 0 {
		t.Fatalf("expected no subagent segment, got %d", len(segs))
	}
}
