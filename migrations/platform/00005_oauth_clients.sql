-- +goose Up
CREATE TABLE IF NOT EXISTS oauth_clients (
    client_id                  varchar(64) PRIMARY KEY,
    tenant_id                  uuid         NOT NULL,
    client_name                varchar(128) NOT NULL,
    redirect_uris              jsonb        NOT NULL,
    grant_types                jsonb        NOT NULL DEFAULT '["authorization_code","refresh_token"]'::jsonb,
    token_endpoint_auth_method varchar(32)  NOT NULL DEFAULT 'none',
    revoked_at                 timestamptz,
    created_at                 timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS oauth_clients_tenant_idx ON oauth_clients (tenant_id);

-- +goose Down
DROP TABLE IF EXISTS oauth_clients;
