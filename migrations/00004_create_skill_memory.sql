-- +goose Up
-- Skill memory: reusable procedures distilled from AI activity (subagent
-- threads and the orchestrator's own problem-solving). Repeated patterns merge
-- into one skill with a use count.

CREATE TABLE skill_memory (
    id            INTEGER PRIMARY KEY,
    session_id    TEXT NOT NULL,
    name          TEXT NOT NULL,
    trigger       TEXT NOT NULL,
    steps         TEXT NOT NULL,
    embedding     BLOB,
    source_thread TEXT,
    use_count     INTEGER NOT NULL DEFAULT 1,
    last_used_at  TEXT,
    name_hash     TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (session_id, name_hash)
);

-- Which threads the skill layer has already distilled (analogous to segment.distilled).
ALTER TABLE thread ADD COLUMN distilled INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE thread DROP COLUMN distilled;
DROP TABLE skill_memory;
