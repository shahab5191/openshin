# Long-Term Memory & Retrieval — Spec

Status: **implemented** (phase 4).

This document describes long-term memory (durable, role-tagged knowledge) and
the retrieval that surfaces it. It is the first phase that *reads* memory back.

## What this phase solves

Mid-term facts decay; a task that comes back weeks later would find them gone.
Long-term keeps what matters permanently, and labels *what kind* of thing it is,
so a new task can ask "what has been decided, and what must not be broken?"
rather than just "what is similar?"

## Long-term extraction

Long-term reads the **full log** — the raw messages of each segment — not
mid-term's facts. For every segment mid-term has finished, it distils the raw
messages into role-tagged items:

- **fact** — a stable truth about the user's world;
- **decision** — a choice that was made and its reasoning;
- **constraint** — a boundary or rule that must not be violated.

Each item is stored once (deduped by content hash), with an embedding and a
back-reference to the segment's message range. Segments are marked `distilled`
so a pass only reads new ones — re-running is idempotent.

## Retrieval (search)

`search` embeds the query and ranks every active memory (non-expired mid-term
facts + long-term items) by cosine similarity, returning the top-K. Mid-term
facts come back untagged; long-term items come back tagged, so the agent can see
"this is a *constraint*, treat it accordingly." The ranking is brute-force,
in-process, exact — fine at local scale.

This is the mechanism behind "constraints/decisions surface on a new task":
an agent calls search with the new task, and the relevant decisions and
constraints come back first, labelled.

## What is built

- `migrations/00003_create_long_term.sql` — `long_term_memory` table and a
  `distilled` flag on `segment`.
- `internal/store/longterm.go` — long-term items, undistilled-segment listing,
  active-fact listing, messages-between.
- `internal/memory/longterm.go` — the `LongTerm` layer (distil each segment).
- `internal/memory/retriever.go` — cosine search over mid-term + long-term.
- `internal/llm/gemini.go` — `ExtractLongTerm` (role-tagged JSON output).
- `POST /v1/search` — the retrieval endpoint.

## How it is tested

- Distilling a segment stores role-tagged items and marks the segment distilled.
- An unexpected role tag from the model is normalised to `fact`.
- Re-running distillation is idempotent (no duplicate items).
- Search ranks by similarity (query embedding pointing at "apples" returns
  "apples" first).
- Search surfaces the `constraint` role tag.

## Out of scope

- **Semantic near-duplicate merging** — today dedup is exact (content hash);
  merging two differently-worded statements of the same thing is future work.
- **Skill memory** (procedures) — the next phase, fed by AI activity.
- **MCP tool surface** — search is HTTP-only until agent alignment.
