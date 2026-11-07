-- +goose Up
-- 手机号主标识：邮箱可缺省。用部分唯一索引保证“有邮箱时仍租户内唯一”。
ALTER TABLE users ALTER COLUMN email SET DEFAULT '';

DROP INDEX IF EXISTS users_tenant_email_uniq;
CREATE UNIQUE INDEX IF NOT EXISTS users_tenant_email_uniq
    ON users (tenant_id, email) WHERE email <> '';

-- +goose Down
DROP INDEX IF EXISTS users_tenant_email_uniq;
CREATE UNIQUE INDEX IF NOT EXISTS users_tenant_email_uniq ON users (tenant_id, email);
