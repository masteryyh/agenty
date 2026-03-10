-- name: GetProviderById :one
SELECT id, name, type, base_url, api_key, created_at, updated_at, deleted_at
FROM model_providers
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetProviderByName :one
SELECT id, name, type, base_url, api_key, created_at, updated_at, deleted_at
FROM model_providers
WHERE name = $1 AND deleted_at IS NULL;

-- name: ListProviders :many
SELECT id, name, type, base_url, api_key, created_at, updated_at, deleted_at
FROM model_providers
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountProviders :one
SELECT COUNT(*) FROM model_providers WHERE deleted_at IS NULL;

-- name: CountProvidersByName :one
SELECT COUNT(*) FROM model_providers WHERE name = $1 AND deleted_at IS NULL;

-- name: CountProvidersByNameExcluding :one
SELECT COUNT(*) FROM model_providers
WHERE name = $1 AND id != $2 AND deleted_at IS NULL;

-- name: CountProvidersByNameAndType :one
SELECT COUNT(*) FROM model_providers WHERE name = $1 AND type = $2 AND deleted_at IS NULL;

-- name: ListProvidersByIds :many
SELECT id, name, type, base_url, api_key, created_at, updated_at, deleted_at
FROM model_providers
WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL;

-- name: CreateProvider :one
INSERT INTO model_providers (name, type, base_url, api_key, created_at, updated_at)
VALUES ($1, $2, $3, $4, NOW(), NOW())
RETURNING id, name, type, base_url, api_key, created_at, updated_at, deleted_at;

-- name: UpdateProvider :one
UPDATE model_providers
SET name = $2, type = $3, base_url = $4, api_key = $5, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, name, type, base_url, api_key, created_at, updated_at, deleted_at;

-- name: SoftDeleteProvider :exec
UPDATE model_providers SET deleted_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;
