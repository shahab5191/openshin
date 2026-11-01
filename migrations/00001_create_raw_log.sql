-- +goose Up
-- Raw conversation log: the immutable source of truth every layer indexes over.
-- Nothing is ever deleted here, so nothing is ever truly lost.

-- A root unit (one Claude Code session, or one user session).
CREATE TABLE session (
    id         TEXT PRIMARY KEY,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- A conversation thread. The root thread (kind = 'user') is the user <-> orchestrator
-- conversation; subagent threads (kind = 'subagent') hang off it via parent_thread_id,
-- forming a tree for fanout. kind drives routing (fact path vs skill path).
CREATE TABLE thread (
    id               TEXT PRIMARY KEY,
    session_id       TEXT NOT NULL REFERENCES session (id),
    parent_thread_id TEXT REFERENCES thread (id),
    task_id          TEXT,
    kind             TEXT NOT NULL DEFAULT 'user' CHECK (kind IN ('user', 'subagent')),
    brief            TEXT,
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    completed_at     TEXT
);

CREATE INDEX thread_session_idx ON thread (session_id);

-- One message. seq is the global monotonic order; thread_seq is the per-thread
-- order all window logic sorts by. source_ref is an idempotency key from the
-- ingest source (e.g. a Claude Code transcript uuid), so a replayed tail does
-- not duplicate.
CREATE TABLE conversation (
    seq        INTEGER PRIMARY KEY,
    thread_id  TEXT NOT NULL REFERENCES thread (id),
    thread_seq INTEGER NOT NULL,
    actor      TEXT NOT NULL,
    role       TEXT NOT NULL,
    content    TEXT NOT NULL,
    source_ref TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (thread_id, thread_seq),
    UNIQUE (source_ref)
);

CREATE INDEX conversation_thread_idx ON conversation (thread_id, thread_seq);

-- +goose Down
DROP TABLE conversation;
DROP TABLE thread;
DROP TABLE session;
