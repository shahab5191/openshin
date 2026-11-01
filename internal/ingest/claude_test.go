package ingest

import "testing"

func TestParseClaudeLineUser(t *testing.T) {
	line := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":[{"type":"text","text":"hi there"}]}}`

	msg, ok, err := parseClaudeLine(line)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !ok {
		t.Fatal("expected a message")
	}
	if msg.Role != "user" || msg.Actor != "user" {
		t.Fatalf("wrong role/actor: %q/%q", msg.Role, msg.Actor)
	}
	if msg.Content != "hi there" {
		t.Fatalf("wrong content: %q", msg.Content)
	}
	if msg.SourceRef != "u1" || msg.SessionID != "s1" {
		t.Fatalf("wrong identity: %q %q", msg.SourceRef, msg.SessionID)
	}
}

func TestParseClaudeLineAssistant(t *testing.T) {
	line := `{"type":"assistant","uuid":"u2","sessionId":"s1","message":{"role":"assistant","content":[{"type":"text","text":"I will do that"}]}}`

	msg, ok, err := parseClaudeLine(line)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !ok {
		t.Fatal("expected a message")
	}
	if msg.Role != "assistant" || msg.Actor != "orchestrator" {
		t.Fatalf("wrong role/actor: %q/%q", msg.Role, msg.Actor)
	}
}

func TestParseClaudeLineJoinsTextBlocks(t *testing.T) {
	line := `{"type":"assistant","uuid":"u3","sessionId":"s1","message":{"role":"assistant","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}}`

	msg, ok, _ := parseClaudeLine(line)
	if !ok {
		t.Fatal("expected a message")
	}
	if msg.Content != "a\nb" {
		t.Fatalf("expected joined blocks, got %q", msg.Content)
	}
}

func TestParseClaudeLineSkipsToolUseOnly(t *testing.T) {
	line := `{"type":"assistant","uuid":"u4","sessionId":"s1","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{}}]}}`

	if _, ok, err := parseClaudeLine(line); err != nil || ok {
		t.Fatalf("expected skip, got ok=%v err=%v", ok, err)
	}
}

func TestParseClaudeLineSkipsSystem(t *testing.T) {
	line := `{"type":"system","subtype":"init","uuid":"u5","sessionId":"s1"}`

	if _, ok, err := parseClaudeLine(line); err != nil || ok {
		t.Fatalf("expected skip, got ok=%v err=%v", ok, err)
	}
}
