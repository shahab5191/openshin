-- name: ListUndistilledSubagentThreads :many
SELECT id, session_id
FROM thread
WHERE kind = 'subagent' AND completed_at IS NOT NULL AND distilled = 0
ORDER BY created_at;

-- name: MarkThreadDistilled :exec
UPDATE thread SET distilled = 1 WHERE id = ?;

-- name: CompleteThread :exec
UPDATE thread SET completed_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?;

-- name: UpsertSkill :exec
INSERT INTO skill_memory (session_id, name, trigger, steps, embedding, source_thread, name_hash)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (session_id, name_hash) DO UPDATE SET
    trigger = excluded.trigger,
    steps = excluded.steps,
    use_count = skill_memory.use_count + 1,
    last_used_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now');

-- name: ListSkills :many
SELECT id, session_id, name, trigger, steps, embedding, source_thread, use_count, last_used_at, name_hash, created_at
FROM skill_memory
WHERE session_id = ?
ORDER BY use_count DESC, id;
