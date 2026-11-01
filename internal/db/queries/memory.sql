-- name: MessagesAfter :many
SELECT seq, thread_id, thread_seq, actor, role, content, source_ref, created_at
FROM conversation
WHERE thread_id = ? AND seq > ?
ORDER BY seq ASC
LIMIT ?;

-- name: LastSegmentEnd :one
SELECT CAST(COALESCE(MAX(end_seq), 0) AS INTEGER) FROM segment WHERE thread_id = ?;

-- name: CurrentSubject :one
SELECT subject FROM thread WHERE id = ?;

-- name: SetSubject :exec
UPDATE thread SET subject = ? WHERE id = ?;

-- name: CreateSegment :one
INSERT INTO segment (session_id, thread_id, start_seq, end_seq, subject)
VALUES (?, ?, ?, ?, ?)
RETURNING id, session_id, thread_id, start_seq, end_seq, subject, created_at;

-- name: UpsertFact :exec
INSERT INTO mid_term_fact (session_id, thread_id, fact, embedding, start_seq, end_seq, fact_hash, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (session_id, fact_hash) DO NOTHING;

-- name: DeleteExpiredFacts :execrows
DELETE FROM mid_term_fact WHERE expires_at < ?;

-- name: ListSegments :many
SELECT id, session_id, thread_id, start_seq, end_seq, subject, created_at
FROM segment
WHERE thread_id = ?
ORDER BY end_seq;

-- name: ListFacts :many
SELECT id, session_id, thread_id, fact, embedding, start_seq, end_seq, fact_hash, expires_at, created_at
FROM mid_term_fact
WHERE session_id = ?
ORDER BY id;
