-- +goose Up
-- Long-term: durable, role-tagged knowledge (fact / decision / constraint)
-- distilled from the full log framed by mid-term segments. Unlike mid-term, it
-- does not expire — it is consolidated, not decayed.

CREATE TABLE long_term_memory (
    id               INTEGER PRIMARY KEY,
    session_id       TEXT NOT NULL,
    content          TEXT NOT NULL,
    role_tag         TEXT NOT NULL CHECK (role_tag IN ('fact', 'decision', 'constraint')),
    embedding        BLOB,
    source_start_seq INTEGER NOT NULL,
    source_end_seq   INTEGER NOT NULL,
    content_hash     TEXT NOT NULL,
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (session_id, content_hash)
);

-- Mark which segments long-term has already distilled, so a pass only reads
-- new segments.
ALTER TABLE segment ADD COLUMN distilled INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE segment DROP COLUMN distilled;
DROP TABLE long_term_memory;
