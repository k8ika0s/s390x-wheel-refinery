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
CREATE INDEX IF NOT EXISTS idx_events_name_timestamp ON events(name, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_events_status_timestamp ON events(status, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_events_name_version_timestamp ON events(name, version, timestamp DESC);

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

CREATE TABLE IF NOT EXISTS log_chunks (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    version    TEXT NOT NULL,
    run_id     TEXT,
    attempt    INT,
    seq        BIGINT,
    content    TEXT,
    timestamp  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_log_chunks_name ON log_chunks(name);
CREATE INDEX IF NOT EXISTS idx_log_chunks_version ON log_chunks(version);
CREATE INDEX IF NOT EXISTS idx_log_chunks_name_version_id ON log_chunks(name, version, id);

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
    failure_summary TEXT,
    recipes       JSONB,
    hint_ids      TEXT[],
    run_id        TEXT,
    plan_id       BIGINT,
    leased_at     TIMESTAMPTZ,
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS worker_status (
    worker_id    TEXT PRIMARY KEY,
    run_id       TEXT,
    last_seen    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    active_builds INT NOT NULL DEFAULT 0,
    build_pool_size INT NOT NULL DEFAULT 0,
    plan_pool_size INT NOT NULL DEFAULT 0,
    heartbeat_interval_sec INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_worker_status_last_seen ON worker_status(last_seen);
CREATE INDEX IF NOT EXISTS idx_build_status_status ON build_status(status);
CREATE INDEX IF NOT EXISTS idx_build_status_plan_id ON build_status(plan_id);
CREATE INDEX IF NOT EXISTS idx_build_status_updated_at ON build_status(updated_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_build_status_pkg_version ON build_status(package, version);

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
