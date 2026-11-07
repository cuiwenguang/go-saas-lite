-- +goose Up
-- 租户文件元数据：对象本体在 Storage 驱动里，本表只存索引与配额口径。
CREATE TABLE IF NOT EXISTS tenant_files (
    id            uuid PRIMARY KEY,
    tenant_id     uuid         NOT NULL,
    key           varchar(512) NOT NULL,
    biz           varchar(64)  NOT NULL,
    original_name varchar(255) NOT NULL DEFAULT '',
    content_type  varchar(128) NOT NULL DEFAULT '',
    size_bytes    bigint       NOT NULL DEFAULT 0,
    created_at    timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS tenant_files_key_uniq ON tenant_files (tenant_id, key);
CREATE INDEX IF NOT EXISTS tenant_files_tenant_biz_idx ON tenant_files (tenant_id, biz, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS tenant_files;
