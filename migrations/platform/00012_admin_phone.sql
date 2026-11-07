-- +goose Up
-- 手机号主标识：平台侧记录租户管理员手机号，用于重试开通与重置密码定位。
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS admin_phone varchar(32);

CREATE INDEX IF NOT EXISTS tenants_admin_phone_idx ON tenants (admin_phone);

-- +goose Down
DROP INDEX IF EXISTS tenants_admin_phone_idx;
ALTER TABLE tenants DROP COLUMN IF EXISTS admin_phone;
