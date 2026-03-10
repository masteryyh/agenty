CREATE TABLE IF NOT EXISTS model_providers (
    id         uuid         NOT NULL DEFAULT uuidv7(),
    name       varchar(255) NOT NULL,
    type       varchar(50)  NOT NULL,
    base_url   varchar(255) NOT NULL,
    api_key    varchar(255) NOT NULL,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    PRIMARY KEY (id)
);
