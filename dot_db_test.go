package xtemplate

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
)

// newTestDB returns an in-memory sqlite DB seeded with a users table containing
// two rows. MaxOpenConns(1) keeps the single connection (and therefore the
// in-memory database) alive across the test's queries.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE users (name TEXT)`); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (name) VALUES ('alice'), ('bob')`); err != nil {
		t.Fatalf("failed to seed rows: %v", err)
	}
	return db
}

func countUsers(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM users`).Scan(&n); err != nil {
		t.Fatalf("failed to count users: %v", err)
	}
	return n
}

// TestDotDB_Query exercises the read path: a template queries the seeded DB and
// renders a scalar result.
func TestDotDB_Query(t *testing.T) {
	db := newTestDB(t)
	inst := buildInstance(t,
		map[string]string{
			"count.html": `{{.DB.QueryVal "SELECT count(*) FROM users"}}`,
		},
		WithDB("DB", db, nil),
	)

	w := doRequest(inst, http.MethodGet, "/count")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "2" {
		t.Errorf("count body = %q, want %q", got, "2")
	}
}

// TestDotDB_AutoCommit verifies the implicit transaction is committed when the
// template completes without error, persisting writes.
func TestDotDB_AutoCommit(t *testing.T) {
	db := newTestDB(t)
	inst := buildInstance(t,
		map[string]string{
			// $_ swallows the (sql.Result, error) return; a non-nil error would
			// abort the template.
			"insert.html": `{{$_ := .DB.Exec "INSERT INTO users (name) VALUES ('carol')"}}ok`,
		},
		WithDB("DB", db, nil),
	)

	w := doRequest(inst, http.MethodGet, "/insert")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if n := countUsers(t, db); n != 3 {
		t.Errorf("row count after commit = %d, want 3", n)
	}
}

// TestDotDB_RollbackOnError verifies the implicit transaction is rolled back
// when the template fails after a write, leaving the DB unchanged.
func TestDotDB_RollbackOnError(t *testing.T) {
	db := newTestDB(t)
	inst := buildInstance(t,
		map[string]string{
			"fail.html": `{{$_ := .DB.Exec "INSERT INTO users (name) VALUES ('dave')"}}{{failf "boom"}}`,
		},
		WithDB("DB", db, nil),
	)

	w := doRequest(inst, http.MethodGet, "/fail")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if n := countUsers(t, db); n != 2 {
		t.Errorf("row count after rollback = %d, want 2 (insert should have been rolled back)", n)
	}
}
