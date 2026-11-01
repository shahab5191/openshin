# Mid-Term Memory — Spec

Status: **implemented** (phase 3).

This document describes the mid-term layer: the decaying memory of recent
conversation. It is the write side of the fact path — it turns finished
segments of the short-term window into atomic facts, and lets them expire.

## What this phase solves

Short-term keeps recent messages raw. But raw messages are only useful for a
short time; what a future task really needs is the *durable content* of those
messages, stripped of the chatter. Mid-term distils finished stretches of
conversation into standalone facts that can be retrieved later, and drops the
noise.

## The two triggers

A thread's messages since the last processed point are the "open window". That
window is turned into a segment when **either** happens first:

1. **Focus change** — the one-sentence subject of the window (extracted by the
   model) differs from the thread's stored subject. The conversation moved on,
   so the previous stretch is settled.
2. **Ceiling** — the open window exceeds a token budget, forcing a segment even
   if the subject never changed (a long-running single topic).

## The two axes are independent

This is the key design point. When a segment is processed:

- **Promotion** (the segment is settled, so it leaves the open window) always
  happens.
- **Extraction** (facts are produced) happens only if the model finds durable
  content. A segment of greetings yields an **empty** fact list — it still
  promotes, it just stores nothing.

Separating the two is what lets trivial conversation be dropped without ever
getting stuck waiting for "something worth keeping".

## Facts

Each fact is one standalone, self-contained sentence, stored as its own row
with:

- a **back-reference** `[start_seq, end_seq]` to the messages that produced it;
- an **embedding** (for later retrieval);
- a **fact hash** so the same fact stated twice dedupes;
- an **expiry time** (TTL) — mid-term is a *decaying* memory. Durable facts are
  long-term's job (phase 4).

## What is built

- `migrations/00002_create_mid_term.sql` — `segment`, `mid_term_fact`, and a
  `subject` column on `thread`.
- `internal/store/memory.go` — `Memory` store: list/commit segments, upsert
  facts (deduped by hash), sweep expired facts, and embedding encode/decode.
- `internal/memory/midterm.go` — the `MidTerm` layer: `Promote` (one thread) and
  `Run` (all user threads + expiry sweep). Subagent threads are skipped — they
  feed skill memory, not facts.
- `internal/llm/gemini.go` — `ExtractSubject`, `ExtractFacts` (empty list
  allowed), `Embed`.
- `serve` runs the promotion pass on an interval (`-promote-every`, default 5s).

## How it is tested

- A subject change commits a segment and stores its facts with the right seq
  range.
- An unchanged subject under the ceiling commits nothing.
- Exceeding the ceiling forces a segment.
- An empty fact list still commits the segment but stores no facts.
- `Run` skips subagent threads.
- Expired facts are swept; duplicates collapse to one row.

## Out of scope

- **Retrieval** — searching the stored facts (and long-term) is the next phase.
- The precise *cut point* within the window: today the whole open window becomes
  one segment on a boundary. A sharper subject-boundary split is future work.
- Long-term distillation and skill memory.
