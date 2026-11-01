# Skill Memory — Spec

Status: **implemented** (phase 5).

This document describes skill memory: reusable procedures distilled from AI
activity, so patterns the agents discover are not rediscovered next time.

## What this phase solves

Facts and decisions are about *what is true*. But much of what an agent learns
is *how to do things* — how to navigate a codebase, work around a library's
shortcoming, or reproduce a fix. That procedural knowledge lives in subagent
conversations, and it is exactly what should be generalised and reused. This
phase distils completed subagent threads into skills.

## Routing reminder

This is the second half of the routing rule from the architecture:

- the **root** user thread feeds the fact path (mid-term → long-term);
- **subagent** threads feed **only** skill memory.

So a subagent's transcript can never pollute user facts — it only ever becomes
procedural know-how.

## How it works

1. A completed subagent thread (`kind='subagent'`, `completed_at` set, not yet
   distilled) is rendered to text.
2. The model generalises it into a procedure: `{ name, trigger, steps }` — or
   empty if there is nothing reusable.
3. The skill is stored (with an embedding for later retrieval). If a skill with
   the same name already exists, the pattern is **merged**: `use_count` is
   incremented and the steps are refreshed. Repeated patterns therefore
   generalise into one skill that floats up in importance.
4. The thread is marked distilled.

Skills are searchable like facts: retrieval returns them tagged `skill`, ranked
by similarity, most-used first within their own listing.

## What is built

- `migrations/00004_create_skill_memory.sql` — `skill_memory` table and a
  `distilled` flag on `thread`.
- `internal/store/skill.go` — `Skill` type, list-undistilled-threads, upsert
  (with `use_count` merge), list-skills, complete-thread.
- `internal/memory/skill.go` — the `SkillMemory` layer.
- `internal/llm/gemini.go` — `ExtractSkill`.
- Retrieval now includes skills (role tag `skill`).

## How it is tested

- A completed subagent thread distils into one skill.
- Two threads with the same skill name merge into one skill with `use_count` 2.
- A thread with nothing reusable is still marked distilled, storing no skill.
- Search surfaces a skill with the `skill` role tag.

## Out of scope

- Detecting subagent spawn/completion from the transcript (agent alignment, the
  next phase). Here threads are completed via the store's `CompleteThread`,
  which the tailer will call once it understands fanout.
- Feeding the orchestrator's *own* assistant reasoning into skill extraction —
  currently only subagent threads are distilled.
