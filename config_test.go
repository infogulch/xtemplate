package xtemplate

import "testing"

func TestConfigMinifyDefault(t *testing.T) {
	// New() (the documented constructor) defaults Minify to true, matching the
	// CLI's default:"true" tag.
	if m := New().Minify; m == nil || !*m {
		t.Error("New() should default Minify to true")
	}

	// Defaults() must default an unset (nil) Minify to true, so the documented
	// default holds regardless of construction path (not just via New()).
	c := &Config{}
	c.Defaults()
	if c.Minify == nil || !*c.Minify {
		t.Error("Defaults() should default an unset Minify to true")
	}

	// An explicit Minify=false must survive re-application of Defaults, which
	// Server and Instance call internally. If Defaults clobbered it, minify
	// could never be disabled via the Go API.
	explicitFalse := false
	c = &Config{Minify: &explicitFalse}
	c.Defaults()
	if c.Minify == nil || *c.Minify {
		t.Error("Defaults() must not clobber an explicit Minify=false")
	}
}
