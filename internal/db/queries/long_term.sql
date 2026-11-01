-- name: ListUndistilledSegments :many
SELECT id, session_id, thread_id, start_seq, end_seq
FROM segment
WHERE distilled = 0
ORDER BY end_seq;

-- name: MarkSegmentDistilled :exec
UPDATE segment SET distilled = 1 WHERE id = ?;

-- name: MessagesBetween :many
SELECT seq, thread_id, thread_seq, actor, role, content, source_ref, created_at
FROM conversation
WHERE thread_id = ? AND seq >= ? AND seq <= ?
ORDER BY seq;

-- name: UpsertLongTerm :exec
INSERT INTO long_term_memory
    (session_id, content, role_tag, embedding, source_start_seq, source_end_seq, content_hash)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (session_id, content_hash) DO NOTHING;

-- name: ListLongTerm :many
SELECT id, session_id, content, role_tag, embedding, source_start_seq, source_end_seq, content_hash, created_at
FROM long_term_memory
WHERE session_id = ?
ORDER BY id;

-- name: ListActiveFacts :many
SELECT id, session_id, thread_id, fact, embedding, start_seq, end_seq, fact_hash, expires_at, created_at
FROM mid_term_fact
WHERE session_id = ? AND expires_at >= ?
ORDER BY id;
