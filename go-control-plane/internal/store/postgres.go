package store

import (
	"context"
	"database/sql"
	"errors"
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
	return nil, errors.New("not implemented")
}

func (p *PostgresStore) History(ctx context.Context, filter HistoryFilter) ([]Event, error) {
	return nil, errors.New("not implemented")
}

func (p *PostgresStore) RecordEvent(ctx context.Context, evt Event) error {
	return errors.New("not implemented")
}

func (p *PostgresStore) ListHints(ctx context.Context) ([]Hint, error) {
	return nil, errors.New("not implemented")
}

func (p *PostgresStore) GetHint(ctx context.Context, id string) (Hint, error) {
	return Hint{}, errors.New("not implemented")
}

func (p *PostgresStore) PutHint(ctx context.Context, hint Hint) error {
	return errors.New("not implemented")
}

func (p *PostgresStore) DeleteHint(ctx context.Context, id string) error {
	return errors.New("not implemented")
}

func (p *PostgresStore) GetLog(ctx context.Context, name, version string) (LogEntry, error) {
	return LogEntry{}, errors.New("not implemented")
}

func (p *PostgresStore) SearchLogs(ctx context.Context, q string, limit int) ([]LogEntry, error) {
	return nil, errors.New("not implemented")
}
