package db

import "testing"

func TestConnectInMemory(t *testing.T) {
	d, err := Connect(":memory:")
	if err != nil {
		t.Fatalf("connect in-memory: %v", err)
	}
	defer d.Close()

	if err := d.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}

	if _, err := d.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
}

func TestConnectEmptyPathFallsBackToMemory(t *testing.T) {
	d, err := Connect("")
	if err != nil {
		t.Fatalf("connect empty path: %v", err)
	}
	defer d.Close()

	if err := d.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}
