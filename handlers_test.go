package xtemplate

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/spf13/afero"
)

// implementsSyscallConn mirrors the exact check in net.sendFile (net/sendfile.go):
// sendfile(2) is used only when the reader handed to http.ServeContent satisfies
// syscall.Conn. The cases below pin down the behavior we rely on so this test
// fails loudly if a future afero/Go version changes it (e.g. if *BasePathFile
// ever forwards SyscallConn, osFile would no longer be needed).
func implementsSyscallConn(f afero.File) bool {
	_, ok := f.(syscall.Conn)
	return ok
}

func TestOSFile_SendfilePrecondition(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Case 1: the default static-file FS (BasePathFs over OsFs) returns a
	// *BasePathFile that does NOT implement syscall.Conn, so sendfile(2) would
	// not be used if it were passed to ServeContent directly.
	osBacked := afero.NewBasePathFs(afero.NewOsFs(), dir)
	f, err := osBacked.Open("f.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, ok := f.(*afero.BasePathFile); !ok {
		t.Fatalf("expected BasePathFs.Open to return *afero.BasePathFile, got %T", f)
	}
	if implementsSyscallConn(f) {
		t.Error("*BasePathFile now implements syscall.Conn; the osFile unwrap may be unnecessary")
	}

	// Case 2: unwrapping that file yields an *os.File, which DOES implement
	// syscall.Conn, restoring the sendfile(2) fast path.
	osf, ok := osFile(f)
	if !ok {
		t.Fatal("osFile failed to unwrap *BasePathFile (OsFs) to *os.File")
	}
	if _, ok := any(osf).(syscall.Conn); !ok {
		t.Error("*os.File does not implement syscall.Conn; sendfile(2) would not be used")
	}

	// Case 3: a BasePathFile backed by MemMapFs (e.g. the git app's FS) has no
	// *os.File underneath, so it neither implements syscall.Conn nor unwraps.
	memBacked := afero.NewBasePathFs(afero.NewMemMapFs(), "/")
	if err := afero.WriteFile(memBacked, "f.txt", []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	mf, err := memBacked.Open("f.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mf.Close() }()
	if _, ok := mf.(*afero.BasePathFile); !ok {
		t.Fatalf("expected BasePathFs.Open to return *afero.BasePathFile, got %T", mf)
	}
	if implementsSyscallConn(mf) {
		t.Error("MemMapFs-backed *BasePathFile unexpectedly implements syscall.Conn")
	}
	if _, ok := osFile(mf); ok {
		t.Error("osFile unexpectedly unwrapped a MemMapFs-backed file to *os.File")
	}
}

// encs builds an encodingInfo slice from coding names, in the given order. The
// order matters: negotiateEncoding breaks q-value ties toward earlier entries,
// and the builder sorts real encodings size-ascending.
func encs(names ...string) []encodingInfo {
	out := make([]encodingInfo, len(names))
	for i, n := range names {
		out[i] = encodingInfo{encoding: n}
	}
	return out
}

func TestNegotiateEncoding(t *testing.T) {
	// For a tiny file, gzip overhead makes the compressed copy larger than the
	// original, so identity sorts first.
	tiny := encs("identity", "gzip")
	// For a real CSS file, br < gzip < identity by size.
	css := encs("br", "gzip", "identity")

	tests := []struct {
		name    string
		encs    []encodingInfo
		accept  []string
		want    string // expected encoding; "" with wantNil means 406
		wantNil bool
	}{
		// Behaviors locked in by test/tests/assets.hurl.
		{"no header serves identity", tiny, nil, "identity", false},
		{"empty header serves identity", tiny, []string{""}, "identity", false},
		{"gzip requested", tiny, []string{"gzip"}, "gzip", false},
		{"gzip or identity prefers identity (earlier in list)", tiny, []string{"gzip, identity"}, "identity", false},
		{"0.09 gzip pref still identity (within tie threshold)", tiny, []string{"gzip;q=0.5, identity;q=0.41"}, "identity", false},
		{"0.11 gzip pref selects gzip (beyond threshold)", tiny, []string{"gzip;q=0.5, identity;q=0.39"}, "gzip", false},
		{"unavailable coding falls back to identity", tiny, []string{"br"}, "identity", false},
		{"css gzip", css, []string{"gzip"}, "gzip", false},
		{"css gzip or br prefers br (earlier/smaller)", css, []string{"gzip, br"}, "br", false},

		// New: q=0 means "not acceptable" (RFC 7231 §5.3.4).
		{"identity refused selects available gzip", tiny, []string{"gzip, identity;q=0"}, "gzip", false},
		{"identity refused with no alternative is 406", tiny, []string{"identity;q=0"}, "", true},
		{"gzip refused falls back to identity", tiny, []string{"gzip;q=0"}, "identity", false},

		// New: wildcard support.
		{"wildcard accepts smallest available", css, []string{"*"}, "br", false},
		{"wildcard q=0 refuses everything", tiny, []string{"*;q=0"}, "", true},
		{"explicit overrides wildcard refusal", tiny, []string{"*;q=0, gzip"}, "gzip", false},
		{"wildcard refuses identity but names gzip", tiny, []string{"*;q=0, gzip;q=1"}, "gzip", false},
		{"identity explicit survives wildcard q=0", tiny, []string{"*;q=0, identity"}, "identity", false},

		// Multiple header lines are combined.
		{"split across header lines", tiny, []string{"gzip;q=0.5", "identity;q=0.39"}, "gzip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := negotiateEncoding(tt.accept, tt.encs)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil (406) result, got %q (err=%v)", got.encoding, err)
				}
				if err == nil {
					t.Error("expected a non-nil error explaining why nothing is acceptable")
				}
				return
			}
			if got == nil {
				t.Fatalf("got nil result, want %q (err=%v)", tt.want, err)
			}
			if got.encoding != tt.want {
				t.Errorf("encoding = %q, want %q", got.encoding, tt.want)
			}
		})
	}
}

func TestNegotiateEncoding_NoEncodings(t *testing.T) {
	got, err := negotiateEncoding(nil, nil)
	if got != nil {
		t.Errorf("expected nil result for empty encodings, got %v", got)
	}
	if err == nil {
		t.Error("expected an error for empty encodings")
	}
}
