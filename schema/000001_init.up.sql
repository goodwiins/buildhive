CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE projects (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE api_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL,
    label        TEXT NOT NULL,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE builders (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL UNIQUE,
    address      TEXT NOT NULL,
    arch         TEXT NOT NULL DEFAULT 'amd64',
    status       TEXT NOT NULL DEFAULT 'healthy',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE builds (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    builder_id  UUID REFERENCES builders(id) ON DELETE SET NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    image_ref   TEXT,
    cache_hit   BOOLEAN NOT NULL DEFAULT FALSE,
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX builds_project_id_idx ON builds(project_id);
CREATE INDEX builds_created_at_idx ON builds(created_at DESC);

CREATE TABLE build_logs (
    id         BIGSERIAL PRIMARY KEY,
    build_id   UUID NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    ts         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    line       TEXT NOT NULL
);

CREATE INDEX build_logs_build_id_idx ON build_logs(build_id);
