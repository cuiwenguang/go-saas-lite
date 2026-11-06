-- +goose Up
CREATE TABLE IF NOT EXISTS widgets (
    id         uuid PRIMARY KEY,
    tenant_id  uuid         NOT NULL,
    name       varchar(128) NOT NULL,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS widgets_tenant_id_idx ON widgets (tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS widgets_tenant_id_id_uniq ON widgets (tenant_id, id);

-- +goose Down
DROP TABLE IF EXISTS widgets;
