# openshin — Architecture

Status: **spec** — phase 0 scaffold only; nothing below is implemented yet.

openshin is a **local, layered-memory daemon for agents**. It runs as a single
persistent process on a developer's machine, owns a SQLite file that holds every
conversation plus distilled memories, and lets an orchestrator agent (and the
subagents it fans out to) recall what has been done, what was decided, and what
might conflict — across sessions, for effectively unbounded history.

The agent is the brain. openshin is the memory.

---

## 1. What it is and is not

- **It is a memory sidecar**, not a chat server. There is no `/chat` endpoint and
  no request/response generation of its own. The surrounding agent does the
  thinking; openshin records and recalls.
- **It is pull-only.** openshin never injects context into an agent. Agents
  retrieve on demand through tools, and are *instructed* (not forced) to always
  check memory for relevant context. Nothing about the design relies on being
  able to clear or rewrite an agent's live context — which is exactly what tools
  like Claude Code do not allow.
- **It is local-first.** One daemon per machine, one SQLite file, no infra. The
  cloud (a fleet of agents sharing a memory service) is a later phase that reuses
  the same interfaces over a different backend.

## 2. Locked decisions

| Decision | Value |
|---|---|
| Runtime | Go, single static binary (`cmd/openshin`) |
| Storage | SQLite via `modernc.org/sqlite` (CGO-free) |
| Vector search | Brute-force cosine in-process (embeddings as `BLOB` float32) |
| Read model | Pull-only; no context injection |
| LLM/embedding | Provider abstraction; Gemini default |
| Memory lifetime | Raw log forever; mid-term TTL; long-term consolidates |
| Namespacing | `session_id` → `thread_id` (tree) → `task_id`, plus `actor` |
| Routing | `thread.kind` decides the destination (fact path vs skill path) |
| Idempotency | Content hash per memory row (not a batch version) |

## 3. System shape

```
                ┌──────────────────────────────────────────────┐
                │                 openshin daemon               │
                │  (one persistent process; the only writer)   │
                │                                              │
                │  pipeline (background, deterministic)         │
                │    raw log → mid-term → long-term   (fact)    │
                │    AI threads          → skill memory (skill) │
                │                                              │
                │  service layer (HTTP ingest + MCP tools)      │
                └───────────────▲───────────────┬──────────────┘
                                │               │ pull (tools)
                         ingest │               │ openshin_get_context
            transcript tail/hook│               │ openshin_search
                                │               │ openshin_read_range
                ┌───────────────┴───────┐       │ openshin_remember
                │  orchestrator agent    │◄──────┘
                │  (Claude Code session) │
                └───────┬───────────────┘
                        │ spawns (Task tool)
              ┌─────────┴─────────┐
              │ subagent A        │  ...  subagent B
              └───────────────────┘
```

- The **daemon** owns the SQLite file and is the only writer of memory. It runs
  the promotion/distillation pipeline in the background and serves the tool
  surface agents call.
- **Ingest** brings raw conversations into the DB: a Claude Code JSONL transcript
  tailer (lossless, captures both sides of every exchange plus tool calls and
  subagent events), hooks for lifecycle signals, and a plain HTTP ingest endpoint
  as the agent-agnostic contract.
- **Agents** pull: `get_context`, `search`, `read_range`, `remember`.

## 4. Memory layers

| Layer | Store | Trigger | Contents | Cleanup |
|---|---|---|---|---|
| **Raw log** | `conversation` | every turn | full text of every exchange, lossless | never |
| **Short-term** | raw `pending` rows | — | the live, un-processed conversation | removed when mid-term processes it |
| **Mid-term** | `mid_term_fact` | focus-change **or** ceiling | atomic facts, `[start_seq, end_seq]` back-refs | TTL expiry |
| **Long-term** | `long_term_memory` | periodic / on-idle | role-tagged facts, decisions, constraints | consolidate/merge |
| **Skill** | `skill_memory` | on subagent/AI-thread completion | procedures ("how the AI solved X") | refresh use-counts |
| **Fallback** | raw log | on-demand tool | `read_range`, `search` | — |

