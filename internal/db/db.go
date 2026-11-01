// Package db owns the SQLite connection and its pragmas. openshin is a local
// single-writer daemon: one process owns the file and every write, so the
// connection is intentionally simple — WAL for concurrent readers, a busy
// timeout for occasional contention, and foreign keys always on.
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, CGO-free)
)

// Connect opens (creating if needed) the SQLite database at path and applies
// the pragmas openshin relies on. An empty path is treated as ":memory:" for
// tests and short-lived tooling.
func Connect(path string) (*sql.DB, error) {
	if path == "" {
		path = ":memory:"
	}

	d, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// A single local daemon is the only writer; MaxOpenConns(1) serialises
	// writes and sidesteps "database is locked" races for the common case.
	// Reads from agent tools are cheap enough that this is not a bottleneck at
	// local scale; revisit before the fleet phase.
	d.SetMaxOpenConns(1)

	// foreign_keys applies per-connection and is OFF by default in SQLite.
	if _, err := d.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		d.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := d.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		d.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	// WAL lets readers (agent tools) proceed during a write. Skipped for
	// :memory:, where journal_mode is a no-op.
	if path != ":memory:" {
		if _, err := d.Exec("PRAGMA journal_mode = WAL;"); err != nil {
			d.Close()
			return nil, fmt.Errorf("enable wal: %w", err)
		}
	}

	if err := d.Ping(); err != nil {
		d.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	return d, nil
}
