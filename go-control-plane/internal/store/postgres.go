package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/settings"
	"github.com/lib/pq"
)

// PostgresStore implements Store using Postgres.
type PostgresStore struct {
	db *sql.DB
}

func (p *PostgresStore) ensureDB() error {
	if p == nil || p.db == nil {
		return fmt.Errorf("db not configured")
	}
	return nil
}

// NewPostgres creates a new store with an existing *sql.DB.
func NewPostgres(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

const schema = `
CREATE EXTENSION IF NOT EXISTS pg_trgm;

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
    examples JSONB,
    deleted_at TIMESTAMPTZ
);

ALTER TABLE hints ADD COLUMN IF NOT EXISTS tags JSONB;
ALTER TABLE hints ADD COLUMN IF NOT EXISTS severity TEXT;
ALTER TABLE hints ADD COLUMN IF NOT EXISTS applies_to JSONB;
ALTER TABLE hints ADD COLUMN IF NOT EXISTS confidence TEXT;
ALTER TABLE hints ADD COLUMN IF NOT EXISTS examples JSONB;
ALTER TABLE hints ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_hints_pattern_trgm ON hints USING GIN (pattern gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_hints_note_trgm ON hints USING GIN (note gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_hints_tags_trgm ON hints USING GIN ((tags::text) gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_hints_recipes_trgm ON hints USING GIN ((recipes::text) gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_hints_applies_trgm ON hints USING GIN ((applies_to::text) gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_hints_examples_trgm ON hints USING GIN ((examples::text) gin_trgm_ops);

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

ALTER TABLE manifests ADD COLUMN IF NOT EXISTS wheel_url TEXT;
ALTER TABLE manifests ADD COLUMN IF NOT EXISTS runtime_url TEXT;
ALTER TABLE manifests ADD COLUMN IF NOT EXISTS pack_urls TEXT[];
ALTER TABLE manifests ADD COLUMN IF NOT EXISTS repair_url TEXT;
ALTER TABLE manifests ADD COLUMN IF NOT EXISTS repair_digest TEXT;

CREATE TABLE IF NOT EXISTS app_settings (
    id         INT PRIMARY KEY DEFAULT 1,
    payload    JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS plans (
    id         BIGSERIAL PRIMARY KEY,
    run_id     TEXT,
    plan       JSONB NOT NULL,
    dag        JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE plans ADD COLUMN IF NOT EXISTS dag JSONB;

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
    recipes       JSONB,
    hint_ids      TEXT[],
    run_id        TEXT,
    plan_id       BIGINT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_build_status_pkg ON build_status(package, version);
CREATE UNIQUE INDEX IF NOT EXISTS idx_build_status_pkg_version_unique ON build_status(package, version);
CREATE INDEX IF NOT EXISTS idx_build_status_status ON build_status(status);

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
CREATE INDEX IF NOT EXISTS idx_pending_inputs_status ON pending_inputs(status);
CREATE INDEX IF NOT EXISTS idx_pending_inputs_deleted ON pending_inputs(deleted_at);

ALTER TABLE pending_inputs ADD COLUMN IF NOT EXISTS source_type TEXT;
ALTER TABLE pending_inputs ADD COLUMN IF NOT EXISTS object_bucket TEXT;
ALTER TABLE pending_inputs ADD COLUMN IF NOT EXISTS object_key TEXT;
ALTER TABLE pending_inputs ADD COLUMN IF NOT EXISTS content_type TEXT;
ALTER TABLE pending_inputs ADD COLUMN IF NOT EXISTS metadata JSONB;
ALTER TABLE pending_inputs ADD COLUMN IF NOT EXISTS loaded_at TIMESTAMPTZ;
ALTER TABLE pending_inputs ADD COLUMN IF NOT EXISTS planned_at TIMESTAMPTZ;
ALTER TABLE pending_inputs ADD COLUMN IF NOT EXISTS processed_at TIMESTAMPTZ;
ALTER TABLE pending_inputs ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE build_status ADD COLUMN IF NOT EXISTS recipes JSONB;
ALTER TABLE build_status ADD COLUMN IF NOT EXISTS hint_ids TEXT[];

CREATE TABLE IF NOT EXISTS plan_metadata (
    id             BIGSERIAL PRIMARY KEY,
    pending_input  BIGINT REFERENCES pending_inputs(id),
    plan_id        BIGINT REFERENCES plans(id),
    status         TEXT NOT NULL DEFAULT 'ready_for_build',
    summary        JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

// RunMigrations ensures schema is present.
func RunMigrations(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	_, err := db.ExecContext(ctx, schema)
	return err
}

// Ping validates DB connectivity; optional for health checks.
func (p *PostgresStore) Ping(ctx context.Context) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	return p.db.PingContext(ctx)
}

// GetSettings returns persisted settings, or defaults if none stored.
func (p *PostgresStore) GetSettings(ctx context.Context) (settings.Settings, error) {
	if err := p.ensureDB(); err != nil {
		return settings.ApplyDefaults(settings.Settings{}), err
	}
	var payload []byte
	err := p.db.QueryRowContext(ctx, `SELECT payload FROM app_settings WHERE id = 1`).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return settings.ApplyDefaults(settings.Settings{}), nil
	}
	if err != nil {
		return settings.ApplyDefaults(settings.Settings{}), err
	}
	var s settings.Settings
	if err := json.Unmarshal(payload, &s); err != nil {
		return settings.ApplyDefaults(settings.Settings{}), err
	}
	return settings.ApplyDefaults(s), nil
}

// SaveSettings upserts settings into the DB.
func (p *PostgresStore) SaveSettings(ctx context.Context, s settings.Settings) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	s = settings.ApplyDefaults(s)
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, `
		INSERT INTO app_settings (id, payload, updated_at)
		VALUES (1, $1, NOW())
		ON CONFLICT (id) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()
	`, data)
	return err
}

// AddPendingInput inserts a new pending input or restores a deleted match by digest.
func (p *PostgresStore) AddPendingInput(ctx context.Context, pi PendingInput) (int64, error) {
	if err := p.ensureDB(); err != nil {
		return 0, err
	}
	if pi.Digest != "" {
		var restoredID int64
		restoreErr := p.db.QueryRowContext(ctx, `
			UPDATE pending_inputs
			SET filename = $2,
				size_bytes = $3,
				status = $4,
				error = '',
				source_type = $5,
				object_bucket = $6,
				object_key = $7,
				content_type = $8,
				metadata = $9,
				loaded_at = NULL,
				planned_at = NULL,
				processed_at = NULL,
				deleted_at = NULL,
				updated_at = NOW()
			WHERE id = (
				SELECT id FROM pending_inputs
				WHERE digest = $1 AND deleted_at IS NOT NULL
				ORDER BY deleted_at DESC
				LIMIT 1
			)
			RETURNING id
		`, pi.Digest, pi.Filename, pi.SizeBytes, pi.Status, pi.SourceType, pi.ObjectBucket, pi.ObjectKey, pi.ContentType, pi.Metadata).Scan(&restoredID)
		if restoreErr == nil && restoredID > 0 {
			return restoredID, nil
		}
		if restoreErr != nil && !errors.Is(restoreErr, sql.ErrNoRows) {
			return 0, restoreErr
		}
	}
	var id int64
	err := p.db.QueryRowContext(ctx, `
		INSERT INTO pending_inputs (
			filename, digest, size_bytes, status,
			source_type, object_bucket, object_key, content_type, metadata
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id
	`, pi.Filename, pi.Digest, pi.SizeBytes, pi.Status, pi.SourceType, pi.ObjectBucket, pi.ObjectKey, pi.ContentType, pi.Metadata).Scan(&id)
	return id, err
}

// ListPendingInputs fetches pending inputs.
func (p *PostgresStore) ListPendingInputs(ctx context.Context, status string) ([]PendingInput, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	q := `SELECT pi.id, pi.filename, pi.digest, pi.size_bytes, pi.status, COALESCE(pi.error,''),
		pi.source_type, pi.object_bucket, pi.object_key, pi.content_type, COALESCE(pi.metadata,'{}'),
		pi.loaded_at, pi.planned_at, pi.processed_at, pi.deleted_at, pi.created_at, pi.updated_at,
		pm.plan_id
		FROM pending_inputs pi
		LEFT JOIN LATERAL (
			SELECT plan_id FROM plan_metadata pm
			WHERE pm.pending_input = pi.id
			ORDER BY pm.created_at DESC
			LIMIT 1
		) pm ON true
		WHERE pi.deleted_at IS NULL`
	args := []any{}
	if status != "" {
		q += ` AND status = $1`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingInput
	for rows.Next() {
		var pi PendingInput
		var planID sql.NullInt64
		if err := rows.Scan(
			&pi.ID,
			&pi.Filename,
			&pi.Digest,
			&pi.SizeBytes,
			&pi.Status,
			&pi.Error,
			&pi.SourceType,
			&pi.ObjectBucket,
			&pi.ObjectKey,
			&pi.ContentType,
			&pi.Metadata,
			&pi.LoadedAt,
			&pi.PlannedAt,
			&pi.ProcessedAt,
			&pi.DeletedAt,
			&pi.CreatedAt,
			&pi.UpdatedAt,
			&planID,
		); err != nil {
			return nil, err
		}
		if planID.Valid {
			pi.PlanID = &planID.Int64
		}
		out = append(out, pi)
	}
	return out, rows.Err()
}

// UpdatePendingInputStatus sets status/error.
func (p *PostgresStore) UpdatePendingInputStatus(ctx context.Context, id int64, status, errMsg string) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	var loadedAt, plannedAt, processedAt *time.Time
	now := time.Now().UTC()
	switch status {
	case "planning":
		loadedAt = &now
	case "planned":
		plannedAt = &now
	case "queued", "build_queued":
		processedAt = &now
	}
	_, err := p.db.ExecContext(ctx, `
		UPDATE pending_inputs
		SET status = $1,
			error = $2,
			loaded_at = COALESCE($3, loaded_at),
			planned_at = COALESCE($4, planned_at),
			processed_at = COALESCE($5, processed_at),
			updated_at = NOW()
		WHERE id = $6
	`, status, errMsg, loadedAt, plannedAt, processedAt, id)
	return err
}

// DeletePendingInput removes a pending input and returns the deleted record.
func (p *PostgresStore) DeletePendingInput(ctx context.Context, id int64) (PendingInput, error) {
	if err := p.ensureDB(); err != nil {
		return PendingInput{}, err
	}
	var pi PendingInput
	err := p.db.QueryRowContext(ctx, `
		UPDATE pending_inputs
		SET status = 'deleted', deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1
		RETURNING id, filename, digest, size_bytes, status, COALESCE(error,''),
			source_type, object_bucket, object_key, content_type, COALESCE(metadata,'{}'),
			loaded_at, planned_at, processed_at, deleted_at, created_at, updated_at
	`, id).Scan(
		&pi.ID,
		&pi.Filename,
		&pi.Digest,
		&pi.SizeBytes,
		&pi.Status,
		&pi.Error,
		&pi.SourceType,
		&pi.ObjectBucket,
		&pi.ObjectKey,
		&pi.ContentType,
		&pi.Metadata,
		&pi.LoadedAt,
		&pi.PlannedAt,
		&pi.ProcessedAt,
		&pi.DeletedAt,
		&pi.CreatedAt,
		&pi.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PendingInput{}, ErrNotFound
	}
	return pi, err
}

// RestorePendingInput resets a pending input to pending and clears deletion state.
func (p *PostgresStore) RestorePendingInput(ctx context.Context, id int64) (PendingInput, error) {
	if err := p.ensureDB(); err != nil {
		return PendingInput{}, err
	}
	var pi PendingInput
	err := p.db.QueryRowContext(ctx, `
		UPDATE pending_inputs
		SET status = 'pending',
			error = '',
			loaded_at = NULL,
			planned_at = NULL,
			processed_at = NULL,
			deleted_at = NULL,
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, filename, digest, size_bytes, status, COALESCE(error,''),
			source_type, object_bucket, object_key, content_type, COALESCE(metadata,'{}'),
			loaded_at, planned_at, processed_at, deleted_at, created_at, updated_at
	`, id).Scan(
		&pi.ID,
		&pi.Filename,
		&pi.Digest,
		&pi.SizeBytes,
		&pi.Status,
		&pi.Error,
		&pi.SourceType,
		&pi.ObjectBucket,
		&pi.ObjectKey,
		&pi.ContentType,
		&pi.Metadata,
		&pi.LoadedAt,
		&pi.PlannedAt,
		&pi.ProcessedAt,
		&pi.DeletedAt,
		&pi.CreatedAt,
		&pi.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PendingInput{}, ErrNotFound
	}
	return pi, err
}

// LinkPlanToPendingInput records a plan association for a pending input.
func (p *PostgresStore) LinkPlanToPendingInput(ctx context.Context, pendingID, planID int64) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO plan_metadata (pending_input, plan_id, status)
		VALUES ($1, $2, 'ready_for_build')
	`, pendingID, planID)
	return err
}

