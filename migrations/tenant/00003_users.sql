-- +goose Up
CREATE TABLE IF NOT EXISTS users (
    id                   uuid PRIMARY KEY,
    tenant_id            uuid         NOT NULL,
    email                varchar(255) NOT NULL,
    phone                varchar(32),
    display_name         varchar(64)  NOT NULL DEFAULT '',
    password_hash        varchar(255) NOT NULL,
    status               varchar(16)  NOT NULL DEFAULT 'active',
    must_change_password boolean      NOT NULL DEFAULT false,
    token_version        integer      NOT NULL DEFAULT 0,
    created_at           timestamptz  NOT NULL DEFAULT now(),
    updated_at           timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS users_tenant_email_uniq ON users (tenant_id, email);
CREATE UNIQUE INDEX IF NOT EXISTS users_tenant_phone_uniq ON users (tenant_id, phone) WHERE phone IS NOT NULL;
CREATE INDEX IF NOT EXISTS users_tenant_id_idx ON users (tenant_id);

-- +goose Down
DROP TABLE IF EXISTS users;
