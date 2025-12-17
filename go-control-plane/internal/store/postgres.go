package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

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

CREATE TABLE IF NOT EXISTS plans (
    id         BIGSERIAL PRIMARY KEY,
    run_id     TEXT,
    plan       JSONB NOT NULL,
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
    last_error    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_build_status_pkg ON build_status(package, version);
CREATE INDEX IF NOT EXISTS idx_build_status_status ON build_status(status);

CREATE TABLE IF NOT EXISTS pending_inputs (
    id          BIGSERIAL PRIMARY KEY,
    filename    TEXT NOT NULL,
    digest      TEXT,
    size_bytes  BIGINT,
    status      TEXT NOT NULL DEFAULT 'pending',
    error       TEXT,
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

// AddPendingInput inserts a new pending input.
func (p *PostgresStore) AddPendingInput(ctx context.Context, pi PendingInput) (int64, error) {
	if err := p.ensureDB(); err != nil {
		return 0, err
	}
	var id int64
	err := p.db.QueryRowContext(ctx, `
		INSERT INTO pending_inputs (filename, digest, size_bytes, status)
		VALUES ($1,$2,$3,$4) RETURNING id
	`, pi.Filename, pi.Digest, pi.SizeBytes, pi.Status).Scan(&id)
	return id, err
}

// ListPendingInputs fetches pending inputs.
func (p *PostgresStore) ListPendingInputs(ctx context.Context, status string) ([]PendingInput, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	q := `SELECT id, filename, digest, size_bytes, status, COALESCE(error,''), created_at, updated_at FROM pending_inputs`
	args := []any{}
	if status != "" {
		q += ` WHERE status = $1`
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
		if err := rows.Scan(&pi.ID, &pi.Filename, &pi.Digest, &pi.SizeBytes, &pi.Status, &pi.Error, &pi.CreatedAt, &pi.UpdatedAt); err != nil {
			return nil, err
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
	_, err := p.db.ExecContext(ctx, `
		UPDATE pending_inputs
		SET status = $1, error = $2, updated_at = NOW()
		WHERE id = $3
	`, status, errMsg, id)
	return err
}

// ListBuilds returns build status rows filtered by status if provided.
func (p *PostgresStore) ListBuilds(ctx context.Context, status string, limit int) ([]BuildStatus, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	q := `SELECT id, package, version, python_tag, platform_tag, status, attempts, COALESCE(last_error,''), extract(epoch from (NOW() - created_at))::bigint as age, extract(epoch from created_at)::bigint, extract(epoch from updated_at)::bigint FROM build_status`
	args := []any{}
	if status != "" {
		q += ` WHERE status = $1`
		args = append(args, status)
	}
	q += ` ORDER BY created_at ASC`
	if limit > 0 {
		q += ` LIMIT $2`
		if status == "" {
			args = append(args, limit)
		}
	}
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BuildStatus
	for rows.Next() {
		var bs BuildStatus
		if err := rows.Scan(&bs.ID, &bs.Package, &bs.Version, &bs.PythonTag, &bs.PlatformTag, &bs.Status, &bs.Attempts, &bs.LastError, &bs.OldestAgeSec, &bs.CreatedAt, &bs.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, bs)
	}
	return out, rows.Err()
}

// UpdateBuildStatus upserts build status by package/version.
func (p *PostgresStore) UpdateBuildStatus(ctx context.Context, pkg, version, status, errMsg string, attempts int) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	if attempts < 0 {
		attempts = 0
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO build_status (package, version, status, last_error, attempts)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (package, version) DO UPDATE
		SET status = EXCLUDED.status,
		    last_error = EXCLUDED.last_error,
		    attempts = EXCLUDED.attempts,
		    updated_at = NOW()
	`, pkg, version, status, errMsg, attempts)
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
	rows, err := p.db.QueryContext(ctx, `SELECT id,pattern,recipes,note FROM hints`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Hint
	for rows.Next() {
		var h Hint
		var recipes json.RawMessage
		if err := rows.Scan(&h.ID, &h.Pattern, &recipes, &h.Note); err != nil {
			return nil, err
		}
		if len(recipes) > 0 {
			_ = json.Unmarshal(recipes, &h.Recipes)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (p *PostgresStore) GetHint(ctx context.Context, id string) (Hint, error) {
	if err := p.ensureDB(); err != nil {
		return Hint{}, err
	}
	var h Hint
	var recipes json.RawMessage
	err := p.db.QueryRowContext(ctx, `SELECT id,pattern,recipes,note FROM hints WHERE id=$1`, id).Scan(&h.ID, &h.Pattern, &recipes, &h.Note)
	if err != nil {
		return Hint{}, err
	}
	if len(recipes) > 0 {
		_ = json.Unmarshal(recipes, &h.Recipes)
	}
	return h, nil
}

func (p *PostgresStore) PutHint(ctx context.Context, hint Hint) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	recipes, _ := json.Marshal(hint.Recipes)
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO hints (id,pattern,recipes,note)
	    VALUES ($1,$2,$3,$4)
	    ON CONFLICT (id) DO UPDATE SET pattern=EXCLUDED.pattern, recipes=EXCLUDED.recipes, note=EXCLUDED.note`,
		hint.ID, hint.Pattern, recipes, hint.Note)
	return err
}

func (p *PostgresStore) DeleteHint(ctx context.Context, id string) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	_, err := p.db.ExecContext(ctx, `DELETE FROM hints WHERE id=$1`, id)
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

func (p *PostgresStore) SavePlan(ctx context.Context, runID string, nodes []PlanNode) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	data, err := json.Marshal(nodes)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, `INSERT INTO plans (run_id, plan) VALUES ($1, $2)`, runID, data)
	return err
}

// Helpers for pq string arrays without importing driver types in interface.
type pqStringArrayParam []string

func (a pqStringArrayParam) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	return pq.Array([]string(a)).Value()
}

func pqStringArray(dst *[]sql.NullString) any {
	return pq.Array(dst)
}