// UpdatePendingInputsForPlan updates pending input status based on plan_id.
func (p *PostgresStore) UpdatePendingInputsForPlan(ctx context.Context, planID int64, status string) (int64, error) {
	if err := p.ensureDB(); err != nil {
		return 0, err
	}
	var plannedAt, processedAt *time.Time
	now := time.Now().UTC()
	switch status {
	case "planned":
		plannedAt = &now
	case "queued", "build_queued":
		processedAt = &now
	case "pending":
	}
	resetTimes := status == "pending"
	q := `
		UPDATE pending_inputs
		SET status = $1,
			error = '',
			planned_at = CASE WHEN $4 THEN NULL WHEN $2 IS NULL THEN planned_at ELSE $2 END,
			processed_at = CASE WHEN $4 THEN NULL WHEN $3 IS NULL THEN processed_at ELSE $3 END,
			updated_at = NOW()
		WHERE deleted_at IS NULL AND id IN (
			SELECT pending_input FROM plan_metadata`
	args := []any{status, plannedAt, processedAt, resetTimes}
	if planID > 0 {
		q += ` WHERE plan_id = $5`
		args = append(args, planID)
	}
	q += `)`
	res, err := p.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	count, _ := res.RowsAffected()
	return count, nil
}

// ListBuilds returns build status rows filtered by status if provided.
func (p *PostgresStore) ListBuilds(ctx context.Context, status string, limit int) ([]BuildStatus, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	q := `SELECT id, package, version, python_tag, platform_tag, status, attempts, COALESCE(last_error,''), run_id, plan_id, extract(epoch from (NOW() - created_at))::bigint as age, extract(epoch from created_at)::bigint, extract(epoch from updated_at)::bigint, COALESCE(extract(epoch from backoff_until),0)::bigint, COALESCE(recipes, '[]'::jsonb), COALESCE(hint_ids, '{}'::text[]) FROM build_status`
	args := []any{}
	if status != "" {
		q += ` WHERE status = $1`
		args = append(args, status)
	}
	q += ` ORDER BY created_at ASC`
	if limit > 0 {
		limitIdx := 1
		if status != "" {
			limitIdx = 2
		}
		q += fmt.Sprintf(" LIMIT $%d::int", limitIdx)
		args = append(args, limit)
	}
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BuildStatus
	for rows.Next() {
		var bs BuildStatus
		var recipes json.RawMessage
		var hints pq.StringArray
		if err := rows.Scan(&bs.ID, &bs.Package, &bs.Version, &bs.PythonTag, &bs.PlatformTag, &bs.Status, &bs.Attempts, &bs.LastError, &bs.RunID, &bs.PlanID, &bs.OldestAgeSec, &bs.CreatedAt, &bs.UpdatedAt, &bs.BackoffUntil, &recipes, &hints); err != nil {
			return nil, err
		}
		if len(recipes) > 0 {
			_ = json.Unmarshal(recipes, &bs.Recipes)
		}
		if len(hints) > 0 {
			bs.HintIDs = hints
		}
		out = append(out, bs)
	}
	return out, rows.Err()
}

