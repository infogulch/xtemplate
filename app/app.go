package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"

	"github.com/infogulch/xtemplate"

	"github.com/alexflint/go-arg"
)

// Config is the CLI app configuration: listen address, log level, and embedded
// xtemplate.Config for template/server options.
type Config struct {
	xtemplate.Config
	Listen      string   `json:"listen" arg:"-l,--listen"`
	LogLevel    int      `json:"log_level" arg:"--loglevel" default:"-2"`
	Configs     []string `json:"-" arg:"-c,--config,separate"`
	ConfigFiles []string `json:"-" arg:"-f,--config-file,separate"`
}

// controllerTypeFlag is kept separate to avoid polluting app.Config.
type controllerTypeFlag struct {
	Type string `arg:"--controller-type" help:"template controller type (see DefaultControllerType when omitted)"`
}

// UnmarshalJSON fills app + embedded xtemplate fields. Uses a method-less Config
// alias so listen/log_level are not dropped. listen and log_level overwrite only
// when present so defaults survive omitted keys. Call CheckLegacyTemplateKeys
// before this when needed (LoadConfig does).
func (a *Config) UnmarshalJSON(data []byte) error {
	type plainXT xtemplate.Config
	type alias struct {
		plainXT
		Listen   *string `json:"listen"`
		LogLevel *int    `json:"log_level"`
	}
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.Config = xtemplate.Config(raw.plainXT)
	if raw.Listen != nil {
		a.Listen = *raw.Listen
	}
	if raw.LogLevel != nil {
		a.LogLevel = *raw.LogLevel
	}
	return nil
}

// defaultListenAddress allows a build-time override:
//
//	-ldflags="-X 'github.com/infogulch/xtemplate/app.defaultListenAddress=0.0.0.0:80'"
//
// Docker sets listen :80 via -ldflags.
var defaultListenAddress = "0.0.0.0:8080"

// SetDefaults sets the default values for this Config.
func (a *Config) SetDefaults() {
	a.Listen = defaultListenAddress
	a.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(a.LogLevel)}))
	a.Config.SetDefaults()
}

// Epilogue is shown at the end of --help.
func (Config) Epilogue() string {
	controllers := strings.Join(xtemplate.RegisteredControllerTypes(), ", ")
	if controllers == "" {
		controllers = "(none registered)"
	}
	def := xtemplate.DefaultControllerType
	return fmt.Sprintf(`Controller types (this build): %s
Default --controller-type: %s
Examples:
    Listen on port 80 (this build's default controller):
    ❯ %[3]s --listen :80

    Specify a template directory (os/watchfs/git path):
    ❯ %[3]s --templates-dir public

    No auto-reload:
    ❯ %[3]s --controller-type os

    Use git controller:
    ❯ %[3]s --controller-type git --git-repo https://example.com/site.git

    Parse template files matching a custom extension; disable minify:
    ❯ %[3]s --template-ext ".go.html" --minify=false`, controllers, def, os.Args[0])
}

// version is stamped at build time for releases via
// -ldflags="-X 'github.com/infogulch/xtemplate/app.version=v1.2.3'". When unset
// (e.g. `go install ...@version` or a plain `go build`), Version() falls back to
// the module/VCS info embedded by the Go toolchain.
var version = ""

func (Config) Version() string {
	if version != "" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "development"
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		return "devel-" + rev + dirty
	}
	return "development"
}

// Main can be called from your func main() if you want your program to act like
// the default xtemplate cli, or use it as a reference for making your own.
// Provide config options to override the defaults like:
//
//	app.Main(xtemplate.WithFooConfig())
func Main(overrides ...xtemplate.Option) {
	config, err := LoadConfig(nil)
	if err != nil {
		// Logger may not be ready; print to stderr.
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}
	Serve(config, overrides...)
}

// LoadConfig merges configuration: CLI flags > inline JSON > files > defaults.
// Pipeline: defaults → pass0 bootstrap → merge JSON → materialize controller
// (CLI type > JSON > default) → parse CLI (help/version exit via go-arg) →
// finalize. Pass nil for os.Args[1:].
func LoadConfig(args []string) (*Config, error) {
	if args == nil {
		args = os.Args[1:]
	}

	config := &Config{}
	config.SetDefaults()

	p0, err := scanPass0(args)
	if err != nil {
		return config, err
	}

	if err := mergeJSON(config, p0.configFiles, p0.configs); err != nil {
		return config, err
	}

	// Empty controllerType means no CLI override (JSON / default win).
	effectiveType, err := config.MaterializeController(p0.controllerType)
	if err != nil {
		return config, err
	}

	if err := parseCLI(config, config.Controller, args); err != nil {
		return config, err
	}

	finalize(config, effectiveType)
	return config, nil
}

