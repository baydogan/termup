package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/baydogan/termup/monitor"

	_ "modernc.org/sqlite"
)

// historyLimit is how many recent results History returns per monitor.
const historyLimit = 60

// resultCols is the column list scanned by scanResult; kept in one place so the
// queries and the scan stay in sync.
const resultCols = "status_code, latency_ms, err, checked_at, cert_expiry, dns_ms, connect_ms, tls_ms, ttfb_ms"

var _ Store = (*SQLite)(nil)

type SQLite struct {
	db *sql.DB
}

func NewSQLite(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite is single-writer; serialize through one connection to avoid
	// "database is locked" under the concurrent probe pool.
	db.SetMaxOpenConns(1)

	s := &SQLite{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

func (s *SQLite) migrate() error {
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
	checked_at   INTEGER NOT NULL,
	cert_expiry  INTEGER NOT NULL DEFAULT 0,
	dns_ms       INTEGER NOT NULL DEFAULT 0,
	connect_ms   INTEGER NOT NULL DEFAULT 0,
	tls_ms       INTEGER NOT NULL DEFAULT 0,
	ttfb_ms      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_results_monitor ON results(monitor_name, id);`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// Backfill columns for databases created before they existed; a
	// duplicate-column error just means the column is already there.
	for _, col := range []string{
		"cert_expiry INTEGER NOT NULL DEFAULT 0",
		"dns_ms INTEGER NOT NULL DEFAULT 0",
		"connect_ms INTEGER NOT NULL DEFAULT 0",
		"tls_ms INTEGER NOT NULL DEFAULT 0",
		"ttfb_ms INTEGER NOT NULL DEFAULT 0",
	} {
		if _, err := s.db.Exec(`ALTER TABLE results ADD COLUMN ` + col); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("migrate %s: %w", col, err)
		}
	}
	return nil
}

func (s *SQLite) List() ([]monitor.Monitor, error) {
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

func (s *SQLite) History(name string) ([]monitor.Result, error) {
	// take the last N by id, then return chronological (oldest -> newest)
	rows, err := s.db.Query(
		`SELECT `+resultCols+` FROM (
			SELECT id, `+resultCols+` FROM results
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

func (s *SQLite) Save(r monitor.Result) error {
	var errStr string
	if r.Err != nil {
		errStr = r.Err.Error()
	}
	var certExpiry int64
	if !r.CertExpiry.IsZero() {
		certExpiry = r.CertExpiry.Unix()
	}
	_, err := s.db.Exec(
		`INSERT INTO results
		   (monitor_name, status_code, latency_ms, err, checked_at, cert_expiry, dns_ms, connect_ms, tls_ms, ttfb_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.MonitorName, r.StatusCode, r.Latency.Milliseconds(), errStr, r.CheckedAt.Unix(), certExpiry,
		r.DNS.Milliseconds(), r.Connect.Milliseconds(), r.TLS.Milliseconds(), r.TTFB.Milliseconds())
	if err != nil {
		return fmt.Errorf("save result: %w", err)
	}
	return nil
}

func (s *SQLite) Prune(before time.Time) error {
	if _, err := s.db.Exec(`DELETE FROM results WHERE checked_at < ?`, before.Unix()); err != nil {
		return fmt.Errorf("prune results: %w", err)
	}
	return nil
}

func (s *SQLite) Sync(monitors []monitor.Monitor) error {
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
		code                            int
		latMs                           int64
		errStr                          string
		checked                         int64
		certUnix                        int64
		dnsMs, connectMs, tlsMs, ttfbMs int64
	)
	if err := sc.Scan(&code, &latMs, &errStr, &checked, &certUnix, &dnsMs, &connectMs, &tlsMs, &ttfbMs); err != nil {
		return monitor.Result{}, err
	}
	r := monitor.Result{
		MonitorName: name,
		StatusCode:  code,
		Latency:     time.Duration(latMs) * time.Millisecond,
		CheckedAt:   time.Unix(checked, 0),
		DNS:         time.Duration(dnsMs) * time.Millisecond,
		Connect:     time.Duration(connectMs) * time.Millisecond,
		TLS:         time.Duration(tlsMs) * time.Millisecond,
		TTFB:        time.Duration(ttfbMs) * time.Millisecond,
	}
	if certUnix != 0 {
		r.CertExpiry = time.Unix(certUnix, 0)
	}
	if errStr != "" {
		r.Err = errors.New(errStr) // identity lost, but State() only checks Err != nil
	}
	return r, nil
}
