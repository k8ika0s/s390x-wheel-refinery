-- Postgres schema draft for Go control plane
-- Tables: events, hints, logs, manifests

CREATE TABLE IF NOT EXISTS events (
    id            BIGSERIAL PRIMARY KEY,
    run_id        TEXT,
    name          TEXT NOT NULL,
    version       TEXT NOT NULL,
    python_tag    TEXT,
    platform_tag  TEXT,
    status        TEXT NOT NULL,
    detail        TEXT,
    metadata      JSONB,
    matched_hint_ids TEXT[],
    duration_ms   BIGINT,
    timestamp     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_events_name ON events(name);
CREATE INDEX IF NOT EXISTS idx_events_status ON events(status);
CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);

CREATE TABLE IF NOT EXISTS hints (
    id       TEXT PRIMARY KEY,
    pattern  TEXT NOT NULL,
    recipes  JSONB,
    note     TEXT,
    tags     JSONB,
    severity TEXT,
    applies_to JSONB,
    confidence TEXT,
    examples JSONB
);

CREATE TABLE IF NOT EXISTS logs (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    version    TEXT NOT NULL,
    content    TEXT,
    timestamp  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_logs_name ON logs(name);
CREATE INDEX IF NOT EXISTS idx_logs_version ON logs(version);

CREATE TABLE IF NOT EXISTS manifests (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT NOT NULL,
    version      TEXT NOT NULL,
    wheel        TEXT NOT NULL,
    wheel_url    TEXT,
    repair_url   TEXT,
    repair_digest TEXT,
    runtime_url  TEXT,
    pack_urls    TEXT[],
    python_tag   TEXT,
    platform_tag TEXT,
    status       TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_manifests_name ON manifests(name);
CREATE INDEX IF NOT EXISTS idx_manifests_version ON manifests(version);

-- Build plan snapshots (latest fetched for UI)
CREATE TABLE IF NOT EXISTS plans (
    id         BIGSERIAL PRIMARY KEY,
    run_id     TEXT,
    plan       JSONB NOT NULL,
    dag        JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS build_status (
    id            BIGSERIAL PRIMARY KEY,
    package       TEXT NOT NULL,
    version       TEXT NOT NULL,
    python_tag    TEXT,
    platform_tag  TEXT,
    status        TEXT NOT NULL DEFAULT 'queued',
    attempts      INT NOT NULL DEFAULT 0,
    backoff_until TIMESTAMPTZ,
    last_error    TEXT,
    run_id        TEXT,
    plan_id       BIGINT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS pending_inputs (
    id          BIGSERIAL PRIMARY KEY,
    filename    TEXT NOT NULL,
    digest      TEXT,
    size_bytes  BIGINT,
    status      TEXT NOT NULL DEFAULT 'pending',
    error       TEXT,
    source_type TEXT,
    object_bucket TEXT,
    object_key  TEXT,
    content_type TEXT,
    metadata    JSONB,
    loaded_at   TIMESTAMPTZ,
    planned_at  TIMESTAMPTZ,
    processed_at TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS plan_metadata (
    id             BIGSERIAL PRIMARY KEY,
    pending_input  BIGINT REFERENCES pending_inputs(id),
    plan_id        BIGINT REFERENCES plans(id),
    status         TEXT NOT NULL DEFAULT 'ready_for_build',
    summary        JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
