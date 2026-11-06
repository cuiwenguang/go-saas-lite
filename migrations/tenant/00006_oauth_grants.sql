-- +goose Up
CREATE TABLE IF NOT EXISTS oauth_grants (
    tenant_id  uuid        NOT NULL,
    user_id    uuid        NOT NULL,
    client_id  varchar(64) NOT NULL,
    scope      varchar(64) NOT NULL DEFAULT 'mcp',
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS oauth_grants_uniq ON oauth_grants (tenant_id, user_id, client_id);

-- +goose Down
DROP TABLE IF EXISTS oauth_grants;
