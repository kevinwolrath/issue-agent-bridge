// Package store persists jobs and owns atomic queue transitions.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/kevinwolrath/issue-agent-bridge/internal/job"
	_ "modernc.org/sqlite"
)

//go:embed migrations/001_jobs.sql
var migration string

type Store struct{ db *sql.DB }

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.initialize(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) initialize(ctx context.Context) error {
	for _, statement := range []string{"PRAGMA busy_timeout = 5000", "PRAGMA journal_mode = WAL", "PRAGMA synchronous = FULL"} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return fmt.Errorf("unsupported database version %d", version)
	}
	if version == 0 {
		if _, err := tx.ExecContext(ctx, migration); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Close() error                   { return s.db.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) Enqueue(ctx context.Context, in job.Input) (bool, error) {
	if in.Provider == "" || in.DeliveryID == "" || in.Repository == "" || in.IssueNumber <= 0 {
		return false, errors.New("incomplete job input")
	}
	now := time.Now().UTC().UnixMilli()
	r, err := s.db.ExecContext(ctx, `INSERT INTO jobs
		(provider, delivery_id, repository, issue_number, issue_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(provider, delivery_id) DO NOTHING`,
		in.Provider, in.DeliveryID, in.Repository, in.IssueNumber, in.IssueURL, now, now)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}

const columns = "id, provider, delivery_id, repository, issue_number, issue_url, status, attempts, last_error, created_at, updated_at"

func scan(row *sql.Row) (*job.Job, error) {
	var j job.Job
	var created, updated int64
	err := row.Scan(&j.ID, &j.Provider, &j.DeliveryID, &j.Repository, &j.IssueNumber, &j.IssueURL, &j.Status, &j.Attempts, &j.LastError, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j.CreatedAt, j.UpdatedAt = time.UnixMilli(created).UTC(), time.UnixMilli(updated).UTC()
	return &j, nil
}

// Claim uses one atomic statement so concurrent workers cannot receive the same job.
func (s *Store) Claim(ctx context.Context) (*job.Job, error) {
	return scan(s.db.QueryRowContext(ctx, `UPDATE jobs SET status='running', attempts=attempts+1, updated_at=?
		WHERE id=(SELECT id FROM jobs WHERE status='queued' ORDER BY id LIMIT 1) AND status='queued'
		RETURNING `+columns, time.Now().UTC().UnixMilli()))
}

func (s *Store) Finish(ctx context.Context, id int64, executionError error) error {
	status, detail := "succeeded", ""
	if executionError != nil {
		status, detail = "failed", executionError.Error()
	}
	r, err := s.db.ExecContext(ctx, "UPDATE jobs SET status=?, last_error=?, updated_at=? WHERE id=? AND status='running'", status, detail, time.Now().UTC().UnixMilli(), id)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err == nil && n != 1 {
		return errors.New("job is missing or not running")
	}
	return err
}

// Recover marks interrupted work failed, never replaying a potentially completed
// external side effect. Run once at startup with a single service replica.
func (s *Store) Recover(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "UPDATE jobs SET status='failed', last_error='service stopped before completion', updated_at=? WHERE status='running'", time.Now().UTC().UnixMilli())
	return err
}

func (s *Store) Get(ctx context.Context, id int64) (*job.Job, error) {
	return scan(s.db.QueryRowContext(ctx, "SELECT "+columns+" FROM jobs WHERE id=?", id))
}
