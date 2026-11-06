-- +goose Up
CREATE TABLE IF NOT EXISTS roles (
    id          uuid PRIMARY KEY,
    tenant_id   uuid         NOT NULL,
    code        varchar(64)  NOT NULL,
    name        varchar(128) NOT NULL,
    permissions jsonb        NOT NULL DEFAULT '[]'::jsonb,
    is_builtin  boolean      NOT NULL DEFAULT false,
    created_at  timestamptz  NOT NULL DEFAULT now(),
    updated_at  timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS roles_tenant_code_uniq ON roles (tenant_id, code);
CREATE INDEX IF NOT EXISTS roles_tenant_id_idx ON roles (tenant_id);

CREATE TABLE IF NOT EXISTS user_roles (
    tenant_id  uuid        NOT NULL,
    user_id    uuid        NOT NULL,
    role_id    uuid        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS user_roles_tenant_user_role_uniq ON user_roles (tenant_id, user_id, role_id);
CREATE INDEX IF NOT EXISTS user_roles_tenant_user_idx ON user_roles (tenant_id, user_id);

-- +goose Down
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;
