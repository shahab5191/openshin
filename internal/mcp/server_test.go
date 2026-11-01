package mcp

import (
	"context"
	"testing"

	"github.com/shahab5191/openshin/internal/db/testdb"
	"github.com/shahab5191/openshin/internal/memory"
	"github.com/shahab5191/openshin/internal/store"
)

type fakeEmbedder struct{}

func (fakeEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{1, 2, 3}, nil
}

func TestRememberStoresFact(t *testing.T) {
	mem := store.NewMemory(testdb.Open(t))
	ctx := context.Background()

	if err := remember(ctx, mem, fakeEmbedder{}, "s1", "the API key lives in .env"); err != nil {
		t.Fatalf("remember: %v", err)
	}

	items, err := mem.ListLongTerm(ctx, "s1")
	if err != nil || len(items) != 1 {
		t.Fatalf("expected 1 item, got %d (err %v)", len(items), err)
	}
	if items[0].RoleTag != "fact" || items[0].Content != "the API key lives in .env" {
		t.Fatalf("unexpected item: %+v", items[0])
	}
}

func TestRenderSearchTags(t *testing.T) {
	out := renderSearch([]memory.SearchResult{
		{Content: "do not touch auth", RoleTag: "constraint"},
		{Content: "apples", RoleTag: ""},
	})
	if out != "[constraint] do not touch auth\n[fact] apples" {
		t.Fatalf("unexpected render: %q", out)
	}
}

func TestRenderContext(t *testing.T) {
	out := renderContext(memory.Context{
		RollingSummary: "talked about auth",
		Messages:       []store.Message{{Role: "user", Content: "hello"}},
	})
	if out != "summary: talked about auth\n\nuser: hello" {
		t.Fatalf("unexpected render: %q", out)
	}
}
