-- +goose Up
-- 租户用户会话：只保留最近一次 refresh 令牌的 jti（单会话）。
-- 改密 / 登出 / 重新登录都会覆写该列，旧 refresh 令牌立即失效。
ALTER TABLE users ADD COLUMN IF NOT EXISTS refresh_jti varchar(64) NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS refresh_jti;
