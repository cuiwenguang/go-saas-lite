-- +goose Up
-- 平台操作审计：日常运维按分类 / 动作 / 操作人 / 级别筛选，保留期由配置参数化。
CREATE TABLE IF NOT EXISTS system_operation_log (
    id          uuid PRIMARY KEY,
    category    varchar(32)  NOT NULL,
    action      varchar(64)  NOT NULL,
    level       varchar(8)   NOT NULL DEFAULT 'info',
    result      varchar(8)   NOT NULL DEFAULT 'success',
    actor_type  varchar(16)  NOT NULL DEFAULT 'admin',
    actor_id    uuid,
    actor_email varchar(255),
    object_type varchar(32)  NOT NULL DEFAULT '',
    object_id   varchar(64)  NOT NULL DEFAULT '',
    summary     varchar(255) NOT NULL DEFAULT '',
    before_data jsonb,
    after_data  jsonb,
    request_id  varchar(64)  NOT NULL DEFAULT '',
    ip          varchar(64)  NOT NULL DEFAULT '',
    created_at  timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS system_operation_log_created_idx ON system_operation_log (created_at DESC);
CREATE INDEX IF NOT EXISTS system_operation_log_category_idx ON system_operation_log (category, created_at DESC);
CREATE INDEX IF NOT EXISTS system_operation_log_object_idx ON system_operation_log (object_type, object_id, created_at DESC);
CREATE INDEX IF NOT EXISTS system_operation_log_actor_idx ON system_operation_log (actor_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS system_operation_log;
