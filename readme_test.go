package xtemplate

// These tests extract the runnable template examples from README.md so that the
// documentation stays honest: each feature test below executes a snippet that is
// stored verbatim as it appears in the README, and TestREADMEExamplesInSync
// guards against drift in both directions:
//
//   - a new template example added to the README with no corresponding test
//     fails the sync test (so tests are added for it), and
//   - a test whose snippet no longer appears verbatim in the README fails the
//     sync test (so the example const is kept in step with the docs).
//
// When you change a README example, update the matching const here (and the
// assertions if behavior changed). When you add a new template example to the
// README, either add a test that registers its snippet in readmeExamples or tag
// the code fence as ```html skip_test if it is illustrative only (e.g. it
// depends on templates or data that aren't part of the snippet).

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// The following consts are the README's ```html template examples, copied
// verbatim (TestREADMEExamplesInSync enforces the "verbatim" part).

const liveReloadExample = "{{- define \"SSE /reload\"}}{{.Flush.WaitForServerStop}}data: reload{{printf \"\\n\\n\"}}{{end}}\n" +
	"<script>new EventSource(\"/reload\").onmessage = () => location.reload()</script>\n" +
	"<!-- Maybe not a great idea for production, but you do you. -->"

const customRoutesExample = "<!-- match on path parameters -->\n" +
	"{{define \"GET /contact/{id}\"}}\n" +
	"{{$contact := .DB.QueryRow `SELECT name,phone FROM contacts WHERE id=?` (.Req.PathValue \"id\")}}\n" +
	"<div>\n" +
	"  <span>Name: {{$contact.name}}</span>\n" +
	"  <span>Phone: {{$contact.phone}}</span>\n" +
	"</div>\n" +
	"{{end}}\n" +
	"\n" +
	"<!-- match on any http method -->\n" +
	"{{define \"DELETE /contact/{id}\"}}\n" +
	"{{$_ := .DB.Exec `DELETE from contacts WHERE id=?` (.Req.PathValue \"id\")}}\n" +
	"{{.Resp.SetStatus 204}}\n" +
	"{{end}}"

const dbQueryExample = "<ul>\n" +
	"  {{range .DB.QueryRows `SELECT id,name FROM contacts`}}\n" +
	"  <li><a href=\"/contact/{{.id}}\">{{.name}}</a></li>\n" +
	"  {{end}}\n" +
	"</ul>"

const fsListExample = "<p>Here are the files:\n" +
	"<ol>\n" +
	"{{range .FS.ReadDir \"dir/\"}}\n" +
	"  <li>{{.Name}}</li>\n" +
	"{{end}}\n" +
	"</ol>"

const staticFileHashExample = "{{- with $hash := .X.StaticFileHash `/assets/reset.css`}}\n" +
	"<link rel=\"stylesheet\" href=\"/reset.css?hash={{$hash}}\" integrity=\"{{$hash}}\">\n" +
	"{{- end}}"

// readmeExamples maps a short description to the exact README snippet that one
// or more tests in this file execute. Every template-bearing ```html block in
// the README must appear here, unless its fence is tagged ```html skip_test.
var readmeExamples = map[string]string{
	"live reload (SSE)": liveReloadExample,
	"custom routes":     customRoutesExample,
	"database query":    dbQueryExample,
	"filesystem list":   fsListExample,
	"static file hash":  staticFileHashExample,
}

// newContactsDB returns an in-memory sqlite DB seeded with the `contacts` table
// used by the README's custom-route examples.
func newContactsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE contacts (id INTEGER PRIMARY KEY, name TEXT, phone TEXT)`); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO contacts (id, name, phone) VALUES (1, 'alice', '555-1111'), (2, 'bob', '555-2222')`); err != nil {
		t.Fatalf("failed to seed rows: %v", err)
	}
	return db
}