// DeleteBuilds removes build status rows matching the status filter.
func (p *PostgresStore) DeleteBuilds(ctx context.Context, status string) (int64, error) {
	if err := p.ensureDB(); err != nil {
		return 0, err
	}
	var (
		res sql.Result
		err error
	)
	if status == "" {
		res, err = p.db.ExecContext(ctx, `DELETE FROM build_status`)
	} else {
		res, err = p.db.ExecContext(ctx, `DELETE FROM build_status WHERE status = $1`, status)
	}
	if err != nil {
		return 0, err
	}
	count, _ := res.RowsAffected()
	return count, nil
}

// UpdateBuildStatus upserts build status by package/version.
func (p *PostgresStore) UpdateBuildStatus(ctx context.Context, pkg, version, status, errMsg string, attempts int, backoffUntil int64, recipes []string, hintIDs []string) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	if attempts < 0 {
		attempts = 0
	}
	var backoff any
	if backoffUntil > 0 {
		backoff = time.Unix(backoffUntil, 0)
	}
	var recipesRaw any
	if recipes != nil {
		if data, err := json.Marshal(recipes); err == nil {
			recipesRaw = data
		}
	}
	var hints any
	if hintIDs != nil {
		hints = pqStringArrayParam(hintIDs)
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO build_status (package, version, status, last_error, attempts, backoff_until, recipes, hint_ids)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (package, version) DO UPDATE
		SET status = EXCLUDED.status,
		    last_error = EXCLUDED.last_error,
		    attempts = EXCLUDED.attempts,
		    backoff_until = EXCLUDED.backoff_until,
		    recipes = COALESCE(EXCLUDED.recipes, build_status.recipes),
		    hint_ids = COALESCE(EXCLUDED.hint_ids, build_status.hint_ids),
		    updated_at = NOW()
	`, pkg, version, status, errMsg, attempts, backoff, recipesRaw, hints)
	return err
}

func (p *PostgresStore) Recent(ctx context.Context, limit, offset int, pkg, status string) ([]Event, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	q := `SELECT run_id,name,version,python_tag,platform_tag,status,detail,metadata,matched_hint_ids,extract(epoch from timestamp)::bigint,duration_ms
	      FROM events WHERE 1=1`
	args := []any{}
	if pkg != "" {
		args = append(args, pkg)
		q += fmt.Sprintf(" AND name = $%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		q += fmt.Sprintf(" AND status = $%d", len(args))
	}
	args = append(args, limit)
	q += fmt.Sprintf(" ORDER BY timestamp DESC LIMIT $%d", len(args))
	if offset > 0 {
		args = append(args, offset)
		q += fmt.Sprintf(" OFFSET $%d", len(args))
	}
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var metaRaw json.RawMessage
		var matched pq.StringArray
		if err := rows.Scan(&e.RunID, &e.Name, &e.Version, &e.PythonTag, &e.PlatformTag, &e.Status, &e.Detail, &metaRaw, &matched, &e.Timestamp, &e.DurationMS); err != nil {
			return nil, err
		}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &e.Metadata)
		}
		e.MatchedHintIDs = matched
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *PostgresStore) History(ctx context.Context, filter HistoryFilter) ([]Event, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	q := `SELECT run_id,name,version,python_tag,platform_tag,status,detail,metadata,matched_hint_ids,extract(epoch from timestamp)::bigint,duration_ms
	      FROM events WHERE 1=1`
	args := []any{}
	if filter.Package != "" {
		args = append(args, filter.Package)
		q += fmt.Sprintf(" AND name = $%d", len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		q += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if filter.RunID != "" {
		args = append(args, filter.RunID)
		q += fmt.Sprintf(" AND run_id = $%d", len(args))
	}
	if filter.FromTs > 0 {
		args = append(args, filter.FromTs)
		q += fmt.Sprintf(" AND extract(epoch from timestamp) >= $%d", len(args))
	}
	if filter.ToTs > 0 {
		args = append(args, filter.ToTs)
		q += fmt.Sprintf(" AND extract(epoch from timestamp) <= $%d", len(args))
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 500 {
		filter.Limit = 500
	}
	args = append(args, filter.Limit)
	q += fmt.Sprintf(" ORDER BY timestamp DESC LIMIT $%d", len(args))
	if filter.Offset > 0 {
		args = append(args, filter.Offset)
		q += fmt.Sprintf(" OFFSET $%d", len(args))
	}
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var metaRaw json.RawMessage
		var matched pq.StringArray
		if err := rows.Scan(&e.RunID, &e.Name, &e.Version, &e.PythonTag, &e.PlatformTag, &e.Status, &e.Detail, &metaRaw, &matched, &e.Timestamp, &e.DurationMS); err != nil {
			return nil, err
		}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &e.Metadata)
		}
		e.MatchedHintIDs = matched
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *PostgresStore) RecordEvent(ctx context.Context, evt Event) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	metaBytes, _ := json.Marshal(evt.Metadata)
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO events (run_id,name,version,python_tag,platform_tag,status,detail,metadata,matched_hint_ids,timestamp)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,TO_TIMESTAMP($10))`,
		evt.RunID, evt.Name, evt.Version, evt.PythonTag, evt.PlatformTag, evt.Status, evt.Detail, metaBytes, pq.Array(evt.MatchedHintIDs), evt.Timestamp)
	return err
}

