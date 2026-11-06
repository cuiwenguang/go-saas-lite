-- +goose Up
-- 开通进度时间线：每次步骤结束追加一条 {step, ok, error, at}。
ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS provision_log jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS admin_email   varchar(255);

CREATE INDEX IF NOT EXISTS tenants_plan_code_idx ON tenants (plan_code);
CREATE INDEX IF NOT EXISTS tenants_expire_at_idx ON tenants (expire_at);

-- +goose Down
DROP INDEX IF EXISTS tenants_expire_at_idx;
DROP INDEX IF EXISTS tenants_plan_code_idx;

ALTER TABLE tenants
    DROP COLUMN IF EXISTS admin_email,
    DROP COLUMN IF EXISTS provision_log;
