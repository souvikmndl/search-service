CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    email_id VARCHAR(40) UNIQUE NOT NULL,
    user_name VARCHAR(15) NOT NULL,
    password TEXT NOT NULL,
    created_at TIMESTAMPTZ(0) NOT NULL DEFAULT NOW() 
);

CREATE TABLE IF NOT EXISTS services (
    id BIGSERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    created_by BIGINT NOT NULL REFERENCES users(id),
    updated_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ(0) NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ(0) NOT NULL DEFAULT NOW(),

    CONSTRAINT name_length_check CHECK (length(name) BETWEEN 3 AND 30),

    search_vector tsvector GENERATED ALWAYS AS (
        to_tsvector('english', coalesce(description, ''))
    ) STORED
);

CREATE TABLE IF NOT EXISTS service_versions (
    id BIGSERIAL PRIMARY KEY,
    service_id BIGINT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    version_string TEXT NOT NULL,
    status TEXT NOT NULL,
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ(0) NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_version UNIQUE (service_id, version_string)
);

CREATE INDEX IF NOT EXISTS idx_services_search_vector ON services 
USING GIN (search_vector);

CREATE INDEX IF NOT EXISTS idx_services_name_trgm ON services USING GIN(name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_service_versions_service_id ON service_versions(service_id);
