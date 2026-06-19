package app

import (
	"os"
	"testing"
)

func TestLoadConfig_GetsArgsFromOSArgs(t *testing.T) {
	savedArgs := os.Args
	defer func() { os.Args = savedArgs }()
	os.Args = []string{"xtemplate", "--listen", ":7777"}
	config, err := LoadConfig(&Config{}, nil)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Listen != ":7777" {
		t.Errorf("Listen = %q, want %q (from OS args)", config.Listen, ":7777")
	}
}

func TestArgsMinifyDefault(t *testing.T) {
	// No --minify flag: the default:"true" tag makes Minify a non-nil true.
	config, err := LoadConfig(&Config{}, []string{})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Minify == nil || !*config.Minify {
		t.Errorf("Minify = %v, want non-nil true by default", config.Minify)
	}

	// --minify=false explicitly disables it.
	config, err = LoadConfig(&Config{}, []string{"--minify=false"})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Minify == nil || *config.Minify {
		t.Errorf("Minify = %v, want non-nil false when --minify=false", config.Minify)
	}
}

func TestLoadConfig_NoSources(t *testing.T) {
	config, err := LoadConfig(&Config{}, []string{"--listen", ":5555"})
	if err != nil {
		t.Fatalf("mergeConfig error: %v", err)
	}
	if config.Listen != ":5555" {
		t.Errorf("Listen = %q, want %q", config.Listen, ":5555")
	}
}

func TestLoadConfig_JSONApplied(t *testing.T) {
	config, err := LoadConfig(&Config{}, []string{"-c", `{"minify":false,"listen":":9999"}`})
	if err != nil {
		t.Fatalf("mergeConfig error: %v", err)
	}
	if config.Listen != ":9999" {
		t.Errorf("Listen = %q, want %q (from JSON arg)", config.Listen, ":9999")
	}
	if config.Minify == nil || *config.Minify {
		t.Errorf("Minify = %v, want non-nil false (from JSON arg)", config.Minify)
	}
}

func TestLoadConfig_CLIOverridesJSON(t *testing.T) {
	argv := []string{"-c", `{"listen":":9999"}`, "--listen", ":7777"}
	config, err := LoadConfig(&Config{}, argv)
	if err != nil {
		t.Fatalf("mergeConfig error: %v", err)
	}
	if config.Listen != ":7777" {
		t.Errorf("Listen = %q, want %q (CLI must override JSON)", config.Listen, ":7777")
	}
}

func TestLoadConfig_FileSource(t *testing.T) {
	tmpFileName, cleanup := mkTemp(t, "conf-*.json", `{"listen":":1234"}`)
	defer cleanup()
	config, err := LoadConfig(&Config{}, []string{"-f", tmpFileName, "--templates-dir", "hello"})
	if err != nil {
		t.Fatalf("mergeConfig error: %v", err)
	}
	if config.Listen != ":1234" {
		t.Errorf("Listen = %q, want %q (from config file)", config.Listen, ":1234")
	}
	if config.TemplatesDir != "hello" {
		t.Errorf("TemplatesDir = %q, want %q (json must not clobber unnamed cli arg)", config.TemplatesDir, "hello")
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
	config, err := LoadConfig(&Config{}, []string{"-c", `{"listen":":9999"}`})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if config.Logger == nil {
		t.Fatal("LoadConfig returned config with nil Logger after decoding a config source")
	}
}

func TestLoadConfig_BadJSON(t *testing.T) {
	config, err := LoadConfig(&Config{}, []string{"-c", `{not valid json`})
	if err == nil {
		t.Error("expected an error for malformed JSON, got nil")
	}
	if config.Logger == nil {
		t.Error("expected Logger to be set after loading config even with bad JSON, got nil")
	}
}

type testConfigDoesNotCallSetDefaults struct {
	Config
}

func (c *testConfigDoesNotCallSetDefaults) SetDefaults() {
	// Test what happens when we don't call the embedded SetDefaults
}

func TestLoadConfig_SetsLoggerWhenOverriding(t *testing.T) {
	config, err := LoadConfig(&testConfigDoesNotCallSetDefaults{}, []string{"-c", `{"listen":":9999"}`})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if config.Logger == nil {
		t.Errorf("LoadConfig returned config with nil Logger")
	}
}
