package memory

import (
	"context"
	"testing"

	"github.com/shahab5191/openshin/internal/db/testdb"
	"github.com/shahab5191/openshin/internal/llm"
	"github.com/shahab5191/openshin/internal/store"
)

type fakeSkillExtractor struct{ skill llm.Skill }

func (f *fakeSkillExtractor) ExtractSkill(ctx context.Context, text string) (llm.Skill, error) {
	return f.skill, nil
}

func seedSubagentThread(t *testing.T, mem *store.Memory, conv *store.Conversations, id string) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := conv.AppendMessage(ctx, store.IngestMessage{
			SessionID: "s1", ThreadID: id, Kind: "subagent",
			Actor: "subagent", Role: store.RoleAssistant, Content: "solved it by pinning the version",
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := mem.CompleteThread(ctx, id); err != nil {
		t.Fatalf("complete: %v", err)
	}
}

func TestSkillMemoryDistillsSubagentThread(t *testing.T) {
	db := testdb.Open(t)
	mem := store.NewMemory(db)
	conv := store.NewConversations(db)
	seedSubagentThread(t, mem, conv, "sub1")

	skill := NewSkillMemory(mem, &fakeSkillExtractor{skill: llm.Skill{
		Name: "pin the API version", Trigger: "dependency drift", Steps: "read go.mod, pin the version",
	}}, &fakeExtractors{embed: []float32{1}})

	if err := skill.Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	skills, err := mem.ListSkills(context.Background(), "s1")
	if err != nil || len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d (err %v)", len(skills), err)
	}
	if skills[0].Name != "pin the API version" || skills[0].UseCount != 1 {
		t.Fatalf("unexpected skill: %+v", skills[0])
	}
}

func TestSkillMemoryMergesRepeatedPattern(t *testing.T) {
	db := testdb.Open(t)
	mem := store.NewMemory(db)
	conv := store.NewConversations(db)
	seedSubagentThread(t, mem, conv, "sub1")
	seedSubagentThread(t, mem, conv, "sub2")

	skill := NewSkillMemory(mem, &fakeSkillExtractor{skill: llm.Skill{
		Name: "pin the API version", Trigger: "dependency drift", Steps: "read go.mod, pin the version",
	}}, &fakeExtractors{embed: []float32{1}})

	if err := skill.Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	skills, _ := mem.ListSkills(context.Background(), "s1")
	if len(skills) != 1 {
		t.Fatalf("expected patterns to merge into 1 skill, got %d", len(skills))
	}
	if skills[0].UseCount != 2 {
		t.Fatalf("expected use_count 2, got %d", skills[0].UseCount)
	}
}

func TestSkillMemorySkipsNothingReusable(t *testing.T) {
	db := testdb.Open(t)
	mem := store.NewMemory(db)
	conv := store.NewConversations(db)
	seedSubagentThread(t, mem, conv, "sub1")

	skill := NewSkillMemory(mem, &fakeSkillExtractor{skill: llm.Skill{}}, &fakeExtractors{embed: []float32{1}})
	if err := skill.Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	skills, _ := mem.ListSkills(context.Background(), "s1")
	if len(skills) != 0 {
		t.Fatalf("expected no skill, got %d", len(skills))
	}

	refs, _ := mem.ListUndistilledSubagentThreads(context.Background())
	if len(refs) != 0 {
		t.Fatalf("expected thread distilled even without a skill, got %d refs", len(refs))
	}
}
