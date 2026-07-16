CREATE TABLE IF NOT EXISTS tags (
    id   BIGSERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS service_tags (
    service_id BIGINT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    tag_id     BIGINT NOT NULL REFERENCES tags(id)     ON DELETE CASCADE,
    PRIMARY KEY (service_id, tag_id)
);

-- No FK on service_id or changed_by so audit records survive service/user deletion.
CREATE TABLE IF NOT EXISTS service_audit_log (
    id         BIGSERIAL   PRIMARY KEY,
    service_id BIGINT      NOT NULL,
    action     TEXT        NOT NULL, -- 'create' | 'update' | 'delete'
    changed_by BIGINT      NOT NULL,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    payload    JSONB
);

CREATE INDEX IF NOT EXISTS idx_service_audit_log_service_id ON service_audit_log(service_id);
