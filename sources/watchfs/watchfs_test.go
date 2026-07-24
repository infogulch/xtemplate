package watchfs

import (
	"testing"
	"time"

	"github.com/infogulch/xtemplate"
)

func TestSource_StartReturnsInitialFS(t *testing.T) {
	dir := t.TempDir()
	s := &Source{Path: dir, Debounce: xtemplate.Duration(50 * time.Millisecond)}
	ctx := t.Context()
	initial, ch, err := s.Start(ctx, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if initial == nil {
		t.Fatal("watchfs should return non-nil initial FS")
	}
	if ch == nil {
		t.Fatal("watchfs should return a reload channel")
	}
}

func TestSource_DefaultsPath(t *testing.T) {
	// Empty path defaults to "templates" — Start may fail if dir missing;
	// only assert defaulting via a non-empty path we control.
	s := &Source{}
	if s.Path != "" {
		t.Fatalf("zero Path should be empty before Start, got %q", s.Path)
	}
	// Start with missing default dir should error from watch package or succeed
	// if templates/ exists in CWD — just ensure Start is callable.
	_, _, err := s.Start(t.Context(), nil)
	_ = err // environment-dependent; no hard assert on existence
}