func (p *PostgresStore) Summary(ctx context.Context, failureLimit int) (Summary, error) {
	if err := p.ensureDB(); err != nil {
		return Summary{}, err
	}
	if failureLimit <= 0 {
		failureLimit = 20
	}
	out := Summary{StatusCounts: map[string]int{}}
	rows, err := p.db.QueryContext(ctx, `SELECT status, count(*) FROM events GROUP BY status`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return out, err
		}
		out.StatusCounts[status] = count
	}
	failureRows, err := p.db.QueryContext(ctx, `SELECT run_id,name,version,python_tag,platform_tag,status,detail,metadata,matched_hint_ids,extract(epoch from timestamp)::bigint
		FROM events WHERE status='failed' ORDER BY timestamp DESC LIMIT $1`, failureLimit)
	if err != nil {
		return out, err
	}
	defer failureRows.Close()
	for failureRows.Next() {
		var e Event
		var metaRaw json.RawMessage
		var matched pq.StringArray
		if err := failureRows.Scan(&e.RunID, &e.Name, &e.Version, &e.PythonTag, &e.PlatformTag, &e.Status, &e.Detail, &metaRaw, &matched, &e.Timestamp); err != nil {
			return out, err
		}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &e.Metadata)
		}
		e.MatchedHintIDs = matched
		out.Failures = append(out.Failures, e)
	}
	return out, failureRows.Err()
}

