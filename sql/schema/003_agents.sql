CREATE TABLE IF NOT EXISTS agents (
    id         uuid         NOT NULL DEFAULT uuidv7(),
    name       varchar(255) NOT NULL,
    soul       text         NOT NULL,
    is_default boolean      NOT NULL DEFAULT false,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    PRIMARY KEY (id)
);
