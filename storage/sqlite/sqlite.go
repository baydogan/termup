package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/storage"

	_ "modernc.org/sqlite"
)

// historyLimit is how many recent results History returns per monitor.
const historyLimit = 60

var _ storage.Store = (*Store)(nil)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite is single-writer; serialize through one connection to avoid
	// "database is locked" under the concurrent probe pool.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS monitors (
	name TEXT PRIMARY KEY,
	url  TEXT NOT NULL,
	pos  INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS results (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	monitor_name TEXT NOT NULL,
	status_code  INTEGER NOT NULL,
	latency_ms   INTEGER NOT NULL,
	err          TEXT NOT NULL DEFAULT '',
	checked_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_results_monitor ON results(monitor_name, id);`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

func (s *Store) List() ([]monitor.Monitor, error) {
	rows, err := s.db.Query(`SELECT name, url FROM monitors ORDER BY pos`)
	if err != nil {
		return nil, fmt.Errorf("list monitors: %w", err)
	}
	defer rows.Close()

	var out []monitor.Monitor
	for rows.Next() {
		var m monitor.Monitor
		if err := rows.Scan(&m.Name, &m.URL); err != nil {
			return nil, fmt.Errorf("scan monitor: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GetStatus(name string) (monitor.Status, error) {
	row := s.db.QueryRow(
		`SELECT status_code, latency_ms, err, checked_at FROM results
		 WHERE monitor_name = ? ORDER BY id DESC LIMIT 1`, name)
	r, err := scanResult(row, name)
	if errors.Is(err, sql.ErrNoRows) {
		return monitor.Status{}, storage.ErrNotFound
	}
	if err != nil {
		return monitor.Status{}, fmt.Errorf("get status: %w", err)
	}
	return monitor.Status{State: r.State()}, nil
}

func (s *Store) History(name string) ([]monitor.Result, error) {
	// take the last N by id, then return chronological (oldest -> newest)
	rows, err := s.db.Query(
		`SELECT status_code, latency_ms, err, checked_at FROM (
			SELECT id, status_code, latency_ms, err, checked_at FROM results
			WHERE monitor_name = ? ORDER BY id DESC LIMIT ?
		 ) ORDER BY id ASC`, name, historyLimit)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	defer rows.Close()

	var out []monitor.Result
	for rows.Next() {
		r, err := scanResult(rows, name)
		if err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Save(r monitor.Result) error {
	var errStr string
	if r.Err != nil {
		errStr = r.Err.Error()
	}
	_, err := s.db.Exec(
		`INSERT INTO results (monitor_name, status_code, latency_ms, err, checked_at)
		 VALUES (?, ?, ?, ?, ?)`,
		r.MonitorName, r.StatusCode, r.Latency.Milliseconds(), errStr, r.CheckedAt.Unix())
	if err != nil {
		return fmt.Errorf("save result: %w", err)
	}
	return nil
}

func (s *Store) Sync(monitors []monitor.Monitor) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("sync begin: %w", err)
	}
	defer tx.Rollback()

	for i, m := range monitors {
		if _, err := tx.Exec(
			`INSERT INTO monitors (name, url, pos) VALUES (?, ?, ?)
			 ON CONFLICT(name) DO UPDATE SET url = excluded.url, pos = excluded.pos`,
			m.Name, m.URL, i); err != nil {
			return fmt.Errorf("sync upsert %q: %w", m.Name, err)
		}
	}

	// prune monitors (and their results) that are no longer in config
	if len(monitors) == 0 {
		if _, err := tx.Exec(`DELETE FROM monitors`); err != nil {
			return fmt.Errorf("sync prune monitors: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM results`); err != nil {
			return fmt.Errorf("sync prune results: %w", err)
		}
		return tx.Commit()
	}

	names := make([]any, len(monitors))
	ph := make([]string, len(monitors))
	for i, m := range monitors {
		names[i], ph[i] = m.Name, "?"
	}
	in := strings.Join(ph, ",")
	if _, err := tx.Exec(`DELETE FROM monitors WHERE name NOT IN (`+in+`)`, names...); err != nil {
		return fmt.Errorf("sync prune monitors: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM results WHERE monitor_name NOT IN (`+in+`)`, names...); err != nil {
		return fmt.Errorf("sync prune results: %w", err)
	}
	return tx.Commit()
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface{ Scan(...any) error }

func scanResult(sc scanner, name string) (monitor.Result, error) {
	var (
		code    int
		latMs   int64
		errStr  string
		checked int64
	)
	if err := sc.Scan(&code, &latMs, &errStr, &checked); err != nil {
		return monitor.Result{}, err
	}
	r := monitor.Result{
		MonitorName: name,
		StatusCode:  code,
		Latency:     time.Duration(latMs) * time.Millisecond,
		CheckedAt:   time.Unix(checked, 0),
	}
	if errStr != "" {
		r.Err = errors.New(errStr) // identity lost, but State() only checks Err != nil
	}
	return r, nil
}
