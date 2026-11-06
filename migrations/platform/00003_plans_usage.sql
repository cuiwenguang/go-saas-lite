-- +goose Up
CREATE TABLE IF NOT EXISTS plans (
    id         uuid PRIMARY KEY,
    code       varchar(64)  NOT NULL,
    name       varchar(128) NOT NULL,
    quota      jsonb        NOT NULL DEFAULT '{}'::jsonb,
    enabled    boolean      NOT NULL DEFAULT true,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS plans_code_uniq ON plans (code);

-- 默认套餐：限额为 0 表示不限。
INSERT INTO plans (id, code, name, quota)
VALUES (
    gen_random_uuid(),
    'free',
    'Free',
    '{"max_users":10,"storage_gb":5,"api_qps":20,"mcp_calls":100000,"mcp_tokens":5000000,"mcp_sessions":50,"features":["sms"]}'::jsonb
)
ON CONFLICT (code) DO NOTHING;

-- 用量每日快照，由 worker 从 Redis 落库。
CREATE TABLE IF NOT EXISTS usage_daily (
    tenant_id  uuid        NOT NULL,
    metric     varchar(32) NOT NULL,
    day        date        NOT NULL,
    value      bigint      NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, metric, day)
);

-- +goose Down
DROP TABLE IF EXISTS usage_daily;
DROP TABLE IF EXISTS plans;
