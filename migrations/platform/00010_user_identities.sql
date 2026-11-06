-- +goose Up
-- 全局登录索引：登录标识 → (tenant_id, user_id)。
-- 只做定位，不存密码与资料；租户库 users 是唯一真相源（readme 1.5）。
CREATE TABLE IF NOT EXISTS user_identities (
    identifier_type varchar(16)  NOT NULL,
    identifier      varchar(255) NOT NULL,
    tenant_id       uuid         NOT NULL,
    user_id         uuid         NOT NULL,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    PRIMARY KEY (identifier_type, identifier)
);

CREATE INDEX IF NOT EXISTS user_identities_tenant_user_idx ON user_identities (tenant_id, user_id);

-- +goose Down
DROP TABLE IF EXISTS user_identities;
