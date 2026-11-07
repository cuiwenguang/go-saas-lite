-- +goose Up
-- 补充 API 调用日上限：0 表示不限；缺失该字段时按 0 处理。
UPDATE plans
SET quota = jsonb_set(quota, '{api_calls}', '0'::jsonb, true),
    updated_at = now()
WHERE code = 'free'
  AND NOT (quota ? 'api_calls');

-- +goose Down
UPDATE plans
SET quota = quota - 'api_calls',
    updated_at = now()
WHERE code = 'free'
  AND quota ? 'api_calls';
