package app

import (
	"os"
	"testing"

	"github.com/infogulch/xtemplate"
	_ "github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/controllers/watchfs"
)

func TestLoadConfig_GetsArgsFromOSArgs(t *testing.T) {
	savedArgs := os.Args
	defer func() { os.Args = savedArgs }()
	os.Args = []string{"xtemplate", "--listen", ":7777"}
	config, err := LoadConfig(nil)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Listen != ":7777" {
		t.Errorf("Listen = %q, want %q (from OS args)", config.Listen, ":7777")
	}
}

func TestArgsMinifyDefault(t *testing.T) {
	// Empty slice (not nil): nil would use os.Args and pick up go test flags.
	config, err := LoadConfig([]string{})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Minify == nil || !*config.Minify {
		t.Errorf("Minify = %v, want non-nil true by default", config.Minify)
	}

	config, err = LoadConfig([]string{"--minify=false"})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Minify == nil || *config.Minify {
		t.Errorf("Minify = %v, want non-nil false when --minify=false", config.Minify)
	}
}

func TestLoadConfig_NoControllers(t *testing.T) {
	config, err := LoadConfig([]string{"--listen", ":5555"})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Listen != ":5555" {
		t.Errorf("Listen = %q, want %q", config.Listen, ":5555")
	}
}

func TestLoadConfig_JSONApplied(t *testing.T) {
	config, err := LoadConfig([]string{"-c", `{"minify":false,"listen":":9999"}`})
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

func TestLoadConfig_JSONOmittingListenKeepsDefault(t *testing.T) {
	// Config files often omit listen; Docker relies on SetDefaults / ldflags.
	config, err := LoadConfig([]string{"-c", `{"minify":false,"controller":{"type":"os","path":"templates"}}`})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Listen != defaultListenAddress {
		t.Errorf("Listen = %q, want default %q when JSON omits listen", config.Listen, defaultListenAddress)
	}
}

func TestLoadConfig_CLIOverridesJSON(t *testing.T) {
	argv := []string{"-c", `{"listen":":9999"}`, "--listen", ":7777"}
	config, err := LoadConfig(argv)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Listen != ":7777" {
		t.Errorf("Listen = %q, want %q (CLI must override JSON)", config.Listen, ":7777")
	}
}

func TestLoadConfig_FileController(t *testing.T) {
	tmpFileName, cleanup := mkTemp(t, "conf-*.json", `{"listen":":1234","controller":{"type":"os","path":"hello"}}`)
	defer cleanup()
	config, err := LoadConfig([]string{"-f", tmpFileName})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if config.Listen != ":1234" {
		t.Errorf("Listen = %q, want %q (from config file)", config.Listen, ":1234")
	}
	if _, ok := config.Controller.(*xtemplate.OsFsController); !ok {
		t.Errorf("Controller = %T, want *OsFsController", config.Controller)
	}
	if config.ControllerRaw != nil {
		t.Error("ControllerRaw should be cleared after materialize")
	}
}

func TestLoadConfig_BannedKey(t *testing.T) {
	_, err := LoadConfig([]string{"-c", `{"templates_dir":"x"}`})
	if err == nil {
		t.Fatal("expected error for banned templates_dir key")
	}
}

func TestLoadConfig_CLIControllerTypeWinsOverJSON(t *testing.T) {
	// CLI --controller-type overrides JSON controller.type (CLI > JSON).
	config, err := LoadConfig([]string{
		"-c", `{"controller":{"type":"os","path":"templates"}}`,
		"--controller-type", "watchfs",
	})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := config.Controller.(*watchfs.Controller); !ok {
		t.Errorf("Controller = %T, want *watchfs.Controller (CLI wins)", config.Controller)
	}
	if config.ControllerRaw != nil {
		t.Error("ControllerRaw should be cleared after materialize")
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

func TestLoadConfig_LoggerSetWithConfig(t *testing.T) {
	config, err := LoadConfig([]string{"-c", `{"listen":":9999"}`})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if config.Logger == nil {
		t.Fatal("LoadConfig returned config with nil Logger after decoding config")
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

func TestLoadConfig_DefaultFollowsDefaultControllerType(t *testing.T) {
	prev := xtemplate.DefaultControllerType
	xtemplate.DefaultControllerType = "watchfs"
	t.Cleanup(func() { xtemplate.DefaultControllerType = prev })

	config, err := LoadConfig([]string{"--listen", ":0"})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := config.Controller.(*watchfs.Controller); !ok {
		t.Errorf("Controller = %T, want *watchfs.Controller (DefaultControllerType)", config.Controller)
	}
}

func TestLoadConfig_CoreDefaultIsOS(t *testing.T) {
	prev := xtemplate.DefaultControllerType
	xtemplate.DefaultControllerType = "os"
	t.Cleanup(func() { xtemplate.DefaultControllerType = prev })

	config, err := LoadConfig([]string{"--listen", ":0"})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := config.Controller.(*xtemplate.OsFsController); !ok {
		t.Errorf("Controller = %T, want *OsFsController", config.Controller)
	}
}

func TestLoadConfig_ExplicitUnregisteredControllerErrors(t *testing.T) {
	_, err := LoadConfig([]string{"--controller-type", "not_a_linked_controller"})
	if err == nil {
		t.Fatal("expected error for explicit unregistered --controller-type")
	}
}
