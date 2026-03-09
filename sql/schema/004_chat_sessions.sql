CREATE TABLE IF NOT EXISTS chat_sessions (
    id              uuid        NOT NULL DEFAULT uuidv7(),
    agent_id        uuid        NOT NULL,
    token_consumed  bigint      NOT NULL DEFAULT 0,
    last_used_model uuid        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    PRIMARY KEY (id)
);
