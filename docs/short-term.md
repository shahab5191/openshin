# Short-Term Context — Spec

Status: **implemented** (phase 2).

This document describes how openshin assembles the short-term context an agent
pulls when it starts a task. It builds on the raw log from phase 1.

## What this phase solves

A conversation grows without bound, but the context an agent pulls must stay
small enough to be useful and cheap. Short-term memory is the answer: keep the
most recent messages raw, and roll everything older into a short summary, so the
window is always bounded but nothing is thrown away.

## How it works

`internal/memory/shortterm.go` assembles the window from a thread's recent
messages:

1. Load the most recent messages of the thread.
2. If they fit under the **token budget**, return them all raw.
3. Otherwise split: the **newest** messages stay raw (up to the recent-window
   budget), and the **older** ones are summarised by the model into a
   "rolling summary".

The result is a `Context`: a rolling summary (if the window was long) plus the
recent raw messages. It never exceeds the budget, and no message is ever
dropped — the older ones live on inside the summary, and the full text is still
in the raw log.

Token counting is a rough estimate (about four characters per token). It exists
to bound cost, not to be exact.

## What is built

- **`internal/memory/shortterm.go`** — the `ShortTerm` layer and the `Context`
  it returns. Budgets are constants: 8000 tokens total, 4000 for the recent raw
  window.
- **`internal/llm/gemini.go`** — the first provider: Gemini, exposing
  `Summarize` for the rolling summary. Configured from `GEMINI_API_KEY` /
  `GEMINI_MODEL`. (More providers come later.)
- **`store.RecentMessages`** — reads the newest N messages of a thread.
- **`GET /v1/threads/{id}/context`** — the API endpoint an agent calls to pull
  the assembled context.

## How it is tested

- A short window (under budget) returns all messages raw, no summary.
- A long window returns a rolling summary plus a trimmed recent window.
- An empty thread returns an empty context.
- The API endpoint serves whatever the assembler returns.

## Out of scope

- Promotion into mid-term (the *reason* a window shrinks permanently) is phase 3.
- Focus, mid-term, and long-term blocks in the context are later phases; here
  the context is short-term only.
- Caching the rolling summary — it is recomputed per request for now.