func (p *PostgresStore) PackageSummary(ctx context.Context, name string) (PackageSummary, error) {
	if err := p.ensureDB(); err != nil {
		return PackageSummary{}, err
	}
	ps := PackageSummary{Name: name, StatusCounts: map[string]int{}}
	rows, err := p.db.QueryContext(ctx, `SELECT status, count(*) FROM events WHERE name=$1 GROUP BY status`, name)
	if err != nil {
		return ps, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return ps, err
		}
		ps.StatusCounts[status] = count
	}
	var e Event
	var metaRaw json.RawMessage
	var matched pq.StringArray
	err = p.db.QueryRowContext(ctx, `SELECT run_id,name,version,python_tag,platform_tag,status,detail,metadata,matched_hint_ids,extract(epoch from timestamp)::bigint
		FROM events WHERE name=$1 ORDER BY timestamp DESC LIMIT 1`, name).Scan(&e.RunID, &e.Name, &e.Version, &e.PythonTag, &e.PlatformTag, &e.Status, &e.Detail, &metaRaw, &matched, &e.Timestamp)
	if err == nil {
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &e.Metadata)
		}
		e.MatchedHintIDs = matched
		ps.Latest = &e
	}
	return ps, nil
}

func (p *PostgresStore) LatestEvent(ctx context.Context, name, version string) (Event, error) {
	if err := p.ensureDB(); err != nil {
		return Event{}, err
	}
	var e Event
	var metaRaw json.RawMessage
	var matched pq.StringArray
	err := p.db.QueryRowContext(ctx, `SELECT run_id,name,version,python_tag,platform_tag,status,detail,metadata,matched_hint_ids,extract(epoch from timestamp)::bigint
		FROM events WHERE name=$1 AND version=$2 ORDER BY timestamp DESC LIMIT 1`, name, version).Scan(&e.RunID, &e.Name, &e.Version, &e.PythonTag, &e.PlatformTag, &e.Status, &e.Detail, &metaRaw, &matched, &e.Timestamp)
	if err != nil {
		return Event{}, err
	}
	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &e.Metadata)
	}
	e.MatchedHintIDs = matched
	return e, nil
}

