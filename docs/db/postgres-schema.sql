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
    note     TEXT
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
