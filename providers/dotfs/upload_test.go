package dotfs

import (
	"strings"
	"testing"
)

func TestStoredExtension(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"photo.JPG", ".jpg"},
		{"a.b.C", ".c"},
		{"noext", ".upload"},
		{"ends.", ".upload"},
		{"x." + strings.Repeat("a", 21), ".upload"},
		{"x.jpeǵ", ".upload"},
		{"x.foo-bar", ".upload"},
		{`C:\path\file.PNG`, ".png"},
		{"", ".upload"},
	}
	for _, tc := range cases {
		if got := storedExtension(tc.in); got != tc.want {
			t.Errorf("storedExtension(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCleanUploadDir(t *testing.T) {
	ok, err := cleanUploadDir("uploads")
	if err != nil || ok != "uploads" {
		t.Fatalf("uploads: got %q, %v", ok, err)
	}
	ok, err = cleanUploadDir("a/b/../c")
	if err != nil || ok != "a/c" {
		t.Fatalf("a/b/../c: got %q, %v", ok, err)
	}
	for _, bad := range []string{"", "..", "../x", "/abs", `C:\x`, `foo\bar`} {
		if _, err := cleanUploadDir(bad); err == nil {
			t.Errorf("cleanUploadDir(%q): want error", bad)
		}
	}
}