func countContacts(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM contacts`).Scan(&n); err != nil {
		t.Fatalf("failed to count contacts: %v", err)
	}
	return n
}

// README "Add custom routes": match on path parameters and read a single row
// with .DB.QueryRow and .Req.PathValue.
func TestREADME_CustomRoute_GetContact(t *testing.T) {
	db := newContactsDB(t)
	inst := buildInstance(t,
		map[string]string{"contacts.html": customRoutesExample},
		WithDB("DB", db, nil),
	)

	w := doRequest(inst, http.MethodGet, "/contact/1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "alice") {
		t.Errorf("body = %q, want it to contain %q", body, "alice")
	}
	if !strings.Contains(body, "555-1111") {
		t.Errorf("body = %q, want it to contain %q", body, "555-1111")
	}
}

// README "Add custom routes": match on any http method, run a write with
// .DB.Exec, and set the response status with .Resp.SetStatus.
func TestREADME_CustomRoute_DeleteContact(t *testing.T) {
	db := newContactsDB(t)
	inst := buildInstance(t,
		map[string]string{"contacts.html": customRoutesExample},
		WithDB("DB", db, nil),
	)

	w := doRequest(inst, http.MethodDelete, "/contact/1")
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if n := countContacts(t, db); n != 1 {
		t.Errorf("contact count after delete = %d, want 1", n)
	}
}

// README "Database context provider": range over .DB.QueryRows results.
func TestREADME_DBQueryRange(t *testing.T) {
	db := newContactsDB(t)
	inst := buildInstance(t,
		map[string]string{"list.html": dbQueryExample},
		WithDB("DB", db, nil),
	)

	w := doRequest(inst, http.MethodGet, "/list")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	for _, want := range []string{`href="/contact/1"`, "alice", `href="/contact/2"`, "bob"} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %q, want it to contain %q", body, want)
		}
	}
}

// README "Filesystem context provider": list files with .FS.ReadDir and read
// each entry's name via the fs.FileInfo .Name method.
func TestREADME_FSList(t *testing.T) {
	dataFS := newMemFS(t, map[string]string{
		"dir/one.txt": "1",
		"dir/two.txt": "2",
	})
	inst := buildInstance(t,
		map[string]string{"files.html": fsListExample},
		WithDir("FS", dataFS),
	)

	w := doRequest(inst, http.MethodGet, "/files")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	for _, want := range []string{"one.txt", "two.txt"} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %q, want it to contain %q", body, want)
		}
	}
}

// README "Optimal asset serving": build an SRI/cache-busting link with
// .X.StaticFileHash.
func TestREADME_StaticFileHash(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"assets/reset.css": "*{margin:0}\n",
		"page.html":        staticFileHashExample,
	})

	w := doRequest(inst, http.MethodGet, "/page")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "sha384-") {
		t.Errorf("body = %q, want it to contain an SRI hash %q", body, "sha384-")
	}
	if !strings.Contains(body, "integrity=") {
		t.Errorf("body = %q, want it to contain an integrity attribute", body)
	}
}

// README "Live reload": an `SSE /reload` handler that blocks on
// .Flush.WaitForServerStop and then emits a reload event when the server stops.
func TestREADME_LiveReloadSSE(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	inst := buildInstance(t,
		// The define lives inside a regular page (per the README), so the file
		// path must not itself collide with the SSE route's GET /reload.
		map[string]string{"index.html": liveReloadExample},
		func(c *Config) error { c.Ctx = ctx; return nil },
	)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reload", nil)
	r.Header.Set("Accept", "text/event-stream")

	done := make(chan struct{})
	go func() {
		inst.ServeHTTP(w, r)
		close(done)
	}()

	// Let the request get past the instance's entry check and block inside
	// WaitForServerStop before canceling the server context, which makes the
	// handler resume and write the reload event.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not return after server context cancellation")
	}

	if body := w.Body.String(); !strings.Contains(body, "data: reload") {
		t.Errorf("body = %q, want it to contain %q", body, "data: reload")
	}
}

// TestREADMEExamplesInSync keeps README.md and this test file in lock-step. It
// reads every ```html code block that contains a template action ("{{"), skips
// the ones whose fence is tagged ```html skip_test, and checks that each
// remaining block is executed by a test (registered in readmeExamples), and
// conversely that every registered example still appears verbatim in the README.
func TestREADMEExamplesInSync(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	blocks := extractTemplateHTMLBlocks(string(data))
	if len(blocks) == 0 {
		t.Fatal("found no template ```html blocks in README.md; did the parser or README format change?")
	}

	// Index registered examples by their normalized text for verbatim matching.
	exampleByText := make(map[string]string, len(readmeExamples))
	for name, snippet := range readmeExamples {
		norm := normalizeExample(snippet)
		if other, dup := exampleByText[norm]; dup {
			t.Errorf("examples %q and %q have identical snippets; give them distinct content or merge them", name, other)
		}
		exampleByText[norm] = name
	}

	// blocks are already normalized by extractTemplateHTMLBlocks.
	matched := make(map[string]bool, len(readmeExamples))
	for _, block := range blocks {
		if name, ok := exampleByText[block]; ok {
			matched[name] = true
			continue
		}
		t.Errorf("README.md contains a template example with no matching test:\n\n%s\n\n"+
			"Add a test in readme_test.go that registers this snippet in readmeExamples, "+
			"or tag the code fence as ```html skip_test if it is illustrative only.", block)
	}

	for name := range readmeExamples {
		if !matched[name] {
			t.Errorf("the %q example does not appear verbatim in README.md; "+
				"the README example likely changed. Update the corresponding const in "+
				"readme_test.go to match the README (and the test assertions if behavior changed).", name)
		}
	}
}

// normalizeExample trims trailing whitespace from each line and removes leading
// and trailing blank lines, so cosmetic edges don't cause spurious mismatches.
func normalizeExample(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	start, end := 0, len(lines)
	for start < end && lines[start] == "" {
		start++
	}
	for end > start && lines[end-1] == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

// extractTemplateHTMLBlocks returns the contents of every fenced ```html code
// block in the README that contains a template action ("{{"), with the snippet
// normalized for comparison. Blocks whose fence info string carries the
// `skip_test` tag (e.g. ```html skip_test) are excluded, marking them as
// illustrative-only and exempt from the sync check.
//
// goldmark already backs the `markdown` template func, and being a CommonMark
// parser it understands the README's <details> blockquotes natively: the fenced
// code blocks nested inside them are reported with their `> ` markers already
// stripped, so no manual unwrapping is needed.
func extractTemplateHTMLBlocks(md string) []string {
	source := []byte(md)
	doc := goldmark.New().Parser().Parse(text.NewReader(source))

	var blocks []string
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}
		// Parse the full fence info string, e.g. "html" or "html skip_test".
		var info string
		if fcb.Info != nil {
			info = string(fcb.Info.Segment.Value(source))
		}
		tags := strings.Fields(info)
		if len(tags) == 0 || tags[0] != "html" || slices.Contains(tags[1:], "skip_test") {
			return ast.WalkSkipChildren, nil
		}
		var sb strings.Builder
		lines := fcb.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			sb.Write(seg.Value(source))
		}
		if body := sb.String(); strings.Contains(body, "{{") {
			blocks = append(blocks, normalizeExample(body))
		}
		return ast.WalkSkipChildren, nil
	})
	return blocks
}
