// Package ingest turns external agent output into store.IngestMessage. The HTTP
// endpoint in internal/api is the agent-agnostic contract; the Claude Code
// transcript parser here is one concrete adapter for it.
package ingest

import (
	"encoding/json"
	"strings"

	"github.com/shahab5191/openshin/internal/store"
)

// claudeRecord is the subset of a Claude Code transcript JSONL line that matters
// for the raw log: the message type, its identity, and any text it carries.
type claudeRecord struct {
	Type      string `json:"type"`
	UUID      string `json:"uuid"`
	SessionID string `json:"sessionId"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

// parseClaudeLine turns one transcript line into an ingest message, or ok=false
// when the line carries no text (a tool event, a system record, etc.).
//
// Phase 1 scope: only user/assistant text is captured. Tool calls and subagent
// spawns (the fanout tree) are handled in the agent-alignment phase.
func parseClaudeLine(line string) (store.IngestMessage, bool, error) {
	var rec claudeRecord
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		return store.IngestMessage{}, false, err
	}
	if rec.SessionID == "" || rec.UUID == "" {
		return store.IngestMessage{}, false, nil
	}

	var role, actor string
	switch rec.Type {
	case "user":
		role, actor = store.RoleUser, "user"
	case "assistant":
		role, actor = store.RoleAssistant, "orchestrator"
	default:
		return store.IngestMessage{}, false, nil
	}

	var b strings.Builder
	for _, block := range rec.Message.Content {
		if block.Type == "text" && block.Text != "" {
			b.WriteString(block.Text)
			b.WriteByte('\n')
		}
	}
	content := strings.TrimRight(b.String(), "\n")
	if content == "" {
		return store.IngestMessage{}, false, nil
	}

	return store.IngestMessage{
		SessionID: rec.SessionID,
		ThreadID:  rec.SessionID, // root thread id == session id
		Actor:     actor,
		Role:      role,
		Content:   content,
		SourceRef: rec.UUID,
		CreatedAt: rec.Timestamp,
	}, true, nil
}
