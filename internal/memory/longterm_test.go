package memory

import (
	"context"
	"testing"

	"github.com/shahab5191/openshin/internal/db/testdb"
	"github.com/shahab5191/openshin/internal/llm"
	"github.com/shahab5191/openshin/internal/store"
)

type fakeLongTermExtractor struct{ items []llm.TaggedItem }

func (f *fakeLongTermExtractor) ExtractLongTerm(ctx context.Context, text string) ([]llm.TaggedItem, error) {
	return f.items, nil
}

func seedSegment(t *testing.T, mem *store.Memory, conv *store.Conversations, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		if _, err := conv.AppendMessage(ctx, store.IngestMessage{
			SessionID: "s1", ThreadID: "s1", Actor: "user", Role: store.RoleUser, Content: "message",
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := mem.CommitSegment(ctx, store.Segment{
		SessionID: "s1", ThreadID: "s1", StartSeq: 1, EndSeq: int64(n), Subject: "topic",
	}, nil); err != nil {
		t.Fatalf("commit segment: %v", err)
	}
}

func TestLongTermRunDistillsSegments(t *testing.T) {
	db := testdb.Open(t)
	mem := store.NewMemory(db)
	conv := store.NewConversations(db)
	seedSegment(t, mem, conv, 3)

	ext := &fakeLongTermExtractor{items: []llm.TaggedItem{
		{RoleTag: "constraint", Content: "the API version must stay pinned"},
		{RoleTag: "decision", Content: "use Go for the daemon"},
	}}
	long := NewLongTerm(mem, ext, &fakeExtractors{embed: []float32{1}})

	if err := long.Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	items, err := mem.ListLongTerm(context.Background(), "s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 long-term items, got %d", len(items))
	}
	if items[0].RoleTag != "constraint" {
		t.Fatalf("expected constraint, got %q", items[0].RoleTag)
	}

	refs, err := mem.UndistilledSegments(context.Background())
	if err != nil || len(refs) != 0 {
		t.Fatalf("expected segment distilled, got %d refs (err %v)", len(refs), err)
	}
}

func TestLongTermNormalizesUnknownRoleTag(t *testing.T) {
	db := testdb.Open(t)
	mem := store.NewMemory(db)
	conv := store.NewConversations(db)
	seedSegment(t, mem, conv, 1)

	ext := &fakeLongTermExtractor{items: []llm.TaggedItem{{RoleTag: "garbage", Content: "something"}}}
	long := NewLongTerm(mem, ext, &fakeExtractors{embed: []float32{1}})
	if err := long.Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	items, _ := mem.ListLongTerm(context.Background(), "s1")
	if len(items) != 1 || items[0].RoleTag != "fact" {
		t.Fatalf("expected normalized to fact, got %+v", items)
	}
}

func TestLongTermIsIdempotent(t *testing.T) {
	db := testdb.Open(t)
	mem := store.NewMemory(db)
	conv := store.NewConversations(db)
	seedSegment(t, mem, conv, 1)

	ext := &fakeLongTermExtractor{items: []llm.TaggedItem{{RoleTag: "fact", Content: "dup content"}}}
	long := NewLongTerm(mem, ext, &fakeExtractors{embed: []float32{1}})

	if err := long.Run(context.Background()); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := long.Run(context.Background()); err != nil {
		t.Fatalf("second run: %v", err)
	}

	items, _ := mem.ListLongTerm(context.Background(), "s1")
	if len(items) != 1 {
		t.Fatalf("expected 1 item after re-run, got %d", len(items))
	}
}