### 4.1 Short-term

Short-term is the raw conversation still waiting to be processed. It is *not*
injected anywhere; it is the source the pipeline reads. It is bounded by a
**token budget**: when the pending content exceeds the budget — or a segment
finishes — the relevant prefix is processed into mid-term. A "rolling summary"
is an optional retrieval-time compaction (`get_context` may return a compressed
recent summary rather than raw text when the window is long); the raw rows
beneath are never dropped, so nothing is lost.

### 4.2 Mid-term — completeness, not importance

Mid-term is a **decaying** memory for recent conversation. It filters *noise*,
not importance:

- A **segment** is a contiguous, settled stretch of the short-term window. The
  daemon decides where it ends (the "cut point") on two triggers, whichever
  fires first:
  1. **focus change** — the conversation's subject moved on, so the previous
     subject's segment is likely complete;
  2. **ceiling** — pending content exceeded the token budget, forcing a process.
- The segment is rendered and passed to `ExtractFacts`, which returns
  `{ facts: []string }` — **an empty list is a valid answer** and means the
  segment was pure noise (greetings, acknowledgments, small talk).
- Each fact is stored as its **own row** with the segment's `[start_seq,
  end_seq]` back-ref, so a later fetch can reach the raw conversation that
  produced it.
- **"Promote the segment" and "store facts" are independent.** A trivial segment
  still leaves short-term (it is settled) even when it writes zero facts.
- Idempotency is a **content hash** (`UNIQUE(user_id, fact_hash)`); a replayed
  extraction upserts, and the same fact restated in two segments dedupes.
- Facts expire after a **TTL**. Anything durable is long-term's job.

### 4.3 Long-term — worthiness and role

Long-term distils what matters and what kind of thing it is. It does **not**
read mid-term's facts; it reads the **full log, framed by the segment
boundaries mid-term produced** (the `segment` table persists those ranges
independently of the decaying facts). It role-tags its output:

- `fact` — a durable truth about the user's world;
- `decision` — a choice made and the reasoning;
- `constraint` — a boundary or rule not to violate;
- `procedure` — reusable know-how (but see skill memory, §4.4).

Retrieval includes a **conflict/constraint mode**: on a new task, the agent can
ask "what has been decided / what might conflict with this?" rather than only
"what is semantically similar?" Long-term merges near-duplicates and resolves
contradictions over time.

### 4.4 Skill memory — procedures from AI activity

Skill memory holds *how the AI solved problems*, not facts about the user. It is
fed by **AI activity** — subagent threads and the orchestrator's own
problem-solving — and is distilled by a *separate* extractor (`{ name, trigger,
steps, context, pitfalls }`) into procedures like "how to add a migration in this
codebase" or "how to work around this library's shortcoming". Skills carry
provenance and `use_count`/`last_used` so frequently-relied-upon patterns float
up.

## 5. Routing — provenance decides the destination

Conversations form a **tree**, not one timeline:

```
user ⇄ orchestrator (root thread, kind='user')
         ├─ orchestrator ⇄ subagent A (kind='subagent', task 1)
         └─ orchestrator ⇄ subagent B (kind='subagent', task 2)
