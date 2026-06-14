package xtemplate

import "testing"

func TestConfigMinifyDefault(t *testing.T) {
	// New() (the documented constructor) defaults Minify to true, matching the
	// CLI's default:"true" tag.
	if !New().Minify {
		t.Error("New() should default Minify to true")
	}

	// An explicit Minify=false must survive re-application of Defaults, which
	// Server and Instance call internally. If Defaults clobbered it, minify
	// could never be disabled via the Go API.
	c := New()
	c.Minify = false
	c.Defaults()
	if c.Minify {
		t.Error("Defaults() must not clobber an explicit Minify=false")
	}
}
