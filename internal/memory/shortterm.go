// Package memory holds the memory layers. Each layer assembles the context it
// is responsible for; nothing here injects anything into an agent — agents pull
// context through the API and MCP tools.
package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/shahab5191/openshin/internal/store"
)

// Short-term budget, in approximate tokens. The window keeps the most recent
// messages raw up to recentWindowBudget, and rolls everything older into a
// summary, so the whole short-term context stays under shortTermBudget.
const (
	shortTermBudget    = 8000
	recentWindowBudget = 4000
	maxRecentMessages  = 500
)

// Context is the assembled short-term context: the raw recent messages plus,
// when the window grew past the budget, a rolling summary of everything older.
type Context struct {
	RollingSummary string         `json:"rolling_summary,omitempty"`
	Messages       []store.Message `json:"messages"`
}

// summarizer compresses older messages into the rolling summary.
type summarizer interface {
	Summarize(ctx context.Context, text string) (string, error)
}

// shortTermStore reads recent messages of a thread.
type shortTermStore interface {
	RecentMessages(ctx context.Context, threadID string, limit int) ([]store.Message, error)
}

// ShortTerm assembles the short-term context window.
type ShortTerm struct {
	store     shortTermStore
	summarize summarizer
}

func NewShortTerm(store shortTermStore, s summarizer) *ShortTerm {
	return &ShortTerm{store: store, summarize: s}
}

// Assemble returns the short-term context for a thread. If everything fits
// under the budget it is returned raw; otherwise the newest messages stay raw
// and the rest are rolled into a summary, so the result never exceeds the
// budget.
func (s *ShortTerm) Assemble(ctx context.Context, threadID string) (Context, error) {
	msgs, err := s.store.RecentMessages(ctx, threadID, maxRecentMessages)
	if err != nil {
		return Context{}, fmt.Errorf("short-term: load: %w", err)
	}
	if len(msgs) == 0 {
		return Context{}, nil
	}

	if totalTokens(msgs) <= shortTermBudget {
		return Context{Messages: msgs}, nil
	}

	idx := len(msgs)
	used := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		tk := estimateTokens(renderMessage(msgs[i]))
		if used+tk > recentWindowBudget && i != len(msgs)-1 {
			break
		}
		used += tk
		idx = i
	}

	recent := msgs[idx:]
	older := msgs[:idx]

	summary, err := s.summarize.Summarize(ctx, renderAll(older))
	if err != nil {
		return Context{}, fmt.Errorf("short-term: summarize: %w", err)
	}

	return Context{RollingSummary: summary, Messages: recent}, nil
}

// estimateTokens is a rough count: about four characters per token. Good enough
// for budgeting, no external tokenizer required.
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}

func totalTokens(msgs []store.Message) int {
	n := 0
	for _, m := range msgs {
		n += estimateTokens(renderMessage(m))
	}
	return n
}

func renderMessage(m store.Message) string {
	return m.Role + ": " + m.Content
}

func renderAll(msgs []store.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(renderMessage(m))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
