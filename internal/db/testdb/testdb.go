// Package testdb opens an in-memory SQLite database with all migrations
// applied, for tests.
package testdb

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/shahab5191/openshin/internal/db"
)

// migrationsDir is the repo's migrations directory, derived from this file's
// location so tests work regardless of the process working directory.
var migrationsDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")
}()

// Open returns an in-memory database with the full schema applied.
func Open(t *testing.T) *sql.DB {
	t.Helper()

	d, err := db.Connect(":memory:")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := goose.SetDialect("sqlite"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(d, migrationsDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	t.Cleanup(func() { d.Close() })
	return d
}
