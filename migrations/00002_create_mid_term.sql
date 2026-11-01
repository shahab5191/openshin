-- +goose Up
-- Mid-term: a decaying memory of recent conversation. Finished segments of the
-- short-term window are distilled into atomic facts, each with a back-reference
-- to the messages that produced it, and each expiring after a TTL.

-- A processed segment: a contiguous, settled run of messages of one thread.
-- Long-term (a later phase) reads the full log framed by these boundaries.
CREATE TABLE segment (
    id         INTEGER PRIMARY KEY,
    session_id TEXT NOT NULL,
    thread_id  TEXT NOT NULL,
    start_seq  INTEGER NOT NULL,
    end_seq    INTEGER NOT NULL,
    subject    TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX segment_thread_idx ON segment (thread_id, end_seq);

-- One durable fact per row, extracted from a segment. fact_hash dedupes the
-- same fact stated twice; expires_at drives the decay sweep.
CREATE TABLE mid_term_fact (
    id         INTEGER PRIMARY KEY,
    session_id TEXT NOT NULL,
    thread_id  TEXT NOT NULL,
    fact       TEXT NOT NULL,
    embedding  BLOB,
    start_seq  INTEGER NOT NULL,
    end_seq    INTEGER NOT NULL,
    fact_hash  TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (session_id, fact_hash)
);

CREATE INDEX mid_term_fact_expiry_idx ON mid_term_fact (expires_at);

-- The current conversation subject, tracked on the thread so a subject change
-- can be detected as a segment boundary.
ALTER TABLE thread ADD COLUMN subject TEXT;

-- +goose Down
ALTER TABLE thread DROP COLUMN subject;
DROP TABLE mid_term_fact;
DROP TABLE segment;