```

`thread.kind` is the routing switch:

| Source | raw log | short-term | mid-term | long-term | skill |
|---|---|---|---|---|---|
| root thread (`user`) | ✓ | ✓ | ✓ | ✓ | ✓ |
| subagent threads | ✓ | ✓ (execution only) | ✗ | ✗ | ✓ |
| orchestrator AI activity | ✓ | — | ✗ | ✗ | ✓ |

The **fact path** (mid → long) is fed only by the root user thread. The **skill
path** is fed by all AI activity. Subagent transcripts therefore can never
pollute user memory; they only ever distill into procedural knowledge.

### 5.1 Fanout isolation

- **Physical separation**: raw rows carry `thread_id` + a per-thread `thread_seq`;
  a subagent's turns never interleave into the user thread's window.
- **Scoped injection → none**: short-term windows are computed per-thread (and, in
  practice, are only ever *read*, never injected — §10).
- **Shared *distilled* knowledge**: mid/long/skill facts are `session_id`-scoped
  and carry provenance (`actor`, `task_id`, `source_thread`, `start_seq`/`end_seq`),
  so the orchestrator can learn "what did subagent A conclude?" as facts — the
  raw transcript is reachable only by an explicit `read_range`.
- **Write asymmetry**: subagents are read-only for facts; findings flow to the
  orchestrator, which is the single owner of the fact path. Subagent output
  auto-distills to skill only.

### 5.2 Subagent thread lifecycle

Subagents do not receive a daemon-managed short-term window. Their working memory
is the brief their parent gave them (Claude Code's `Task` prompt) plus their own
turns. The daemon records, bounds, and routes them:

```
(no thread)
   │  Task tool call (brief captured)
   ▼
spawning ──thread+brief created──▶ active
active ──task result / SubagentStop──▶ completed
completed ──skill extraction──▶ skill_extracted   (0..N procedures, or dropped)
completed ──sweep──▶ archived                      (raw rows remain forever)
active ──session end / crash──▶ orphaned ──reconcile──▶ completed
```

Nested fanout is free: a subagent spawning its own sub-subagent creates another
`kind='subagent'` thread with `parent_thread_id` pointing at its parent.

## 6. Data model (SQLite)

```sql
session        (session_id PK, created_at)                 -- root unit (a Claude Code session)
thread         (thread_id PK, session_id, parent_thread_id NULL,
                task_id NULL, kind TEXT, brief TEXT, completed_at NULL)
conversation   (seq INTEGER, thread_id, thread_seq, actor TEXT, role TEXT,
                content TEXT, created_at)
segment        (segment_id PK, session_id, thread_id, start_seq, end_seq, completed_at)
mid_term_fact  (fact_id PK, user_id/session_id, fact TEXT, embedding BLOB,
                start_seq, end_seq, expires_at, fact_hash TEXT UNIQUE)
long_term_memory(user_id/session_id, content TEXT, role_tag TEXT, embedding BLOB,
                source_segments TEXT, ...)
skill_memory   (skill_id PK, session_id, name TEXT, trigger TEXT, steps TEXT,
                embedding BLOB, use_count INT, last_used_at, source_thread)
```

- `thread_seq` is the per-thread ordering key used by all window logic; `seq` is
  a global monotonic value for audit/ordering only.
- `segment` persists the cut points long-term reuses (§4.3), independent of the
  decaying mid-term facts.
- Embeddings are `BLOB` float32; cosine is computed in-process (exact, ample for
  local scale — no vector extension, no ANN index).

## 7. Providers

A single abstraction with several backends:

```
Chat      Generate(system, prompt) → text
Embed     Embed(text, taskType) → []float32
Extract   ExtractFacts(text) → []string      (mid-term)
          ExtractSubject(text) → keep, subject  (focus/segment signal)
          Summarize(text) → string            (rolling summary)
