-- +goose Up
CREATE TABLE IF NOT EXISTS platform_users (
    id            uuid PRIMARY KEY,
    email         varchar(255) NOT NULL,
    password_hash varchar(255) NOT NULL,
    role          varchar(32)  NOT NULL DEFAULT 'owner',
    status        varchar(16)  NOT NULL DEFAULT 'active',
    created_at    timestamptz  NOT NULL DEFAULT now(),
    updated_at    timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS platform_users_email_uniq ON platform_users (email);

-- +goose Down
DROP TABLE IF EXISTS platform_users;
