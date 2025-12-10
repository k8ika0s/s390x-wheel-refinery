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

// NewPostgres creates a new store with an existing *sql.DB.
func NewPostgres(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// Ping validates DB connectivity; optional for health checks.
func (p *PostgresStore) Ping(ctx context.Context) error {
	if p.db == nil {
		return fmt.Errorf("db not configured")
	}
	return p.db.PingContext(ctx)
}

func (p *PostgresStore) Recent(ctx context.Context, limit, offset int, pkg, status string) ([]Event, error) {
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
	metaBytes, _ := json.Marshal(evt.Metadata)
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO events (run_id,name,version,python_tag,platform_tag,status,detail,metadata,matched_hint_ids,timestamp)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,TO_TIMESTAMP($10))`,
		evt.RunID, evt.Name, evt.Version, evt.PythonTag, evt.PlatformTag, evt.Status, evt.Detail, metaBytes, pq.Array(evt.MatchedHintIDs), evt.Timestamp)
	return err
}

func (p *PostgresStore) Summary(ctx context.Context, failureLimit int) (Summary, error) {
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
	recipes, _ := json.Marshal(hint.Recipes)
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO hints (id,pattern,recipes,note)
	    VALUES ($1,$2,$3,$4)
	    ON CONFLICT (id) DO UPDATE SET pattern=EXCLUDED.pattern, recipes=EXCLUDED.recipes, note=EXCLUDED.note`,
		hint.ID, hint.Pattern, recipes, hint.Note)
	return err
}

func (p *PostgresStore) DeleteHint(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM hints WHERE id=$1`, id)
	return err
}

func (p *PostgresStore) GetLog(ctx context.Context, name, version string) (LogEntry, error) {
	var le LogEntry
	err := p.db.QueryRowContext(ctx, `
	    SELECT name,version,content,extract(epoch from timestamp)::bigint FROM logs
	    WHERE name=$1 AND version=$2 ORDER BY timestamp DESC LIMIT 1`, name, version).Scan(&le.Name, &le.Version, &le.Content, &le.Timestamp)
	return le, err
}

func (p *PostgresStore) PutLog(ctx context.Context, entry LogEntry) error {
	_, err := p.db.ExecContext(ctx, `INSERT INTO logs (name,version,content,timestamp) VALUES ($1,$2,$3,TO_TIMESTAMP($4))`, entry.Name, entry.Version, entry.Content, entry.Timestamp)
	return err
}

func (p *PostgresStore) SearchLogs(ctx context.Context, q string, limit int) ([]LogEntry, error) {
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
	if limit <= 0 {
		limit = 200
	}
	rows, err := p.db.QueryContext(ctx, `SELECT name,version,wheel,python_tag,platform_tag,status,extract(epoch from created_at)::bigint FROM manifests ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManifestEntry
	for rows.Next() {
		var m ManifestEntry
		if err := rows.Scan(&m.Name, &m.Version, &m.Wheel, &m.PythonTag, &m.PlatformTag, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (p *PostgresStore) SaveManifest(ctx context.Context, entries []ManifestEntry) error {
	if len(entries) == 0 {
		return nil
	}
	for _, m := range entries {
		if m.CreatedAt == 0 {
			m.CreatedAt = time.Now().Unix()
		}
		_, err := p.db.ExecContext(ctx, `INSERT INTO manifests (name,version,wheel,python_tag,platform_tag,status,created_at)
			VALUES ($1,$2,$3,$4,$5,$6,TO_TIMESTAMP($7))`, m.Name, m.Version, m.Wheel, m.PythonTag, m.PlatformTag, m.Status, m.CreatedAt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PostgresStore) Artifacts(ctx context.Context, limit int) ([]Artifact, error) {
	manifests, err := p.Manifest(ctx, limit)
	if err != nil {
		return nil, err
	}
	var out []Artifact
	for _, m := range manifests {
		out = append(out, Artifact{Name: m.Name, Version: m.Version, Path: m.Wheel, URL: ""})
	}
	return out, nil
}

func (p *PostgresStore) Plan(ctx context.Context) ([]PlanNode, error) {
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
