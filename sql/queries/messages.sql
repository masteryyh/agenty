-- name: CreateMessage :one
INSERT INTO chat_messages (session_id, agent_id, role, content, tool_calls, tool_results,
                           model_id, reasoning_content, provider_specifics, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
RETURNING id, session_id, agent_id, role, content, tool_calls, tool_results,
          model_id, reasoning_content, provider_specifics, created_at, deleted_at;

-- name: BatchCreateMessages :many
INSERT INTO chat_messages (session_id, agent_id, role, content, tool_calls, tool_results,
                           model_id, reasoning_content, provider_specifics, created_at)
SELECT unnest(@session_ids::uuid[]),
       unnest(@agent_ids::uuid[]),
       unnest(@roles::varchar[]),
       unnest(@contents::text[]),
       unnest(@tool_calls_arr::jsonb[]),
       unnest(@tool_results_arr::jsonb[]),
       unnest(@model_ids::uuid[]),
       unnest(@reasoning_contents::text[]),
       unnest(@provider_specifics_arr::jsonb[]),
       unnest(@created_ats::timestamptz[])
RETURNING id, session_id, agent_id, role, content, tool_calls, tool_results,
          model_id, reasoning_content, provider_specifics, created_at, deleted_at;

-- name: GetMessagesBySession :many
SELECT id, session_id, agent_id, role, content, tool_calls, tool_results,
       model_id, reasoning_content, provider_specifics, created_at, deleted_at
FROM chat_messages
WHERE session_id = $1 AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: GetDistinctModelIdsBySession :many
SELECT DISTINCT model_id
FROM chat_messages
WHERE session_id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteMessagesByAgent :exec
UPDATE chat_messages SET deleted_at = NOW()
WHERE agent_id = $1 AND deleted_at IS NULL;
