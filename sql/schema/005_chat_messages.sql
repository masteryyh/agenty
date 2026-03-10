CREATE TABLE IF NOT EXISTS chat_messages (
    id                 uuid        NOT NULL DEFAULT uuidv7(),
    session_id         uuid        NOT NULL,
    agent_id           uuid        NOT NULL,
    role               varchar(50) NOT NULL,
    content            text        NOT NULL DEFAULT '',
    tool_calls         jsonb,
    tool_results       jsonb,
    model_id           uuid        NOT NULL,
    reasoning_content  text        NOT NULL DEFAULT '',
    provider_specifics jsonb,
    created_at         timestamptz NOT NULL DEFAULT now(),
    deleted_at         timestamptz,
    PRIMARY KEY (id)
);
