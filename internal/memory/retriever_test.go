package memory

import (
	"context"
	"testing"

	"github.com/shahab5191/openshin/internal/db/testdb"
	"github.com/shahab5191/openshin/internal/store"
)

func TestSearchRanksBySimilarity(t *testing.T) {
	db := testdb.Open(t)
	mem := store.NewMemory(db)
	ctx := context.Background()

	if err := mem.CommitSegment(ctx, store.Segment{SessionID: "s1", ThreadID: "t1", StartSeq: 1, EndSeq: 2}, []store.Fact{
		{SessionID: "s1", ThreadID: "t1", Fact: "apples", Embedding: store.EncodeEmbedding([]float32{1, 0, 0}), StartSeq: 1, EndSeq: 2, FactHash: "a", ExpiresAt: "2099-01-01T00:00:00Z"},
		{SessionID: "s1", ThreadID: "t1", Fact: "bananas", Embedding: store.EncodeEmbedding([]float32{0, 1, 0}), StartSeq: 1, EndSeq: 2, FactHash: "b", ExpiresAt: "2099-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// The query embedding points at "apples", so apples must rank first.
	ret := NewRetriever(mem, &fakeExtractors{embed: []float32{1, 0, 0}})
	results, err := ret.Search(ctx, "s1", "apples", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Content != "apples" || results[0].Similarity < results[1].Similarity {
		t.Fatalf("expected apples first, got %+v", results)
	}
}

func TestSearchSurfacesRoleTags(t *testing.T) {
	db := testdb.Open(t)
	mem := store.NewMemory(db)
	ctx := context.Background()

	if err := mem.UpsertLongTerm(ctx, store.LongTermItem{
		SessionID: "s1", Content: "do not touch the auth module", RoleTag: "constraint",
		Embedding: store.EncodeEmbedding([]float32{1, 0, 0}), StartSeq: 1, EndSeq: 2, ContentHash: "c",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	ret := NewRetriever(mem, &fakeExtractors{embed: []float32{1, 0, 0}})
	results, err := ret.Search(ctx, "s1", "auth rules", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].RoleTag != "constraint" {
		t.Fatalf("expected constraint role tag, got %q", results[0].RoleTag)
	}
}

func TestSearchSurfacesSkill(t *testing.T) {
	db := testdb.Open(t)
	mem := store.NewMemory(db)
	ctx := context.Background()

	if err := mem.UpsertSkill(ctx, store.Skill{
		SessionID: "s1", Name: "pin the version", Trigger: "drift", Steps: "edit go.mod",
		Embedding: store.EncodeEmbedding([]float32{1, 0, 0}), NameHash: "s",
	}); err != nil {
		t.Fatalf("upsert skill: %v", err)
	}

	ret := NewRetriever(mem, &fakeExtractors{embed: []float32{1, 0, 0}})
	results, err := ret.Search(ctx, "s1", "how to pin", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].RoleTag != "skill" {
		t.Fatalf("expected a skill result, got %+v", results)
	}
}

func TestCosine(t *testing.T) {
	if got := cosine([]float32{1, 0}, []float32{1, 0}); got != 1 {
		t.Fatalf("identical vectors should be 1, got %v", got)
	}
	if got := cosine([]float32{1, 0}, []float32{0, 1}); got != 0 {
		t.Fatalf("orthogonal vectors should be 0, got %v", got)
	}
	if got := cosine([]float32{}, []float32{1}); got != 0 {
		t.Fatalf("mismatched/empty vectors should be 0, got %v", got)
	}
}
