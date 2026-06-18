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

type Config struct {
	xtemplate.Config
	Watch          []string `json:"watch_dirs" arg:",separate"`
	WatchTemplates bool     `json:"watch_templates"`
	Listen         string   `json:"listen" arg:"-l"`
	LogLevel       int      `json:"log_level" default:"-2"`
	Configs        []string `json:"-" arg:"-c,--config,separate"`
	ConfigFiles    []string `json:"-" arg:"-f,--config-file,separate"`
}

var _ Configurable = (*Config)(nil)

func (a *Config) appconfig() *Config { return a }

// these allow for build-time overrides with:
//
//	-ldflags="-X 'github.com/infogulch/xtemplate/app.defaultWatchTemplates=false'"
//
// Used by the default docker build to adjust xtemplate's defaults to better
// suit to that environment.
var defaultWatchTemplates = "true"
var defaultListenAddress = "0.0.0.0:8080"

// SetDefaults sets the default values for this Config.
func (a *Config) SetDefaults() {
	a.WatchTemplates = defaultWatchTemplates == "true"
	a.Listen = defaultListenAddress
	a.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(a.LogLevel)}))
	a.Config.SetDefaults()
}

// Epilogue is called by arg when the user requests help via the cli. Can be
// overridden by a Configurable implementation.
func (Config) Epilogue() string {
	arg0 := os.Args[0]
	return fmt.Sprintf(`Examples:
    Listen on port 80:
    ❯ %[1]s --listen :80

    Specify a template directory and reload when it changes:
    ❯ %[1]s --template-dir public --watch-templates

    Parse template files matching a custom extension and minify them:
    ❯ %[1]s --template-ext ".go.html" --minify`, arg0)
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

// Main can be called from your func main() if you want your program to act like
// the default xtemplate cli, or use it as a reference for making your own.
// Provide config options to override the defaults like:
//
//	app.Main(xtemplate.WithFooConfig())
func Main(overrides ...xtemplate.Option) {
	config, err := LoadConfig(&Config{}, nil)
	if err == arg.ErrHelp || err == arg.ErrVersion {
		os.Exit(0)
	}
	if err != nil {
		config.appconfig().Logger.Error("failed to load configuration", slog.Any("error", err))
		os.Exit(1)
	}
	Serve(config, overrides...)
}

// Configurable is satisfied by *Config and by a pointer to any struct that embeds
// Config and implements New. Implementers may implement SetDefaults and Epilogue
// at their discretion, since the embedded Config implements them natively.
type Configurable interface {
	appconfig() *Config

	// SetDefaults can be overridden to provide custom default values
	// configuration defined by the Configurable
	SetDefaults()

	// Epilogue can be overridden to provide a custom epilogue message for help
	// output. Consider combining your custom epilogue with the embedded Config's
	// Epilogue.
	Epilogue() string
}

// LoadConfig loads the app configuration, merging sources in priority order:
//
//	CLI flags > JSON cli > JSON file > defaults
//
// This function will load configuration into any struct implementing the
// Configurable interface. To get the merged configuration, call this function
// with a pointer to the config struct. Pass nil `args` to use the default os.Args.
//
// Note: to give CLI args precedence over JSON sources, CLI args are parsed twice:
// first to discover which config files to load, then again at the end to override any
// values set by json sources.
func LoadConfig[T Configurable](config T, args []string) (T, error) {
	config.appconfig().SetDefaults()
	config.SetDefaults()
	if args == nil {
		args = os.Args[1:]
	}

	// parse CLI args to discover config files/values to load
	{
		p, err := arg.NewParser(arg.Config{}, config)
		if err != nil {
			return config, err
		}
		// call MustParse to handle arg parse errors and version/help flags
		p.MustParse(args)
	}

	// parse json file/cli config
	appconfig := config.appconfig()
	for _, name := range appconfig.ConfigFiles {
		data, err := os.ReadFile(name)
		if err != nil {
			return config, fmt.Errorf("failed to read config file %q: %w", name, err)
		}
		if err := json.Unmarshal(data, config); err != nil {
			return config, fmt.Errorf("failed to decode config file %q: %w", name, err)
		}
	}
	for _, conf := range appconfig.Configs {
		if err := json.Unmarshal([]byte(conf), config); err != nil {
			return config, fmt.Errorf("failed to decode --config value: %w", err)
		}
	}

	// parse CLI args again to preserve the defined config precedence
	{
		p, err := arg.NewParser(arg.Config{}, config)
		if err != nil {
			return config, err
		}
		if err := p.Parse(args); err != nil {
			return config, fmt.Errorf("failed to parse cli flags: %w", err)
		}
	}

	appconfig.Logger.Debug("loaded configuration", slog.Any("config", config))
	return config, nil
}

// Serve builds the xtemplate server from config and serves it, owning the
// reload semantics. If config.Reload is already set it drives reloads (e.g. a
// remote source set by the caller); otherwise Serve falls back to watching
// local template directories with fsnotify. This call blocks until the server
// stops.
func Serve(configurable Configurable, overrides ...xtemplate.Option) {
	config := configurable.appconfig()

	if config.Reload == nil && config.WatchTemplates && config.TemplatesFS == nil {
		config.Watch = append(config.Watch, config.TemplatesDir)
	}

	if config.Reload == nil && len(config.Watch) != 0 {
		watchCh := make(chan []xtemplate.Option)
		_, err := watch.Watch(config.Watch, 200*time.Millisecond, config.Logger.WithGroup("fswatch"), func() bool {
			watchCh <- nil
			return true
		})
		if err != nil {
			config.Logger.Info("failed to watch directories", slog.Any("error", err), slog.Any("directories", config.Watch))
			os.Exit(4)
		}
		config.Reload = watchCh
	}

	server, err := config.Server(overrides...)
	if err != nil {
		config.Logger.Error("failed to start server", slog.Any("error", err))
		os.Exit(2)
	}
	config.Logger.Info("server stopped", slog.Any("exit", server.Serve(config.Listen)))
}
