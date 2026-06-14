package app

import (
	"testing"

	"github.com/alexflint/go-arg"
)

// parseCLI mirrors Main's first pass: parse argv onto a copy of defaultArgs and
// apply Defaults, returning the resulting Args.
func parseCLI(t *testing.T, argv []string) Args {
	t.Helper()
	cfg := defaultArgs
	p, err := arg.NewParser(arg.Config{}, &cfg)
	if err != nil {
		t.Fatalf("failed to construct parser: %v", err)
	}
	if err := p.Parse(argv); err != nil {
		t.Fatalf("failed to parse %v: %v", argv, err)
	}
	cfg.Defaults()
	return cfg
}

func TestArgsMinifyDefault(t *testing.T) {
	// No --minify flag: the default:"true" tag makes Minify a non-nil true.
	cfg := parseCLI(t, nil)
	if cfg.Minify == nil || !*cfg.Minify {
		t.Errorf("Minify = %v, want non-nil true by default", cfg.Minify)
	}

	// --minify=false explicitly disables it.
	cfg = parseCLI(t, []string{"--minify=false"})
	if cfg.Minify == nil || *cfg.Minify {
		t.Errorf("Minify = %v, want non-nil false when --minify=false", cfg.Minify)
	}
}

func TestMergeConfig_NoSources(t *testing.T) {
	argv := []string{"--listen", ":5555"}
	cli := parseCLI(t, argv)
	merged, err := mergeConfig(argv, cli, nil)
	if err != nil {
		t.Fatalf("mergeConfig error: %v", err)
	}
	if merged.Listen != ":5555" {
		t.Errorf("Listen = %q, want %q", merged.Listen, ":5555")
	}
}

func TestMergeConfig_JSONApplied(t *testing.T) {
	argv := []string{"-c", `{"minify":false,"listen":":9999"}`}
	cli := parseCLI(t, argv)
	merged, err := mergeConfig(argv, cli, nil)
	if err != nil {
		t.Fatalf("mergeConfig error: %v", err)
	}
	if merged.Listen != ":9999" {
		t.Errorf("Listen = %q, want %q (from JSON)", merged.Listen, ":9999")
	}
	if merged.Minify == nil || *merged.Minify {
		t.Errorf("Minify = %v, want non-nil false (from JSON)", merged.Minify)
	}
}

func TestMergeConfig_CLIOverridesJSON(t *testing.T) {
	argv := []string{"-c", `{"listen":":9999"}`, "--listen", ":7777"}
	cli := parseCLI(t, argv)
	merged, err := mergeConfig(argv, cli, nil)
	if err != nil {
		t.Fatalf("mergeConfig error: %v", err)
	}
	if merged.Listen != ":7777" {
		t.Errorf("Listen = %q, want %q (CLI must override JSON)", merged.Listen, ":7777")
	}
}

func TestMergeConfig_FileSource(t *testing.T) {
	argv := []string{"-f", "conf.json"}
	cli := parseCLI(t, argv)
	read := func(name string) ([]byte, error) {
		if name != "conf.json" {
			t.Errorf("readFile called with %q, want %q", name, "conf.json")
		}
		return []byte(`{"listen":":1234"}`), nil
	}
	merged, err := mergeConfig(argv, cli, read)
	if err != nil {
		t.Fatalf("mergeConfig error: %v", err)
	}
	if merged.Listen != ":1234" {
		t.Errorf("Listen = %q, want %q (from config file)", merged.Listen, ":1234")
	}
}

func TestMergeConfig_BadJSON(t *testing.T) {
	argv := []string{"-c", `{not valid json`}
	cli := parseCLI(t, argv)
	if _, err := mergeConfig(argv, cli, nil); err == nil {
		t.Error("expected an error for malformed JSON, got nil")
	}
}
