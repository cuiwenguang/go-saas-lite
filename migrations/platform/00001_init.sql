-- +goose Up
CREATE TABLE IF NOT EXISTS tenants (
    id         uuid PRIMARY KEY,
    code       varchar(64)  NOT NULL,
    name       varchar(128) NOT NULL,
    status     varchar(16)  NOT NULL DEFAULT 'pending',
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS tenants_code_uniq ON tenants (code);

-- +goose Down
DROP TABLE IF EXISTS tenants;