```

- **Gemini** (default), **Anthropic** (Claude API), **OpenAI**, **Ollama**
  (local, OpenAI-compatible).
- **AgentHost** — *use the model the user is already running*. Instead of making
  its own API call, the daemon enqueues an extraction that the surrounding agent
  fulfills (an `openshin_extract` tool, or a hook producing the JSON inline).
  Zero extra keys, rides the user's existing spend. It is a *mode* (inverts
  control and is only available while the agent is active), not the default.

## 8. Agent alignment

Three surfaces, all over the same `service` layer:

1. **MCP server** (`openshin mcp`, stdio) — tools:
   - `openshin_get_context(session)` — focus subject + top-K mid-term facts +
     relevant long-term constraints + recent short-term summary, in one call;
   - `openshin_search(query)` — semantic facts with `[start_seq, end_seq]` refs;
   - `openshin_read_range(start, end, thread)` — raw log slice;
   - `openshin_remember(content)` — explicit targeted retention.
2. **Hooks** — lifecycle: `SessionStart`/`Stop` for session bookkeeping and
   ingest nudges (not context injection).
3. **Transcript tailer** — the lossless raw-log source: tail the session JSONL
   (`~/.claude/projects/<project>/<session>.jsonl`) into `conversation`.

**"Always check memory"** is a behavioral contract, enforced softly: a generated
CLAUDE.md snippet + MCP tool descriptions instruct the agent to call
`get_context` at task start and before touching anything. No forced hook.

**Fanout scoping**: the orchestrator exports `OPENSHIN_SESSION`; subagents
inherit it with a `OPENSHIN_TASK` id so all their turns land under the same
session, scoped per task, retrievable later.

## 9. Retrieval model

Pull-only, in two tiers:

1. **One-shot context** — `get_context` returns the assembled, bounded memory an
   agent needs to start: focus + relevant mid-term + relevant long-term
   constraints + a recent short-term summary. This is the "remember what's been
   done and what might conflict" moment.
2. **On-demand deep dive** — `search` (semantic, across layers) and `read_range`
   (exact raw slice) for when a specific thread, decision, or conversation needs
   to be reopened.

The lossless guarantee is structural: because every conversation lives in the raw
log and every fact carries a `[start_seq, end_seq]` back-ref, nothing is ever
truly lost — memory is an *index over an immutable source of truth*, not the
source itself.

## 10. Pipeline state machine

Mid-term processing is the heart of the write side. The heavy claim/lease/
reclaim machinery from the earlier Postgres design is dropped: a single local
writer promotes in a serial loop under one transaction, keeping a light
`processing` marker and a reconcile-on-start for crash recovery. The fencing
returns only at fleet scale.

- **Segment boundary**: focus change (primary) or ceiling (safety net).
- **Extraction**: `ExtractFacts` per segment; empty list allowed.
- **Promotion**: forward-only along `thread_seq`; a settled segment never
  re-opens — revisiting an old topic is a *new* segment with new refs.
- **Idempotency**: content hash; **decay**: TTL on mid-term facts.

Long-term and skill distillation run periodically / on-idle; with a local
session-scoped presence this is **catch-up-on-start** (flush pending + run due
distillation before the next session begins) plus an optional short linger.

## 11. Roadmap

| Phase | Deliverable |
|---|---|
| **0 (here)** | Repo scaffold: `go.mod`, goose + sqlc config, SQLite connect, CI, this doc |
| **1** | Raw log + ingest (transcript tailer, HTTP ingest, `read_range`) |
| **2** | Retrieval tools (`get_context`, `search`, `remember`) |
| **3** | Mid-term: segments (focus-change/ceiling), atomic-fact extraction, TTL |
| **4** | Long-term: role tags, conflict/constraint retrieval, consolidation |
| **5** | Skill memory: procedural extraction from AI threads |
| **6** | Agent alignment: MCP server, hooks, CLAUDE.md generation, Claude Code wiring |
| **7** | Fleet (later): Postgres/pgvector backend, shared bus, multi-worker claims |

Each phase ships a spec doc (this style) before its implementation, with an
acceptance test that gates approval.

## 12. Out of scope / open

- **Fleet** (phase 7) — separate backend + transport, same interfaces.
- **The exact provider surface** — which backends ship in v1 (Gemini + AgentHost
  are the candidates) is decided at implementation time.
- **Injection** — deliberately ruled out by §1; if a future need for
  session-start context emerges, it is opt-in via a tool, not automatic.