// pass0 holds argv scan results before full go-arg parsing.
// Hand-scanned because go-arg needs the concrete controller type as a dest for
// type-specific flags, but that type depends on --controller-type and JSON
// (chicken-and-egg). See docs/reference/cli.md.
type pass0 struct {
	controllerType string
	configFiles    []string
	configs        []string
}

// scanPass0 scans argv for --controller-type, -f/--config-file, and -c/--config
// without full go-arg on controller structs. Help/version are left to go-arg.
func scanPass0(args []string) (pass0, error) {
	var p pass0
	types, err := flagValues(args, "--controller-type")
	if err != nil {
		return p, err
	}
	if n := len(types); n > 0 {
		p.controllerType = types[n-1] // last wins
	}
	if p.configFiles, err = flagValues(args, "-f", "--config-file"); err != nil {
		return p, err
	}
	if p.configs, err = flagValues(args, "-c", "--config"); err != nil {
		return p, err
	}
	return p, nil
}

// flagValues collects every occurrence of the named flags from args: separate
// form (name value) or, for long names, inline form (name=value). Left-to-right.
func flagValues(args []string, names ...string) ([]string, error) {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		for _, name := range names {
			if a == name {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("%s requires a value", name)
				}
				i++
				out = append(out, args[i])
				break
			}
			if strings.HasPrefix(name, "--") && strings.HasPrefix(a, name+"=") {
				out = append(out, strings.TrimPrefix(a, name+"="))
				break
			}
		}
	}
	return out, nil
}

// mergeJSON applies config files then inline fragments in order. Later sources win.
func mergeJSON(config *Config, configFiles, configs []string) error {
	apply := func(data []byte, origin string) error {
		if err := xtemplate.CheckLegacyTemplateKeys(data); err != nil {
			return fmt.Errorf("%s: %w", origin, err)
		}
		if err := json.Unmarshal(data, config); err != nil {
			return fmt.Errorf("%s: %w", origin, err)
		}
		return nil
	}
	for _, name := range configFiles {
		data, err := os.ReadFile(name)
		if err != nil {
			return fmt.Errorf("failed to read config file %q: %w", name, err)
		}
		if err := apply(data, fmt.Sprintf("config file %q", name)); err != nil {
			return err
		}
	}
	for _, conf := range configs {
		if err := apply([]byte(conf), "--config"); err != nil {
			return err
		}
	}
	return nil
}

// parseCLI binds argv into app config and the already-materialized controller.
// typeFlag absorbs --controller-type so go-arg does not reject it.
// --help / --version print and exit like arg.MustParse; other errors are returned.
func parseCLI(config *Config, controller xtemplate.ServerController, args []string) error {
	var typeFlag controllerTypeFlag
	parser, err := arg.NewParser(arg.Config{}, config, controller, &typeFlag)
	if err != nil {
		return err
	}
	switch err := parser.Parse(args); {
	case errors.Is(err, arg.ErrHelp):
		parser.WriteHelp(os.Stdout)
		os.Exit(0)
		return nil
	case errors.Is(err, arg.ErrVersion):
		_, _ = fmt.Fprintln(os.Stdout, config.Version())
		os.Exit(0)
		return nil
	case err != nil:
		return fmt.Errorf("failed to parse cli flags: %w", err)
	}
	return nil
}

// finalize rebuilds the logger after flags/JSON so --loglevel applies.
func finalize(config *Config, effectiveType string) {
	config.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(config.LogLevel)}))
	config.Logger.Debug("loaded configuration", slog.String("controller_type", effectiveType), slog.Any("listen", config.Listen))
}

// Serve sets up the xtemplate server from config and serves it.
// Serve blocks until the server stops.
func Serve(config *Config, options ...xtemplate.Option) {
	_, err := config.Options(options...)
	if err != nil {
		config.Logger.Error("failed to apply overrides", slog.Any("error", err))
		os.Exit(2)
	}
	server, err := config.Server()
	if err != nil {
		config.Logger.Error("failed to start server", slog.Any("error", err))
		os.Exit(3)
	}
	config.Logger.Info("server stopped", slog.Any("exit", server.Serve(config.Listen)))
}
