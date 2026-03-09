-- name: CreateAgent :one
INSERT INTO agents (name, soul, is_default, created_at, updated_at)
VALUES ($1, $2, $3, NOW(), NOW())
RETURNING id, name, soul, is_default, created_at, updated_at, deleted_at;

-- name: GetAgentById :one
SELECT id, name, soul, is_default, created_at, updated_at, deleted_at
FROM agents
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListAgents :many
SELECT id, name, soul, is_default, created_at, updated_at, deleted_at
FROM agents
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountAgents :one
SELECT COUNT(*) FROM agents WHERE deleted_at IS NULL;

-- name: CountAgentsByName :one
SELECT COUNT(*) FROM agents WHERE name = $1 AND deleted_at IS NULL;

-- name: CountAgentsByNameExcluding :one
SELECT COUNT(*) FROM agents
WHERE name = $1 AND id != $2 AND deleted_at IS NULL;

-- name: ClearAllDefaultAgents :exec
UPDATE agents SET is_default = false, updated_at = NOW()
WHERE deleted_at IS NULL AND is_default IS TRUE;

-- name: SetAgentDefault :exec
UPDATE agents SET is_default = true, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SetAgentNotDefault :exec
UPDATE agents SET is_default = false, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateAgentFields :exec
UPDATE agents
SET name = $2, soul = $3, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteAgent :exec
UPDATE agents SET deleted_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;
