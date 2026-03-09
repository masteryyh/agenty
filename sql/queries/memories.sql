-- name: CreateMemory :one
INSERT INTO memories (agent_id, content, embedding, created_at, updated_at)
VALUES ($1, $2, $3, NOW(), NOW())
RETURNING id, agent_id, content, created_at, updated_at, deleted_at;

-- name: VectorSearchMemories :many
SELECT id, agent_id, content, created_at, updated_at, deleted_at
FROM memories
WHERE deleted_at IS NULL AND agent_id = $1
ORDER BY embedding <=> $2
LIMIT $3;

-- name: FullTextSearchMemories :many
SELECT id, agent_id, content, created_at, updated_at, deleted_at
FROM memories
WHERE deleted_at IS NULL AND agent_id = $1
  AND to_tsvector('simple', content) @@ plainto_tsquery('simple', $2)
ORDER BY ts_rank(to_tsvector('simple', content), plainto_tsquery('simple', $2)) DESC
LIMIT $3;

-- name: SoftDeleteMemoriesByAgent :exec
UPDATE memories SET deleted_at = NOW()
WHERE agent_id = $1 AND deleted_at IS NULL;
