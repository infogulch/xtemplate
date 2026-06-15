package app

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"time"

	"github.com/infogulch/xtemplate"

	"github.com/alexflint/go-arg"
	"github.com/infogulch/watch"
)

type Args struct {
	xtemplate.Config
	Watch          []string `json:"watch_dirs" arg:",separate"`
	WatchTemplates bool     `json:"watch_templates"`
	Listen         string   `json:"listen" arg:"-l"`
	LogLevel       int      `json:"log_level" default:"-2"`
	Configs        []string `json:"-" arg:"-c,--config,separate"`
	ConfigFiles    []string `json:"-" arg:"-f,--config-file,separate"`
}

func (Args) Epilogue() string {
	return `Examples:
    Listen on port 80:
    $ ./xtemplate --listen :80

    Specify a context directory and reload when it changes:
    $ ./xtemplate --template-dir public --watch-templates

    Parse template files matching a custom extension and minify them:
    $ ./xtemplate --template-ext ".go.html" --minify`
}

// mergeConfig resolves the final configuration by applying JSON sources named by
// the already-parsed CLI flags onto a fresh defaults base, then re-applying the
// CLI args so flags take precedence over JSON. The resulting precedence is
// CLI flags > JSON (--config-file files, then --config values) > defaults.
//
// readFile loads --config-file contents (injectable for testing); it may be nil
// when there are no config files to read.
func mergeConfig(argv []string, cli Args, readFile func(string) ([]byte, error)) (Args, error) {
	jsonConfig := defaultArgs
	decoded := false
	for _, name := range cli.ConfigFiles {
		data, err := readFile(name)
		if err != nil {
			return Args{}, fmt.Errorf("failed to read config file %q: %w", name, err)
		}
		if err := json.Unmarshal(data, &jsonConfig); err != nil {
			return Args{}, fmt.Errorf("failed to decode config file %q: %w", name, err)
		}
		decoded = true
	}
	for _, conf := range cli.Configs {
		if err := json.Unmarshal([]byte(conf), &jsonConfig); err != nil {
			return Args{}, fmt.Errorf("failed to decode --config value: %w", err)
		}
		decoded = true
	}
	if !decoded {
		return cli, nil
	}

	// Re-apply the CLI flags over the JSON-derived config. go-arg treats the
	// nonzero fields already present in jsonConfig as defaults, so JSON values
	// survive unless a corresponding flag was passed.
	p, err := arg.NewParser(arg.Config{}, &jsonConfig)
	if err != nil {
		return Args{}, err
	}
	if err := p.Parse(argv); err != nil {
		return Args{}, fmt.Errorf("failed to re-apply cli flags over json config: %w", err)
	}
	return jsonConfig, nil
}

// version is stamped at build time for releases via
// -ldflags="-X 'github.com/infogulch/xtemplate/app.version=v1.2.3'". When unset
// (e.g. `go install ...@version` or a plain `go build`), Version() falls back to
// the module/VCS info embedded by the Go toolchain.
var version = ""

func (Args) Version() string {
	if version != "" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "development"
	}
	// Set for module-based installs, e.g. `go install ...@v0.8.4`.
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	// Otherwise derive from the VCS info the toolchain stamps into the binary.
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

var defaultWatchTemplates = "true"
var defaultListenAddress = "0.0.0.0:8080"
var defaultArgs = Args{WatchTemplates: defaultWatchTemplates == "true", Listen: defaultListenAddress}

// Main can be called from your func main() if you want your program to act like
// the default xtemplate cli, or use it as a reference for making your own.
// Provide configs to override the defaults like:
//
//	app.Main(xtemplate.WithFooConfig())
func Main(overrides ...xtemplate.Option) {
	config := defaultArgs
	var log *slog.Logger

	// Configuration precedence, highest priority first:
	//
	//  1. CLI flags
	//  2. JSON from --config values and --config-file files (later sources
	//     override earlier ones; files are applied before inline values)
	//  3. built-in defaults (defaultArgs + struct `default` tags)
	//
	// This is implemented with a two-pass parse: parse the CLI once to discover
	// which config files/values to load, decode those onto a fresh defaults base,
	// then re-parse the CLI over the decoded result so flags win over JSON.
	{
		arg.MustParse(&config)
		config.Defaults()

		level := config.LogLevel
		log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(level)}))

		merged, err := mergeConfig(os.Args[1:], config, os.ReadFile)
		if err != nil {
			log.Error("failed to load configuration", slog.Any("error", err))
			os.Exit(1)
		}
		config = merged

		if config.LogLevel != level {
			log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(config.LogLevel)}))
		}

		config.Logger = log

		log.Debug("loaded configuration", slog.Any("config", &config))
	}

	server, err := config.Server(overrides...)
	if err != nil {
		log.Error("failed to load xtemplate", slog.Any("error", err))
		os.Exit(2)
	}

	if config.WatchTemplates && config.TemplatesFS == nil {
		config.Watch = append(config.Watch, config.TemplatesDir)
	}
	if len(config.Watch) != 0 {
		_, err := watch.Watch(config.Watch, 200*time.Millisecond, log.WithGroup("fswatch"), func() bool {
			_ = server.Reload()
			return true
		})
		if err != nil {
			log.Info("failed to watch directories", slog.Any("error", err), slog.Any("directories", config.Watch))
			os.Exit(4)
		}
	}

	log.Info("server stopped", slog.Any("exit", server.Serve(config.Listen)))
}
