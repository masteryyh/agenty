-- name: CreateSession :one
INSERT INTO chat_sessions (agent_id, last_used_model, created_at, updated_at)
VALUES ($1, $2, NOW(), NOW())
RETURNING id, agent_id, token_consumed, last_used_model, created_at, updated_at, deleted_at;

-- name: GetSessionById :one
SELECT id, agent_id, token_consumed, last_used_model, created_at, updated_at, deleted_at
FROM chat_sessions
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetLastSession :one
SELECT id, agent_id, token_consumed, last_used_model, created_at, updated_at, deleted_at
FROM chat_sessions
WHERE deleted_at IS NULL
ORDER BY updated_at DESC
LIMIT 1;

-- name: GetLastSessionByAgent :one
SELECT id, agent_id, token_consumed, last_used_model, created_at, updated_at, deleted_at
FROM chat_sessions
WHERE agent_id = $1 AND deleted_at IS NULL
ORDER BY updated_at DESC
LIMIT 1;

-- name: ListSessions :many
SELECT id, agent_id, token_consumed, last_used_model, created_at, updated_at, deleted_at
FROM chat_sessions
WHERE deleted_at IS NULL
ORDER BY updated_at DESC
LIMIT $1 OFFSET $2;

-- name: CountSessions :one
SELECT COUNT(*) FROM chat_sessions WHERE deleted_at IS NULL;

-- name: UpdateSessionTokenAndModel :exec
UPDATE chat_sessions
SET token_consumed = $2, last_used_model = $3, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteSessionsByAgent :exec
UPDATE chat_sessions SET deleted_at = NOW()
WHERE agent_id = $1 AND deleted_at IS NULL;
