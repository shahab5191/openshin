// Package mcp exposes the memory surface as Model Context Protocol tools, so an
// agent (Claude Code or otherwise) can pull context and recall memories on
// demand. It is a thin adapter over the same types the HTTP API serves.
package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shahab5191/openshin/internal/memory"
	"github.com/shahab5191/openshin/internal/store"
)

type contextAssembler interface {
	Assemble(ctx context.Context, threadID string) (memory.Context, error)
}

type searcher interface {
	Search(ctx context.Context, sessionID, query string, topK int) ([]memory.SearchResult, error)
}

type embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// New builds an MCP server exposing the memory tools over the given deps.
func New(conv *store.Conversations, mem *store.Memory, assembler contextAssembler, search searcher, embed embedder) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "openshin", Version: "0.1.0"}, nil)

	mcp.AddTool(s, &mcp.Tool{Name: "openshin_get_context", Description: "Return the assembled short-term context for a conversation thread."},
		func(ctx context.Context, _ *mcp.CallToolRequest, args struct {
			ThreadID string `json:"thread_id" jsonschema:"the thread id (usually the session id)"`
		}) (*mcp.CallToolResult, any, error) {
			c, err := assembler.Assemble(ctx, args.ThreadID)
			if err != nil {
				return nil, nil, err
			}
			return textResult(renderContext(c)), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "openshin_search", Description: "Search stored memories (facts, decisions, constraints, skills) by similarity."},
		func(ctx context.Context, _ *mcp.CallToolRequest, args struct {
			SessionID string `json:"session_id" jsonschema:"the session id"`
			Query     string `json:"query" jsonschema:"what to search for"`
			TopK      int    `json:"top_k" jsonschema:"max results"`
		}) (*mcp.CallToolResult, any, error) {
			results, err := search.Search(ctx, args.SessionID, args.Query, args.TopK)
			if err != nil {
				return nil, nil, err
			}
			return textResult(renderSearch(results)), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "openshin_read_range", Description: "Read raw conversation messages in a seq range for a thread."},
		func(ctx context.Context, _ *mcp.CallToolRequest, args struct {
			ThreadID string `json:"thread_id" jsonschema:"the thread id"`
			StartSeq int64  `json:"start_seq" jsonschema:"first seq, inclusive"`
			EndSeq   int64  `json:"end_seq" jsonschema:"last seq, inclusive"`
		}) (*mcp.CallToolResult, any, error) {
			msgs, err := conv.ReadRange(ctx, args.ThreadID, args.StartSeq, args.EndSeq)
			if err != nil {
				return nil, nil, err
			}
			return textResult(renderMessages(msgs)), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "openshin_remember", Description: "Explicitly persist a durable fact into long-term memory."},
		func(ctx context.Context, _ *mcp.CallToolRequest, args struct {
			SessionID string `json:"session_id" jsonschema:"the session id"`
			Content   string `json:"content" jsonschema:"the fact to remember"`
		}) (*mcp.CallToolResult, any, error) {
			if err := remember(ctx, mem, embed, args.SessionID, args.Content); err != nil {
				return nil, nil, err
			}
			return textResult("remembered"), nil, nil
		})

	return s
}

// Serve runs the MCP server on the stdio transport until ctx is done.
func Serve(ctx context.Context, s *mcp.Server) error {
	return s.Run(ctx, &mcp.StdioTransport{})
}

// remember stores an explicit fact as a long-term item.
func remember(ctx context.Context, mem *store.Memory, embed embedder, sessionID, content string) error {
	emb, err := embed.Embed(ctx, content)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(content))
	return mem.UpsertLongTerm(ctx, store.LongTermItem{
		SessionID:   sessionID,
		Content:     content,
		RoleTag:     "fact",
		Embedding:   store.EncodeEmbedding(emb),
		ContentHash: hex.EncodeToString(sum[:]),
	})
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func renderContext(c memory.Context) string {
	var b strings.Builder
	if c.RollingSummary != "" {
		b.WriteString("summary: ")
		b.WriteString(c.RollingSummary)
		b.WriteString("\n\n")
	}
	for _, m := range c.Messages {
		fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Content)
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderSearch(results []memory.SearchResult) string {
	var b strings.Builder
	for _, r := range results {
		tag := r.RoleTag
		if tag == "" {
			tag = "fact"
		}
		fmt.Fprintf(&b, "[%s] %s\n", tag, r.Content)
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderMessages(msgs []store.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Content)
	}
	return strings.TrimRight(b.String(), "\n")
}
