-- +goose Up
-- 租户内操作审计：与平台审计分开，按 tenant_id 隔离。
CREATE TABLE IF NOT EXISTS tenant_operation_log (
    id          uuid PRIMARY KEY,
    tenant_id   uuid         NOT NULL,
    actor_id    uuid,
    actor_email varchar(255) NOT NULL DEFAULT '',
    category    varchar(32)  NOT NULL,
    action      varchar(64)  NOT NULL,
    level       varchar(8)   NOT NULL DEFAULT 'info',
    result      varchar(8)   NOT NULL DEFAULT 'success',
    object_type varchar(32)  NOT NULL DEFAULT '',
    object_id   varchar(64)  NOT NULL DEFAULT '',
    summary     varchar(255) NOT NULL DEFAULT '',
    before_data jsonb,
    after_data  jsonb,
    request_id  varchar(64)  NOT NULL DEFAULT '',
    ip          varchar(64)  NOT NULL DEFAULT '',
    created_at  timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS tenant_operation_log_time_idx
    ON tenant_operation_log (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS tenant_operation_log_category_idx
    ON tenant_operation_log (tenant_id, category, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS tenant_operation_log;
