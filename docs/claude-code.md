# Claude Code Alignment — Spec

Status: **implemented** (phase 6).

This document describes how openshin connects to an agent (Claude Code is the
reference). It covers the MCP tools, the hooks, and the wiring a developer
applies once.

## The model in one line

openshin never injects context into the agent. The agent *pulls* what it needs
through MCP tools, and openshin records everything losslessly in the background.
"Always check memory" is a behaviour the agent is *instructed* to follow, not
something the daemon enforces.

## 1. MCP tools

`openshin mcp` serves four tools over the MCP stdio transport, all backed by the
same types the HTTP API uses:

| Tool | What it returns |
|---|---|
| `openshin_get_context` | assembled short-term context (rolling summary + recent messages) |
| `openshin_search` | top-K memories by similarity — facts, decisions, constraints, skills |
| `openshin_read_range` | raw conversation in a `[start_seq, end_seq]` range |
| `openshin_remember` | persists an explicit durable fact into long-term memory |

## 2. Wiring (once per machine)

Build the binary and register the MCP server:

```sh
go build -o ~/bin/openshin ./cmd/openshin
claude mcp add openshin -- ~/bin/openshin mcp
```

Run the daemon so the background pipeline (mid/long/skill distillation) runs
and the HTTP ingest endpoint is available:

```sh
OPENSHIN_DB_PATH=~/.openshin/memory.db GEMINI_API_KEY=... openshin serve -addr :8080
```

## 3. "Always check memory" (CLAUDE.md)

Add to the project's `CLAUDE.md` so every agent checks memory before working:

```markdown
## Memory

Before starting a task, call `openshin_get_context` for the current session.
When touching existing code or decisions, call `openshin_search` for relevant
constraints, decisions, and past skills. If you learn a durable fact or decision,
persist it with `openshin_remember`.
```

## 4. Hooks (lossless ingest)

A `Stop` hook tails the session transcript after each response, so every turn is
recorded without the agent doing anything:

```jsonc
// ~/.claude/settings.json
{
  "hooks": {
    "Stop": [
      {
        "hooks": [{ "type": "command", "command": "openshin hook" }]
      }
    ]
  }
}
```

The hook command reads the payload from stdin, extracts `transcript_path`, and
ingests any new lines (`cmd/openshin` `hook`). The same transcript can also be
ingested manually with `openshin tail <transcript.jsonl>`.

## 5. Fanout (subagents)

Subagent threads are created via the HTTP ingest API with `kind: "subagent"` and
a `parent_thread_id`, and completed via `CompleteThread` (or a future hook on
`SubagentStop`). Completed subagent threads feed skill memory, never the fact
path. Full transcript-based spawn detection is not implemented yet — this phase
provides the tool surface and wiring; the tailer's fanout parsing is follow-up.

## 6. Verification (manual, live)

- Start a Claude Code session with the MCP server registered; confirm
  `openshin_get_context` returns the session context and `openshin_search`
  recalls earlier facts.
- Confirm a `Stop` hook ingests the transcript (rows appear in the DB).
- Confirm `openshin_remember` persists and is later returned by search.
