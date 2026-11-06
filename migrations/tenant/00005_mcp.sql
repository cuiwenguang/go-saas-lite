-- +goose Up
CREATE TABLE IF NOT EXISTS mcp_servers (
    id         uuid PRIMARY KEY,
    tenant_id  uuid         NOT NULL,
    name       varchar(64)  NOT NULL,
    base_url   varchar(512) NOT NULL,
    credential text,
    enabled    boolean      NOT NULL DEFAULT true,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS mcp_servers_tenant_name_uniq ON mcp_servers (tenant_id, name);

CREATE TABLE IF NOT EXISTS mcp_tools (
    id             uuid PRIMARY KEY,
    tenant_id      uuid         NOT NULL,
    server_id      uuid         NOT NULL,
    name           varchar(128) NOT NULL,
    description    text         NOT NULL DEFAULT '',
    input_schema   jsonb        NOT NULL DEFAULT '{}'::jsonb,
    required_perms jsonb        NOT NULL DEFAULT '[]'::jsonb,
    enabled        boolean      NOT NULL DEFAULT true,
    created_at     timestamptz  NOT NULL DEFAULT now(),
    updated_at     timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS mcp_tools_tenant_server_name_uniq ON mcp_tools (tenant_id, server_id, name);

CREATE TABLE IF NOT EXISTS mcp_call_log (
    id          uuid PRIMARY KEY,
    tenant_id   uuid        NOT NULL,
    user_id     uuid        NOT NULL,
    client_id   varchar(64) NOT NULL,
    tool_name   varchar(128) NOT NULL,
    status      varchar(16) NOT NULL,
    duration_ms integer     NOT NULL DEFAULT 0,
    error_code  varchar(32),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS mcp_call_log_tenant_time_idx ON mcp_call_log (tenant_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS mcp_call_log;
DROP TABLE IF EXISTS mcp_tools;
DROP TABLE IF EXISTS mcp_servers;
