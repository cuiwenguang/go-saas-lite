-- +goose Up
CREATE TABLE IF NOT EXISTS tenant_configs (
    tenant_id  uuid         NOT NULL,
    namespace  varchar(64)  NOT NULL,
    key        varchar(128) NOT NULL,
    value      text         NOT NULL,
    updated_at timestamptz  NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, namespace, key)
);

CREATE INDEX IF NOT EXISTS tenant_configs_tenant_idx ON tenant_configs (tenant_id);

-- +goose Down
DROP TABLE IF EXISTS tenant_configs;
