package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/shahab5191/openshin/internal/db/testdb"
	"github.com/shahab5191/openshin/internal/store"
)

type fakeSummarizer struct{ out string }

func (f *fakeSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	return f.out, nil
}

func seed(t *testing.T, c *store.Conversations, n int, content string) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		if _, err := c.AppendMessage(ctx, store.IngestMessage{
			SessionID: "s1", Actor: "user", Role: store.RoleUser, Content: content,
		}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
}

func TestAssembleSmallWindowStaysRaw(t *testing.T) {
	c := store.NewConversations(testdb.Open(t))
	seed(t, c, 5, "short message")

	st := NewShortTerm(c, &fakeSummarizer{out: "ROLLED"})
	got, err := st.Assemble(context.Background(), "s1")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if got.RollingSummary != "" {
		t.Fatalf("expected no summary for a small window, got %q", got.RollingSummary)
	}
	if len(got.Messages) != 5 {
		t.Fatalf("expected all 5 messages, got %d", len(got.Messages))
	}
}

func TestAssembleLargeWindowRollsSummary(t *testing.T) {
	c := store.NewConversations(testdb.Open(t))
	seed(t, c, 200, strings.Repeat("x", 200))

	st := NewShortTerm(c, &fakeSummarizer{out: "ROLLED"})
	got, err := st.Assemble(context.Background(), "s1")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if got.RollingSummary != "ROLLED" {
		t.Fatalf("expected rolling summary, got %q", got.RollingSummary)
	}
	if len(got.Messages) == 0 || len(got.Messages) >= 200 {
		t.Fatalf("expected a trimmed recent window, got %d messages", len(got.Messages))
	}
}

func TestAssembleEmptyThread(t *testing.T) {
	c := store.NewConversations(testdb.Open(t))
	st := NewShortTerm(c, &fakeSummarizer{out: "ROLLED"})

	got, err := st.Assemble(context.Background(), "s1")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(got.Messages) != 0 || got.RollingSummary != "" {
		t.Fatalf("expected empty context, got %+v", got)
	}
}
