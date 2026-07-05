package sql_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/spf13/afero"

	"github.com/infogulch/xtemplate"
	provsql "github.com/infogulch/xtemplate/providers/dotsql"
)

// buildInstance builds an Instance from an in-memory template fs and any extra
// options, failing the test if construction fails.
func buildInstance(t *testing.T, files map[string]string, opts ...xtemplate.Option) *xtemplate.Instance {
	t.Helper()
	fs := afero.NewMemMapFs()
	for name, content := range files {
		if err := afero.WriteFile(fs, name, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %q to mem fs: %v", name, err)
		}
	}
	cfg := xtemplate.New()
	allOpts := append([]xtemplate.Option{xtemplate.WithTemplateFS(fs)}, opts...)
	inst, _, _, err := cfg.Instance(allOpts...)
	if err != nil {
		t.Fatalf("failed to build instance: %v", err)
	}
	return inst
}

func doRequest(inst *xtemplate.Instance, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, target, nil)
	inst.ServeHTTP(w, r)
	return w
}

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
	t.Cleanup(func() { _ = db.Close() })

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

// TestDotSql_Query exercises the read path: a template queries the seeded DB and
// renders a scalar result.
func TestDotSql_Query(t *testing.T) {
	db := newTestDB(t)
	inst := buildInstance(t,
		map[string]string{
			"count.html": `{{.DB.QueryVal "SELECT count(*) FROM users"}}`,
		},
		provsql.WithSql("DB", db, nil),
	)

	w := doRequest(inst, http.MethodGet, "/count")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "2" {
		t.Errorf("count body = %q, want %q", got, "2")
	}
}

// TestDotSql_QueryRowsStream renders every row yielded by the QueryRows iterator.
func TestDotSql_QueryRowsStream(t *testing.T) {
	db := newTestDB(t)
	inst := buildInstance(t,
		map[string]string{
			"names.html": `{{range .DB.QueryRows "SELECT name FROM users ORDER BY name"}}{{.name}},{{end}}`,
		},
		provsql.WithSql("DB", db, nil),
	)

	w := doRequest(inst, http.MethodGet, "/names")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != "alice,bob," {
		t.Errorf("body = %q, want %q", got, "alice,bob,")
	}
}

// TestDotSql_QueryRowsIterationError verifies that an error encountered partway
// through iterating the QueryRows result aborts template execution cleanly.
func TestDotSql_QueryRowsIterationError(t *testing.T) {
	db := newTestDB(t)
	inst := buildInstance(t,
		map[string]string{
			"boom.html": `{{range .DB.QueryRows "SELECT 1 AS n UNION ALL SELECT abs(-9223372036854775808)"}}{{.n}}{{end}}`,
		},
		provsql.WithSql("DB", db, nil),
	)

	w := doRequest(inst, http.MethodGet, "/boom")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// TestDotSql_QueryRowWrongCount verifies QueryRow rejects results that don't
// contain exactly one row.
func TestDotSql_QueryRowWrongCount(t *testing.T) {
	db := newTestDB(t)
	for _, tc := range []struct {
		name  string
		query string
	}{
		{"zero rows", "SELECT name FROM users WHERE name='nobody'"},
		{"multiple rows", "SELECT name FROM users"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inst := buildInstance(t,
				map[string]string{
					"row.html": `{{$r := .DB.QueryRow "` + tc.query + `"}}{{$r.name}}`,
				},
				provsql.WithSql("DB", db, nil),
			)

			w := doRequest(inst, http.MethodGet, "/row")
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
			}
		})
	}
}

// TestDotSql_AutoCommit verifies the implicit transaction is committed when the
// template completes without error, persisting writes.
func TestDotSql_AutoCommit(t *testing.T) {
	db := newTestDB(t)
	inst := buildInstance(t,
		map[string]string{
			"insert.html": `{{$_ := .DB.Exec "INSERT INTO users (name) VALUES ('carol')"}}ok`,
		},
		provsql.WithSql("DB", db, nil),
	)

	w := doRequest(inst, http.MethodGet, "/insert")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if n := countUsers(t, db); n != 3 {
		t.Errorf("row count after commit = %d, want 3", n)
	}
}

// TestDotSql_RollbackOnError verifies the implicit transaction is rolled back
// when the template fails after a write.
func TestDotSql_RollbackOnError(t *testing.T) {
	db := newTestDB(t)
	inst := buildInstance(t,
		map[string]string{
			"fail.html": `{{$_ := .DB.Exec "INSERT INTO users (name) VALUES ('dave')"}}{{failf "boom"}}`,
		},
		provsql.WithSql("DB", db, nil),
	)

	w := doRequest(inst, http.MethodGet, "/fail")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if n := countUsers(t, db); n != 2 {
		t.Errorf("row count after rollback = %d, want 2 (insert should have been rolled back)", n)
	}
}
