CREATE TABLE IF NOT EXISTS models (
    id                          uuid         NOT NULL DEFAULT uuidv7(),
    provider_id                 uuid         NOT NULL,
    name                        varchar(255) NOT NULL,
    code                        varchar(255) NOT NULL,
    default_model               boolean      NOT NULL DEFAULT false,
    thinking                    boolean      NOT NULL DEFAULT false,
    thinking_levels             jsonb        NOT NULL DEFAULT '[]'::jsonb,
    anthropic_adaptive_thinking boolean      NOT NULL DEFAULT false,
    created_at                  timestamptz  NOT NULL DEFAULT now(),
    updated_at                  timestamptz  NOT NULL DEFAULT now(),
    deleted_at                  timestamptz,
    PRIMARY KEY (id)
);
