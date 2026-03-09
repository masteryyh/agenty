-- name: GetModelById :one
SELECT id, provider_id, name, code, default_model, thinking, thinking_levels,
       anthropic_adaptive_thinking, created_at, updated_at, deleted_at
FROM models
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetModelByNameAndProvider :one
SELECT id, provider_id, name, code, default_model, thinking, thinking_levels,
       anthropic_adaptive_thinking, created_at, updated_at, deleted_at
FROM models
WHERE name = $1 AND provider_id = $2 AND deleted_at IS NULL;

-- name: GetDefaultModel :one
SELECT id, provider_id, name, code, default_model, thinking, thinking_levels,
       anthropic_adaptive_thinking, created_at, updated_at, deleted_at
FROM models
WHERE default_model IS TRUE AND deleted_at IS NULL
LIMIT 1;

-- name: GetFirstModel :one
SELECT id, provider_id, name, code, default_model, thinking, thinking_levels,
       anthropic_adaptive_thinking, created_at, updated_at, deleted_at
FROM models
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: GetCurrentDefaultExcluding :one
SELECT id, provider_id, name, code, default_model, thinking, thinking_levels,
       anthropic_adaptive_thinking, created_at, updated_at, deleted_at
FROM models
WHERE default_model IS TRUE AND id != $1 AND deleted_at IS NULL
LIMIT 1;

-- name: ListModels :many
SELECT id, provider_id, name, code, default_model, thinking, thinking_levels,
       anthropic_adaptive_thinking, created_at, updated_at, deleted_at
FROM models
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountModels :one
SELECT COUNT(*) FROM models WHERE deleted_at IS NULL;

-- name: ListModelsByProvider :many
SELECT id, provider_id, name, code, default_model, thinking, thinking_levels,
       anthropic_adaptive_thinking, created_at, updated_at, deleted_at
FROM models
WHERE provider_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountModelsByProvider :one
SELECT COUNT(*) FROM models WHERE provider_id = $1 AND deleted_at IS NULL;

-- name: ListModelsByIds :many
SELECT id, provider_id, name, code, default_model, thinking, thinking_levels,
       anthropic_adaptive_thinking, created_at, updated_at, deleted_at
FROM models
WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL;

-- name: GetModelsByProvider :many
SELECT id, provider_id, name, code, default_model, thinking, thinking_levels,
       anthropic_adaptive_thinking, created_at, updated_at, deleted_at
FROM models
WHERE provider_id = $1 AND deleted_at IS NULL;

-- name: CountModelsByName :one
SELECT COUNT(*) FROM models
WHERE name = $1 AND provider_id = $2 AND deleted_at IS NULL;

-- name: CountModelsByCode :one
SELECT COUNT(*) FROM models
WHERE code = $1 AND provider_id = $2 AND deleted_at IS NULL;

-- name: CountModelsByNameExcluding :one
SELECT COUNT(*) FROM models
WHERE name = $1 AND provider_id = $2 AND id != $3 AND deleted_at IS NULL;

-- name: CreateModel :one
INSERT INTO models (provider_id, name, code, default_model, thinking, thinking_levels,
                    anthropic_adaptive_thinking, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
RETURNING id, provider_id, name, code, default_model, thinking, thinking_levels,
          anthropic_adaptive_thinking, created_at, updated_at, deleted_at;

-- name: UpdateModelFields :exec
UPDATE models
SET name = $2, thinking = $3, thinking_levels = $4,
    anthropic_adaptive_thinking = $5, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SetModelAsDefault :exec
UPDATE models SET default_model = true, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SetModelDefaultFalse :exec
UPDATE models SET default_model = false, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: ClearDefaultModelExcluding :exec
UPDATE models SET default_model = false, updated_at = NOW()
WHERE id != $1 AND default_model IS TRUE AND deleted_at IS NULL;

-- name: SoftDeleteModel :exec
UPDATE models SET deleted_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteModelsByProvider :exec
UPDATE models SET deleted_at = NOW()
WHERE provider_id = $1 AND deleted_at IS NULL;
