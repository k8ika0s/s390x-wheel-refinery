package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"

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

func (p *PostgresStore) Recent(ctx context.Context, limit int, pkg, status string) ([]Event, error) {
	q := `SELECT run_id,name,version,python_tag,platform_tag,status,detail,metadata,matched_hint_ids,extract(epoch from timestamp)::bigint
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
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var metaRaw json.RawMessage
		var matched []sql.NullString
		if err := rows.Scan(&e.RunID, &e.Name, &e.Version, &e.PythonTag, &e.PlatformTag, &e.Status, &e.Detail, &metaRaw, pqStringArray(&matched), &e.Timestamp); err != nil {
			return nil, err
		}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &e.Metadata)
		}
		for _, v := range matched {
			if v.Valid {
				e.MatchedHintIDs = append(e.MatchedHintIDs, v.String)
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *PostgresStore) History(ctx context.Context, filter HistoryFilter) ([]Event, error) {
	q := `SELECT run_id,name,version,python_tag,platform_tag,status,detail,metadata,matched_hint_ids,extract(epoch from timestamp)::bigint
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
		var matched []sql.NullString
		if err := rows.Scan(&e.RunID, &e.Name, &e.Version, &e.PythonTag, &e.PlatformTag, &e.Status, &e.Detail, &metaRaw, pqStringArray(&matched), &e.Timestamp); err != nil {
			return nil, err
		}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &e.Metadata)
		}
		for _, v := range matched {
			if v.Valid {
				e.MatchedHintIDs = append(e.MatchedHintIDs, v.String)
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *PostgresStore) RecordEvent(ctx context.Context, evt Event) error {
	metaBytes, _ := json.Marshal(evt.Metadata)
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO events (run_id,name,version,python_tag,platform_tag,status,detail,metadata,matched_hint_ids,timestamp)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,TO_TIMESTAMP($10))`,
		evt.RunID, evt.Name, evt.Version, evt.PythonTag, evt.PlatformTag, evt.Status, evt.Detail, metaBytes, pqStringArrayParam(evt.MatchedHintIDs), evt.Timestamp)
	return err
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
