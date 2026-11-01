-- name: UpsertSession :exec
INSERT INTO session (id) VALUES (?) ON CONFLICT (id) DO NOTHING;

-- name: UpsertThread :exec
INSERT INTO thread (id, session_id, parent_thread_id, task_id, kind, brief)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    session_id = excluded.session_id,
    parent_thread_id = COALESCE(excluded.parent_thread_id, thread.parent_thread_id),
    task_id = COALESCE(excluded.task_id, thread.task_id),
    kind = excluded.kind,
    brief = COALESCE(excluded.brief, thread.brief);

-- name: NextThreadSeq :one
SELECT COALESCE(MAX(thread_seq), 0) + 1 FROM conversation WHERE thread_id = ?;

-- name: AppendMessage :one
INSERT INTO conversation (thread_id, thread_seq, actor, role, content, source_ref, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING seq, thread_id, thread_seq, actor, role, content, source_ref, created_at;

-- name: GetMessageBySourceRef :one
SELECT seq, thread_id, thread_seq, actor, role, content, source_ref, created_at
FROM conversation
WHERE source_ref = ?;

-- name: LatestMessages :many
SELECT seq, thread_id, thread_seq, actor, role, content, source_ref, created_at
FROM conversation
WHERE thread_id = ?
ORDER BY seq DESC
LIMIT ?;

-- name: ReadRange :many
SELECT seq, thread_id, thread_seq, actor, role, content, source_ref, created_at
FROM conversation
WHERE thread_id = ? AND seq >= ? AND seq <= ?
ORDER BY seq ASC;

-- name: GetThread :one
SELECT id, session_id, parent_thread_id, task_id, kind, brief, created_at, completed_at, subject
FROM thread
WHERE id = ?;

-- name: ListThreads :many
SELECT id, session_id, parent_thread_id, task_id, kind, brief, created_at, completed_at, subject
FROM thread
ORDER BY created_at ASC;
