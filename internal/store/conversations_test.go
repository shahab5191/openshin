package store

import (
	"context"
	"testing"

	"github.com/shahab5191/openshin/internal/db/testdb"
)

func TestAppendMessageCreatesSessionAndThread(t *testing.T) {
	c := NewConversations(testdb.Open(t))
	ctx := context.Background()

	msg, err := c.AppendMessage(ctx, IngestMessage{
		SessionID: "s1",
		Actor:     "user",
		Role:      RoleUser,
		Content:   "hello",
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if msg.Seq == 0 {
		t.Fatal("expected non-zero seq")
	}
	if msg.ThreadSeq != 1 {
		t.Fatalf("expected thread_seq 1, got %d", msg.ThreadSeq)
	}
	if msg.ThreadID != "s1" {
		t.Fatalf("expected root thread id == session id, got %q", msg.ThreadID)
	}

	thread, err := c.GetThread(ctx, "s1")
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if thread.Kind != string(KindUser) {
		t.Fatalf("expected kind user, got %q", thread.Kind)
	}
}

func TestAppendMessageAssignsSequentialThreadSeq(t *testing.T) {
	c := NewConversations(testdb.Open(t))
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		msg, err := c.AppendMessage(ctx, IngestMessage{
			SessionID: "s1",
			Actor:     "user",
			Role:      RoleUser,
			Content:   "m",
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if msg.ThreadSeq != int64(i) {
			t.Fatalf("expected thread_seq %d, got %d", i, msg.ThreadSeq)
		}
	}
}

func TestAppendMessageIdempotentBySourceRef(t *testing.T) {
	c := NewConversations(testdb.Open(t))
	ctx := context.Background()

	first, err := c.AppendMessage(ctx, IngestMessage{
		SessionID: "s1",
		Actor:     "assistant",
		Role:      RoleAssistant,
		Content:   "answer",
		SourceRef: "uuid-1",
	})
	if err != nil {
		t.Fatalf("first append: %v", err)
	}

	again, err := c.AppendMessage(ctx, IngestMessage{
		SessionID: "s1",
		Actor:     "assistant",
		Role:      RoleAssistant,
		Content:   "answer",
		SourceRef: "uuid-1",
	})
	if err != nil {
		t.Fatalf("second append: %v", err)
	}
	if again.Seq != first.Seq {
		t.Fatalf("expected idempotent append to return same seq %d, got %d", first.Seq, again.Seq)
	}

	msgs, err := c.ReadRange(ctx, "s1", 1, 1<<62)
	if err != nil {
		t.Fatalf("read range: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 row after idempotent append, got %d", len(msgs))
	}
}

func TestReadRangeLossless(t *testing.T) {
	c := NewConversations(testdb.Open(t))
	ctx := context.Background()

	const content = "line one\nline two\twith\ttabs\nand unicode \u2603"
	if _, err := c.AppendMessage(ctx, IngestMessage{
		SessionID: "s1", Actor: "user", Role: RoleUser, Content: content,
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	msgs, err := c.ReadRange(ctx, "s1", 1, 1<<62)
	if err != nil {
		t.Fatalf("read range: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != content {
		t.Fatalf("content lost in round trip:\n got %q\nwant %q", msgs[0].Content, content)
	}
}

func TestReadRangeBounds(t *testing.T) {
	c := NewConversations(testdb.Open(t))
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		if _, err := c.AppendMessage(ctx, IngestMessage{
			SessionID: "s1", Actor: "user", Role: RoleUser, Content: "m",
		}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	msgs, err := c.ReadRange(ctx, "s1", 2, 4)
	if err != nil {
		t.Fatalf("read range: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages in [2,4], got %d", len(msgs))
	}
	for _, m := range msgs {
		if m.Seq < 2 || m.Seq > 4 {
			t.Fatalf("seq %d outside requested range", m.Seq)
		}
	}
}
