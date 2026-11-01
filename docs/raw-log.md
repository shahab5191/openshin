# Raw Log & Ingest — Spec

Status: **implemented** (phase 1).

This document describes phase 1 of openshin: the raw conversation log and the
two ways data gets into it. It is written for a human reader, not as a reference
for the code — every design choice below maps to a specific file.

## What this phase solves

Every other layer (short-term, mid-term, long-term, skill memory) is an *index
over the raw log*. None of them can exist until conversations are stored,
queryable, and lossless. Phase 1 delivers exactly that foundation: a place every
conversation lands, and a way to read any slice of it back unchanged.

## What is built

### The schema (`migrations/00001_create_raw_log.sql`)

Three tables:

- **`session`** — one row per root unit (a Claude Code session, or a user
  session). It exists so everything is scoped to a session, which is what lets
  the memory be shared across many sessions later.
- **`thread`** — one row per conversation thread. The *root* thread is the
  user ⇄ orchestrator conversation; *subagent* threads hang off it via
  `parent_thread_id`, forming a tree for fanout. `kind` is the routing switch
  (`user` → fact path, `subagent` → skill path) that later phases rely on.
- **`conversation`** — one row per message. `seq` is a global monotonic order;
  `thread_seq` is the per-thread order (the number every window sorts by).
  `source_ref` is an idempotency key so a replayed ingest never duplicates.

Nothing is ever deleted from these tables. That is the lossless guarantee the
whole system is built on.

### The store (`internal/store/conversations.go`)

Two operations matter here:

- **`AppendMessage`** — takes an agent-agnostic `IngestMessage`, ensures the
  session and thread exist, assigns the next `thread_seq`, and inserts the
  message, all in one transaction. If a message with the same `source_ref`
  already exists, it returns the existing row instead of writing a duplicate.
- **`ReadRange`** — returns the messages of a thread within an inclusive
  `[start_seq, end_seq]` window, oldest first.

### Ingest — two paths, one contract

Both paths reduce to `store.IngestMessage`:

1. **HTTP API** (`internal/api/server.go`) — `POST /v1/ingest` accepts a message
   and `GET /v1/threads/{id}/messages` reads it back. This is the agent-agnostic
   contract any agent or hook can call.
2. **Transcript tailer** (`internal/ingest/`) — reads a Claude Code transcript
   JSONL file incrementally. It remembers how far it got in a sidecar
   `.offset` file, so re-runs are cheap and idempotent. It parses user and
   assistant text lines and skips everything else (tool events, system records —
   those are handled in a later phase).

### The command (`cmd/openshin/main.go`)

- `openshin serve` runs the HTTP API.
- `openshin tail <transcript.jsonl>` ingests new lines from a transcript.

## How it is tested

- `internal/store` — appends create session/thread, assign sequential
  `thread_seq`, dedupe by `source_ref`, and `read_range` returns content
  byte-for-byte (lossless).
- `internal/ingest` — the parser extracts user/assistant text and skips tool and
  system lines; the tailer is incremental and idempotent across runs.
- `internal/api` — an HTTP round-trip: ingest then read it back.

## What is explicitly out of scope here

- Tool calls and subagent spawns (the fanout tree) — later, in agent alignment.
- Any *memory* logic: short-term windows, extraction, decay. This phase only
  stores and reads raw conversations.
