-- +goose Up
CREATE TABLE IF NOT EXISTS api_keys (
    id           uuid PRIMARY KEY,
    tenant_id    uuid        NOT NULL,
    user_id      uuid        NOT NULL,
    name         varchar(64) NOT NULL,
    key_prefix   varchar(16) NOT NULL,
    key_hash     varchar(64) NOT NULL,
    revoked_at   timestamptz,
    last_used_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS api_keys_hash_uniq ON api_keys (key_hash);
CREATE INDEX IF NOT EXISTS api_keys_tenant_user_idx ON api_keys (tenant_id, user_id);

-- +goose Down
DROP TABLE IF EXISTS api_keys;
