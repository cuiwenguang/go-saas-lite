-- +goose Up
ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS domain           varchar(255),
    ADD COLUMN IF NOT EXISTS subdomain        varchar(63),
    ADD COLUMN IF NOT EXISTS plan_code        varchar(64)  NOT NULL DEFAULT 'free',
    ADD COLUMN IF NOT EXISTS expire_at        timestamptz,
    ADD COLUMN IF NOT EXISTS timezone         varchar(64)  NOT NULL DEFAULT 'UTC',
    ADD COLUMN IF NOT EXISTS locale           varchar(16)  NOT NULL DEFAULT 'zh-CN',
    ADD COLUMN IF NOT EXISTS contact_name     varchar(64),
    ADD COLUMN IF NOT EXISTS contact_phone    varchar(32),
    ADD COLUMN IF NOT EXISTS contact_email    varchar(128),
    ADD COLUMN IF NOT EXISTS provision_status varchar(16)  NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS provision_step   varchar(64),
    ADD COLUMN IF NOT EXISTS deleted_at       timestamptz;

CREATE UNIQUE INDEX IF NOT EXISTS tenants_domain_uniq ON tenants (domain) WHERE domain IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS tenants_subdomain_uniq ON tenants (subdomain) WHERE subdomain IS NOT NULL;
CREATE INDEX IF NOT EXISTS tenants_status_idx ON tenants (status);

-- +goose Down
DROP INDEX IF EXISTS tenants_status_idx;
DROP INDEX IF EXISTS tenants_subdomain_uniq;
DROP INDEX IF EXISTS tenants_domain_uniq;

ALTER TABLE tenants
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS provision_step,
    DROP COLUMN IF EXISTS provision_status,
    DROP COLUMN IF EXISTS contact_email,
    DROP COLUMN IF EXISTS contact_phone,
    DROP COLUMN IF EXISTS contact_name,
    DROP COLUMN IF EXISTS locale,
    DROP COLUMN IF EXISTS timezone,
    DROP COLUMN IF EXISTS expire_at,
    DROP COLUMN IF EXISTS plan_code,
    DROP COLUMN IF EXISTS subdomain,
    DROP COLUMN IF EXISTS domain;
