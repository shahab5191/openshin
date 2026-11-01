package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/shahab5191/openshin/internal/db/testdb"
)

func TestCommitSegmentAndList(t *testing.T) {
	db := testdb.Open(t)
	m := NewMemory(db)
	ctx := context.Background()

	// The thread row must exist first (created by ingest in the real flow).
	if _, err := NewConversations(db).AppendMessage(ctx, IngestMessage{
		SessionID: "s1", ThreadID: "t1", Actor: "user", Role: RoleUser, Content: "x",
	}); err != nil {
		t.Fatalf("seed thread: %v", err)
	}

	seg := Segment{SessionID: "s1", ThreadID: "t1", StartSeq: 1, EndSeq: 3, Subject: "topic"}
	facts := []Fact{
		{SessionID: "s1", ThreadID: "t1", Fact: "fact a", Embedding: []byte{1, 2}, StartSeq: 1, EndSeq: 3, FactHash: "h1", ExpiresAt: "2099-01-01T00:00:00Z"},
		{SessionID: "s1", ThreadID: "t1", Fact: "fact b", Embedding: []byte{3, 4}, StartSeq: 1, EndSeq: 3, FactHash: "h2", ExpiresAt: "2099-01-01T00:00:00Z"},
	}

	if err := m.CommitSegment(ctx, seg, facts); err != nil {
		t.Fatalf("commit: %v", err)
	}

	segs, err := m.ListSegments(ctx, "t1")
	if err != nil || len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d (err %v)", len(segs), err)
	}
	if segs[0].EndSeq != 3 {
		t.Fatalf("expected end_seq 3, got %d", segs[0].EndSeq)
	}

	got, err := m.ListFacts(ctx, "s1")
	if err != nil || len(got) != 2 {
		t.Fatalf("expected 2 facts, got %d (err %v)", len(got), err)
	}

	subj, err := m.CurrentSubject(ctx, "t1")
	if err != nil || subj != "topic" {
		t.Fatalf("expected subject topic, got %q (err %v)", subj, err)
	}
}

func TestCommitSegmentDedupesByFactHash(t *testing.T) {
	m := NewMemory(testdb.Open(t))
	ctx := context.Background()

	fact := Fact{SessionID: "s1", ThreadID: "t1", Fact: "dup", StartSeq: 1, EndSeq: 2, FactHash: "same", ExpiresAt: "2099-01-01T00:00:00Z"}

	if err := m.CommitSegment(ctx, Segment{SessionID: "s1", ThreadID: "t1", StartSeq: 1, EndSeq: 2}, []Fact{fact}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := m.CommitSegment(ctx, Segment{SessionID: "s1", ThreadID: "t1", StartSeq: 3, EndSeq: 4}, []Fact{fact}); err != nil {
		t.Fatalf("second: %v", err)
	}

	got, _ := m.ListFacts(ctx, "s1")
	if len(got) != 1 {
		t.Fatalf("expected 1 deduped fact, got %d", len(got))
	}
}

func TestDeleteExpiredFacts(t *testing.T) {
	m := NewMemory(testdb.Open(t))
	ctx := context.Background()

	facts := []Fact{
		{SessionID: "s1", ThreadID: "t1", Fact: "old", FactHash: "old", ExpiresAt: "2020-01-01T00:00:00Z"},
		{SessionID: "s1", ThreadID: "t1", Fact: "fresh", FactHash: "fresh", ExpiresAt: "2099-01-01T00:00:00Z"},
	}
	if err := m.CommitSegment(ctx, Segment{SessionID: "s1", ThreadID: "t1", StartSeq: 1, EndSeq: 2}, facts); err != nil {
		t.Fatalf("commit: %v", err)
	}

	n, err := m.DeleteExpiredFacts(ctx, "2026-01-01T00:00:00Z")
	if err != nil || n != 1 {
		t.Fatalf("expected 1 expired deleted, got %d (err %v)", n, err)
	}

	got, _ := m.ListFacts(ctx, "s1")
	if len(got) != 1 || got[0].Fact != "fresh" {
		t.Fatalf("expected only the fresh fact, got %+v", got)
	}
}

func TestEncodeDecodeEmbedding(t *testing.T) {
	in := []float32{1.5, -2.25, 0.0, 3.14}
	got := DecodeEmbedding(EncodeEmbedding(in))
	if !reflect.DeepEqual(in, got) {
		t.Fatalf("round trip failed: got %v want %v", got, in)
	}
}
