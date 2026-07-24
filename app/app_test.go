package app

import (
	"os"
	"testing"

	"github.com/infogulch/xtemplate"
	_ "github.com/infogulch/xtemplate/sources/watchfs"
)

func TestLoadConfig_GetsArgsFromOSArgs(t *testing.T) {
	savedArgs := os.Args
	defer func() { os.Args = savedArgs }()
	os.Args = []string{"xtemplate", "--listen", ":7777", "--source-type", "os"}
	config, err := LoadConfig(nil)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Listen != ":7777" {
		t.Errorf("Listen = %q, want %q (from OS args)", config.Listen, ":7777")
	}
}

func TestArgsMinifyDefault(t *testing.T) {
	config, err := LoadConfig([]string{"--source-type", "os"})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Minify == nil || !*config.Minify {
		t.Errorf("Minify = %v, want non-nil true by default", config.Minify)
	}

	config, err = LoadConfig([]string{"--source-type", "os", "--minify=false"})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Minify == nil || *config.Minify {
		t.Errorf("Minify = %v, want non-nil false when --minify=false", config.Minify)
	}
}

func TestLoadConfig_NoSources(t *testing.T) {
	config, err := LoadConfig([]string{"--listen", ":5555", "--source-type", "os"})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Listen != ":5555" {
		t.Errorf("Listen = %q, want %q", config.Listen, ":5555")
	}
}

func TestLoadConfig_JSONApplied(t *testing.T) {
	config, err := LoadConfig([]string{"-c", `{"minify":false,"listen":":9999"}`, "--source-type", "os"})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Listen != ":9999" {
		t.Errorf("Listen = %q, want %q (from JSON arg)", config.Listen, ":9999")
	}
	if config.Minify == nil || *config.Minify {
		t.Errorf("Minify = %v, want non-nil false (from JSON arg)", config.Minify)
	}
}

func TestLoadConfig_CLIOverridesJSON(t *testing.T) {
	argv := []string{"-c", `{"listen":":9999"}`, "--listen", ":7777", "--source-type", "os"}
	config, err := LoadConfig(argv)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Listen != ":7777" {
		t.Errorf("Listen = %q, want %q (CLI must override JSON)", config.Listen, ":7777")
	}
}

func TestLoadConfig_FileSource(t *testing.T) {
	tmpFileName, cleanup := mkTemp(t, "conf-*.json", `{"listen":":1234","source":{"type":"os","path":"hello"}}`)
	defer cleanup()
	config, err := LoadConfig([]string{"-f", tmpFileName})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Listen != ":1234" {
		t.Errorf("Listen = %q, want %q (from config file)", config.Listen, ":1234")
	}
	if config.SourceType != "os" {
		t.Errorf("SourceType = %q, want os", config.SourceType)
	}
	if config.SourceRaw != nil {
		t.Error("SourceRaw should be cleared after materialize")
	}
}

func TestLoadConfig_BannedKey(t *testing.T) {
	_, err := LoadConfig([]string{"-c", `{"templates_dir":"x"}`})
	if err == nil {
		t.Fatal("expected error for banned templates_dir key")
	}
}

func TestLoadConfig_SourceTypeMismatch(t *testing.T) {
	_, err := LoadConfig([]string{
		"-c", `{"source":{"type":"os","path":"templates"}}`,
		"--source-type", "watchfs",
	})
	if err == nil {
		t.Fatal("expected mismatch error")
	}
}

func mkTemp(t *testing.T, name, content string) (string, func()) {
	tmpFile, err := os.CreateTemp("", name)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFileName := tmpFile.Name()
	n, err := tmpFile.Write([]byte(content))
	if err != nil || n != len(content) {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return tmpFileName, func() {
		err := tmpFile.Close()
		if err != nil {
			t.Fatalf("failed to close temp file: %v", err)
		}
		err = os.Remove(tmpFileName)
		if err != nil {
			t.Errorf("failed to remove temp file: %v", err)
		}
	}
}

func TestLoadConfig_LoggerSetWithConfigSource(t *testing.T) {
	config, err := LoadConfig([]string{"-c", `{"listen":":9999"}`, "--source-type", "os"})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if config.Logger == nil {
		t.Fatal("LoadConfig returned config with nil Logger after decoding a config source")
	}
}

func TestLoadConfig_BadJSON(t *testing.T) {
	config, err := LoadConfig([]string{"-c", `{not valid json`})
	if err == nil {
		t.Error("expected an error for malformed JSON, got nil")
	}
	if config == nil || config.Logger == nil {
		t.Error("expected Logger to be set after loading config even with bad JSON, got nil")
	}
}

func TestLoadConfig_DefaultFallsBackWhenUnregistered(t *testing.T) {
	// Simulate a custom binary (e.g. examples/embedded) that uses app.Main but
	// does not blank-import the release default source type (watchfs).
	saved := defaultSourceType
	defaultSourceType = "not_a_linked_source"
	defer func() { defaultSourceType = saved }()

	config, err := LoadConfig([]string{"--listen", ":0"})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if config.SourceType != "os" {
		t.Errorf("SourceType = %q, want os fallback when default is unregistered", config.SourceType)
	}
	if _, ok := config.Source.(*xtemplate.OsFsSource); !ok {
		t.Errorf("Source = %T, want *OsFsSource", config.Source)
	}
}

func TestLoadConfig_ExplicitUnregisteredSourceErrors(t *testing.T) {
	_, err := LoadConfig([]string{"--source-type", "not_a_linked_source"})
	if err == nil {
		t.Fatal("expected error for explicit unregistered --source-type")
	}
}