func (p *PostgresStore) Failures(ctx context.Context, name string, limit int) ([]Event, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	args := []any{limit}
	q := `SELECT run_id,name,version,python_tag,platform_tag,status,detail,metadata,matched_hint_ids,extract(epoch from timestamp)::bigint
		FROM events WHERE status='failed'`
	if name != "" {
		args = append([]any{name}, args...)
		q += " AND name=$1"
	}
	q += fmt.Sprintf(" ORDER BY timestamp DESC LIMIT $%d", len(args))
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var metaRaw json.RawMessage
		var matched pq.StringArray
		if err := rows.Scan(&e.RunID, &e.Name, &e.Version, &e.PythonTag, &e.PlatformTag, &e.Status, &e.Detail, &metaRaw, &matched, &e.Timestamp); err != nil {
			return nil, err
		}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &e.Metadata)
		}
		e.MatchedHintIDs = matched
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *PostgresStore) Variants(ctx context.Context, name string, limit int) ([]Event, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.db.QueryContext(ctx, `SELECT run_id,name,version,python_tag,platform_tag,status,detail,metadata,matched_hint_ids,extract(epoch from timestamp)::bigint
		FROM events WHERE name=$1 ORDER BY timestamp DESC LIMIT $2`, name, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var metaRaw json.RawMessage
		var matched pq.StringArray
		if err := rows.Scan(&e.RunID, &e.Name, &e.Version, &e.PythonTag, &e.PlatformTag, &e.Status, &e.Detail, &metaRaw, &matched, &e.Timestamp); err != nil {
			return nil, err
		}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &e.Metadata)
		}
		e.MatchedHintIDs = matched
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *PostgresStore) TopFailures(ctx context.Context, limit int) ([]Stat, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := p.db.QueryContext(ctx, `SELECT name, count(*)::float FROM events WHERE status='failed' GROUP BY name ORDER BY count(*) DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Stat
	for rows.Next() {
		var st Stat
		if err := rows.Scan(&st.Name, &st.Value); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (p *PostgresStore) TopSlowest(ctx context.Context, limit int) ([]Stat, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := p.db.QueryContext(ctx, `SELECT name, avg((metadata->>'duration_ms')::bigint)::float AS avg_ms
		FROM events WHERE metadata ? 'duration_ms' GROUP BY name ORDER BY avg_ms DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Stat
	for rows.Next() {
		var st Stat
		if err := rows.Scan(&st.Name, &st.Value); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (p *PostgresStore) ListHints(ctx context.Context) ([]Hint, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	rows, err := p.db.QueryContext(ctx, `SELECT id,pattern,recipes,note,tags,severity,applies_to,confidence,examples,deleted_at FROM hints WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHints(rows)
}

// ListHintsPaged returns hints with optional search and paging.
func (p *PostgresStore) ListHintsPaged(ctx context.Context, limit, offset int, query string) ([]Hint, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	args := []any{}
	whereClause := "WHERE deleted_at IS NULL"
	if strings.TrimSpace(query) != "" {
		args = append(args, "%"+query+"%")
		whereClause = `
			WHERE deleted_at IS NULL AND (
				   id ILIKE $1
				OR pattern ILIKE $1
				OR note ILIKE $1
				OR tags::text ILIKE $1
				OR recipes::text ILIKE $1
				OR applies_to::text ILIKE $1
				OR examples::text ILIKE $1)`
	}
	limitIdx := len(args) + 1
	offsetIdx := len(args) + 2
	args = append(args, limit, offset)
	querySQL := fmt.Sprintf(`SELECT id,pattern,recipes,note,tags,severity,applies_to,confidence,examples,deleted_at
		FROM hints %s ORDER BY id LIMIT $%d OFFSET $%d`, whereClause, limitIdx, offsetIdx)
	rows, err := p.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHints(rows)
}

func scanHints(rows *sql.Rows) ([]Hint, error) {
	var out []Hint
	for rows.Next() {
		var h Hint
		var recipes json.RawMessage
		var tags json.RawMessage
		var applies json.RawMessage
		var examples json.RawMessage
		var deletedAt sql.NullTime
		if err := rows.Scan(&h.ID, &h.Pattern, &recipes, &h.Note, &tags, &h.Severity, &applies, &h.Confidence, &examples, &deletedAt); err != nil {
			return nil, err
		}
		if len(recipes) > 0 {
			_ = json.Unmarshal(recipes, &h.Recipes)
		}
		if len(tags) > 0 {
			_ = json.Unmarshal(tags, &h.Tags)
		}
		if len(applies) > 0 {
			_ = json.Unmarshal(applies, &h.AppliesTo)
		}
		if len(examples) > 0 {
			_ = json.Unmarshal(examples, &h.Examples)
		}
		if deletedAt.Valid {
			t := deletedAt.Time
			h.DeletedAt = &t
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// HintCount returns the total number of hint entries.
func (p *PostgresStore) HintCount(ctx context.Context) (int, error) {
	if err := p.ensureDB(); err != nil {
		return 0, err
	}
	var count int
	if err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM hints`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (p *PostgresStore) GetHint(ctx context.Context, id string) (Hint, error) {
	if err := p.ensureDB(); err != nil {
		return Hint{}, err
	}
	var h Hint
	var recipes json.RawMessage
	var tags json.RawMessage
	var applies json.RawMessage
	var examples json.RawMessage
	err := p.db.QueryRowContext(ctx, `SELECT id,pattern,recipes,note,tags,severity,applies_to,confidence,examples FROM hints WHERE id=$1`, id).
		Scan(&h.ID, &h.Pattern, &recipes, &h.Note, &tags, &h.Severity, &applies, &h.Confidence, &examples)
	if err != nil {
		return Hint{}, err
	}
	if len(recipes) > 0 {
		_ = json.Unmarshal(recipes, &h.Recipes)
	}
	if len(tags) > 0 {
		_ = json.Unmarshal(tags, &h.Tags)
	}
	if len(applies) > 0 {
		_ = json.Unmarshal(applies, &h.AppliesTo)
	}
	if len(examples) > 0 {
		_ = json.Unmarshal(examples, &h.Examples)
	}
	return h, nil
}

func (p *PostgresStore) PutHint(ctx context.Context, hint Hint) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	recipes, _ := json.Marshal(hint.Recipes)
	tags, _ := json.Marshal(hint.Tags)
	applies, _ := json.Marshal(hint.AppliesTo)
	examples, _ := json.Marshal(hint.Examples)
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO hints (id,pattern,recipes,note,tags,severity,applies_to,confidence,examples)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	    ON CONFLICT (id) DO UPDATE
	    SET pattern=EXCLUDED.pattern,
	        recipes=EXCLUDED.recipes,
	        note=EXCLUDED.note,
	        tags=EXCLUDED.tags,
	        severity=EXCLUDED.severity,
	        applies_to=EXCLUDED.applies_to,
	        confidence=EXCLUDED.confidence,
	        examples=EXCLUDED.examples,
	        deleted_at=NULL`,
		hint.ID, hint.Pattern, recipes, hint.Note, tags, hint.Severity, applies, hint.Confidence, examples)
	return err
}

func (p *PostgresStore) DeleteHint(ctx context.Context, id string) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	_, err := p.db.ExecContext(ctx, `UPDATE hints SET deleted_at=NOW() WHERE id=$1`, id)
	return err
}

func (p *PostgresStore) GetLog(ctx context.Context, name, version string) (LogEntry, error) {
	if err := p.ensureDB(); err != nil {
		return LogEntry{}, err
	}
	var le LogEntry
	err := p.db.QueryRowContext(ctx, `
	    SELECT name,version,content,extract(epoch from timestamp)::bigint FROM logs
	    WHERE name=$1 AND version=$2 ORDER BY timestamp DESC LIMIT 1`, name, version).Scan(&le.Name, &le.Version, &le.Content, &le.Timestamp)
	return le, err
}

func (p *PostgresStore) PutLog(ctx context.Context, entry LogEntry) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	_, err := p.db.ExecContext(ctx, `INSERT INTO logs (name,version,content,timestamp) VALUES ($1,$2,$3,TO_TIMESTAMP($4))`, entry.Name, entry.Version, entry.Content, entry.Timestamp)
	return err
}

func (p *PostgresStore) SearchLogs(ctx context.Context, q string, limit int) ([]LogEntry, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := p.db.QueryContext(ctx, `
	    SELECT name,version,content,extract(epoch from timestamp)::bigint
	    FROM logs WHERE content ILIKE $1 ORDER BY timestamp DESC LIMIT $2`, "%"+q+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var le LogEntry
		if err := rows.Scan(&le.Name, &le.Version, &le.Content, &le.Timestamp); err != nil {
			return nil, err
		}
		out = append(out, le)
	}
	return out, rows.Err()
}

func (p *PostgresStore) Manifest(ctx context.Context, limit int) ([]ManifestEntry, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := p.db.QueryContext(ctx, `SELECT name,version,wheel,wheel_url,repair_url,repair_digest,runtime_url,pack_urls,python_tag,platform_tag,status,extract(epoch from created_at)::bigint FROM manifests ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManifestEntry
	for rows.Next() {
		var m ManifestEntry
		var packs pq.StringArray
		if err := rows.Scan(&m.Name, &m.Version, &m.Wheel, &m.WheelURL, &m.RepairURL, &m.RepairDigest, &m.RuntimeURL, &packs, &m.PythonTag, &m.PlatformTag, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.PackURLs = []string(packs)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (p *PostgresStore) SaveManifest(ctx context.Context, entries []ManifestEntry) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	for _, m := range entries {
		if m.CreatedAt == 0 {
			m.CreatedAt = time.Now().Unix()
		}
		_, err := p.db.ExecContext(ctx, `INSERT INTO manifests (name,version,wheel,wheel_url,repair_url,repair_digest,runtime_url,pack_urls,python_tag,platform_tag,status,created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,TO_TIMESTAMP($12))`,
			m.Name, m.Version, m.Wheel, m.WheelURL, m.RepairURL, m.RepairDigest, m.RuntimeURL, pq.StringArray(m.PackURLs), m.PythonTag, m.PlatformTag, m.Status, m.CreatedAt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PostgresStore) Artifacts(ctx context.Context, limit int) ([]Artifact, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	manifests, err := p.Manifest(ctx, limit)
	if err != nil {
		return nil, err
	}
	var out []Artifact
	for _, m := range manifests {
		out = append(out, Artifact{Name: m.Name, Version: m.Version, Path: m.Wheel, URL: m.WheelURL})
	}
	return out, nil
}

func (p *PostgresStore) Plan(ctx context.Context) ([]PlanNode, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	rows, err := p.db.QueryContext(ctx, `SELECT plan FROM plans ORDER BY created_at DESC LIMIT 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		var raw json.RawMessage
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var nodes []PlanNode
		if err := json.Unmarshal(raw, &nodes); err != nil {
			return nil, err
		}
		return nodes, nil
	}
	return []PlanNode{}, nil
}

func (p *PostgresStore) SavePlan(ctx context.Context, runID string, nodes []PlanNode, dag json.RawMessage) (int64, error) {
	if err := p.ensureDB(); err != nil {
		return 0, err
	}
	data, err := json.Marshal(nodes)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := p.db.QueryRowContext(ctx, `INSERT INTO plans (run_id, plan, dag) VALUES ($1, $2, $3) RETURNING id`, runID, data, dag).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// DeletePlans removes plan snapshots. If planID is 0, all plans are deleted.
func (p *PostgresStore) DeletePlans(ctx context.Context, planID int64) (int64, error) {
	if err := p.ensureDB(); err != nil {
		return 0, err
	}
	if planID > 0 {
		if _, err := p.db.ExecContext(ctx, `DELETE FROM plan_metadata WHERE plan_id = $1`, planID); err != nil {
			return 0, err
		}
		res, err := p.db.ExecContext(ctx, `DELETE FROM plans WHERE id = $1`, planID)
		if err != nil {
			return 0, err
		}
		count, _ := res.RowsAffected()
		return count, nil
	}
	if _, err := p.db.ExecContext(ctx, `DELETE FROM plan_metadata`); err != nil {
		return 0, err
	}
	res, err := p.db.ExecContext(ctx, `DELETE FROM plans`)
	if err != nil {
		return 0, err
	}
	count, _ := res.RowsAffected()
	return count, nil
}

// PlanSnapshot returns a stored plan snapshot by id.
func (p *PostgresStore) PlanSnapshot(ctx context.Context, planID int64) (PlanSnapshot, error) {
	if err := p.ensureDB(); err != nil {
		return PlanSnapshot{}, err
	}
	var snap PlanSnapshot
	var planRaw json.RawMessage
	var dagRaw json.RawMessage
	row := p.db.QueryRowContext(ctx, `
		SELECT p.id,
		       p.run_id,
		       p.plan,
		       p.dag,
		       EXISTS (
		         SELECT 1 FROM build_status bs
		         WHERE bs.plan_id = p.id
		           AND bs.status IN ('pending','retry','building')
		       ) AS queued
		FROM plans p
		WHERE p.id = $1
	`, planID)
	if err := row.Scan(&snap.ID, &snap.RunID, &planRaw, &dagRaw, &snap.Queued); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PlanSnapshot{}, ErrNotFound
		}
		return PlanSnapshot{}, err
	}
	if err := json.Unmarshal(planRaw, &snap.Plan); err != nil {
		return PlanSnapshot{}, err
	}
	if len(dagRaw) > 0 {
		snap.DAG = dagRaw
	}
	return snap, nil
}

// LatestPlanSnapshot returns the newest plan snapshot.
func (p *PostgresStore) LatestPlanSnapshot(ctx context.Context) (PlanSnapshot, error) {
	if err := p.ensureDB(); err != nil {
		return PlanSnapshot{}, err
	}
	var snap PlanSnapshot
	var planRaw json.RawMessage
	var dagRaw json.RawMessage
	row := p.db.QueryRowContext(ctx, `
		SELECT p.id,
		       p.run_id,
		       p.plan,
		       p.dag,
		       EXISTS (
		         SELECT 1 FROM build_status bs
		         WHERE bs.plan_id = p.id
		           AND bs.status IN ('pending','retry','building')
		       ) AS queued
		FROM plans p
		ORDER BY created_at DESC
		LIMIT 1`)
	if err := row.Scan(&snap.ID, &snap.RunID, &planRaw, &dagRaw, &snap.Queued); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PlanSnapshot{}, ErrNotFound
		}
		return PlanSnapshot{}, err
	}
	if err := json.Unmarshal(planRaw, &snap.Plan); err != nil {
		return PlanSnapshot{}, err
	}
	if len(dagRaw) > 0 {
		snap.DAG = dagRaw
	}
	return snap, nil
}

// ListPlans returns recent plan summaries.
func (p *PostgresStore) ListPlans(ctx context.Context, limit int) ([]PlanSummary, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := p.db.QueryContext(ctx, `
		SELECT p.id,
		       p.run_id,
		       EXTRACT(EPOCH FROM p.created_at)::BIGINT AS created_at,
		       CASE WHEN jsonb_typeof(p.plan) = 'array' THEN jsonb_array_length(p.plan) ELSE 0 END AS node_count,
		       CASE WHEN jsonb_typeof(p.plan) = 'array'
		            THEN (SELECT COUNT(*) FROM jsonb_array_elements(p.plan) elem WHERE lower(elem->>'action') = 'build')
		            ELSE 0 END AS build_count,
		       EXISTS (
		         SELECT 1 FROM build_status bs
		         WHERE bs.plan_id = p.id
		           AND bs.status IN ('pending','retry','building')
		       ) AS queued
		FROM plans p
		ORDER BY p.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanSummary
	for rows.Next() {
		var entry PlanSummary
		if err := rows.Scan(&entry.ID, &entry.RunID, &entry.CreatedAt, &entry.NodeCount, &entry.BuildCount, &entry.Queued); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// QueueBuildsFromPlan seeds build_status rows for build nodes in a plan.
func (p *PostgresStore) QueueBuildsFromPlan(ctx context.Context, runID string, planID int64, nodes []PlanNode) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt := `
		INSERT INTO build_status (package, version, python_tag, platform_tag, status, attempts, run_id, plan_id, backoff_until, last_error, recipes)
		VALUES ($1,$2,$3,$4,'pending',0,$5,$6,NULL,'',$7)
		ON CONFLICT (package, version) DO UPDATE
		SET python_tag = EXCLUDED.python_tag,
		    platform_tag = EXCLUDED.platform_tag,
		    run_id = EXCLUDED.run_id,
		    plan_id = EXCLUDED.plan_id,
		    status = 'pending',
		    attempts = 0,
		    backoff_until = NULL,
		    last_error = '',
		    recipes = COALESCE(EXCLUDED.recipes, build_status.recipes)
	`
	for _, n := range nodes {
		if strings.ToLower(n.Action) != "build" || n.Name == "" || n.Version == "" {
			continue
		}
		recipes := planRecipeNames(n.Recipes)
		var recipesRaw any
		if recipes != nil {
			if data, err := json.Marshal(recipes); err == nil {
				recipesRaw = data
			}
		}
		if _, err := tx.ExecContext(ctx, stmt, n.Name, n.Version, n.PythonTag, n.PlatformTag, runID, planID, recipesRaw); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LeaseBuilds returns ready builds and marks them building with attempt increment.
func (p *PostgresStore) LeaseBuilds(ctx context.Context, max int) ([]BuildStatus, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	if max <= 0 {
		max = 1
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		WITH cte AS (
			SELECT id
			FROM build_status
			WHERE status IN ('pending','retry')
			  AND (backoff_until IS NULL OR backoff_until <= NOW())
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE build_status b
		SET status = 'building',
		    attempts = b.attempts + 1,
		    updated_at = NOW()
		FROM cte
		WHERE b.id = cte.id
		RETURNING b.id, b.package, b.version, b.python_tag, b.platform_tag, b.status, b.attempts, COALESCE(b.last_error,''), b.run_id, b.plan_id, COALESCE(extract(epoch from b.backoff_until),0)::bigint, extract(epoch from b.created_at)::bigint, extract(epoch from b.updated_at)::bigint, COALESCE(b.recipes, '[]'::jsonb), COALESCE(b.hint_ids, '{}'::text[])
	`, max)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BuildStatus
	for rows.Next() {
		var bs BuildStatus
		var recipes json.RawMessage
		var hints pq.StringArray
		if err := rows.Scan(&bs.ID, &bs.Package, &bs.Version, &bs.PythonTag, &bs.PlatformTag, &bs.Status, &bs.Attempts, &bs.LastError, &bs.RunID, &bs.PlanID, &bs.BackoffUntil, &bs.CreatedAt, &bs.UpdatedAt, &recipes, &hints); err != nil {
			return nil, err
		}
		if len(recipes) > 0 {
			_ = json.Unmarshal(recipes, &bs.Recipes)
		}
		if len(hints) > 0 {
			bs.HintIDs = hints
		}
		out = append(out, bs)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// Helpers for pq string arrays without importing driver types in interface.
type pqStringArrayParam []string

func (a pqStringArrayParam) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	return pq.Array([]string(a)).Value()
}

func planRecipeNames(recipes []PlanRecipe) []string {
	if len(recipes) == 0 {
		return nil
	}
	out := make([]string, 0, len(recipes))
	for _, r := range recipes {
		if r.Name == "" {
			continue
		}
		out = append(out, r.Name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func pqStringArray(dst *[]sql.NullString) any {
	return pq.Array(dst)
}
