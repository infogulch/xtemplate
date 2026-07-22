package app

import (
	"encoding/json"
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
	SourceType  string   `json:"-" arg:"--source-type"`
}

// UnmarshalJSON fills app + embedded xtemplate fields without invoking
// [xtemplate.Config.UnmarshalJSON] on the whole object (that would drop listen).
// Callers that need the legacy-key ban-list should run CheckLegacyTemplateKeys
// first (LoadConfig does).
func (a *Config) UnmarshalJSON(data []byte) error {
	type plainXT xtemplate.Config
	type alias struct {
		plainXT
		Listen   string `json:"listen"`
		LogLevel int    `json:"log_level"`
	}
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.Config = xtemplate.Config(raw.plainXT)
	a.Listen = raw.Listen
	a.LogLevel = raw.LogLevel
	return nil
}

// defaultListenAddress and defaultSourceType allow build-time overrides:
//
//	-ldflags="-X 'github.com/infogulch/xtemplate/app.defaultListenAddress=0.0.0.0:80'"
//	-ldflags="-X 'github.com/infogulch/xtemplate/app.defaultSourceType=os'"
//
// Docker builds set listen :80 and source type os. Release cmd/xtemplate leaves
// defaultSourceType as watchfs.
var (
	defaultListenAddress = "0.0.0.0:8080"
	defaultSourceType    = "watchfs"
)

// SetDefaults sets the default values for this Config.
func (a *Config) SetDefaults() {
	a.Listen = defaultListenAddress
	a.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(a.LogLevel)}))
	a.Config.SetDefaults()
}

// Epilogue is shown at the end of --help.
func (Config) Epilogue() string {
	srcList := strings.Join(xtemplate.RegisteredSourceTypes(), ", ")
	if srcList == "" {
		srcList = "(none registered)"
	}
	return fmt.Sprintf(`Source types (this build): %s
Default --source-type: %s

Examples:
    Listen on port 80:
    ❯ %[3]s --listen :80

    Specify a template directory (os/watchfs/git path):
    ❯ %[3]s --templates-dir public

    Use git source:
    ❯ %[3]s --source-type git --git-repo https://example.com/site.git

    Parse template files matching a custom extension; disable minify:
    ❯ %[3]s --template-ext ".go.html" --minify=false`, srcList, defaultSourceType, os.Args[0])
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

// pass0 holds argv scan results before full go-arg parsing.
type pass0 struct {
	sourceType  string
	configFiles []string
	configs     []string
	wantHelp    bool
	wantVersion bool
}

// scanPass0 scans argv for --source-type, -f/--config-file, -c/--config, and
// help/version without full go-arg on source structs.
func scanPass0(args []string) (pass0, error) {
	var p pass0
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			p.wantHelp = true
		case a == "--version":
			p.wantVersion = true
		case a == "--source-type":
			if i+1 >= len(args) {
				return p, fmt.Errorf("--source-type requires a value")
			}
			i++
			p.sourceType = args[i]
		case strings.HasPrefix(a, "--source-type="):
			p.sourceType = strings.TrimPrefix(a, "--source-type=")
		case a == "-f" || a == "--config-file":
			if i+1 >= len(args) {
				return p, fmt.Errorf("%s requires a value", a)
			}
			i++
			p.configFiles = append(p.configFiles, args[i])
		case strings.HasPrefix(a, "--config-file="):
			p.configFiles = append(p.configFiles, strings.TrimPrefix(a, "--config-file="))
		case a == "-c" || a == "--config":
			if i+1 >= len(args) {
				return p, fmt.Errorf("%s requires a value", a)
			}
			i++
			p.configs = append(p.configs, args[i])
		case strings.HasPrefix(a, "--config="):
			p.configs = append(p.configs, strings.TrimPrefix(a, "--config="))
		}
	}
	return p, nil
}

// peekSourceType reads "source"."type" from raw JSON without full Config decode.
// Caller is responsible for CheckLegacyTemplateKeys before this when needed.
func peekSourceType(data []byte) (string, error) {
	var probe struct {
		Source *struct {
			Type string `json:"type"`
		} `json:"source"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return "", err
	}
	if probe.Source == nil {
		return "", nil
	}
	return probe.Source.Type, nil
}

// LoadConfig loads the app configuration, merging sources in priority order:
//
//	CLI flags > JSON cli > JSON file > defaults
//
// Pass 0 scans argv for --source-type and config paths, loads JSON (with ban-list
// probe), picks the effective source type, then Pass B parses CLI flags into the
// app config and the effective source dest. After load, Source is materialized
// and SourceRaw is cleared.
//
// Pass nil args to use os.Args[1:].
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

	// Help/version: parse app flags only so go-arg can exit with help text.
	if p0.wantHelp || p0.wantVersion {
		src, _ := xtemplate.NewSource(defaultSourceType)
		if src == nil {
			src = &xtemplate.OsFsSource{}
		}
		parser, err := arg.NewParser(arg.Config{}, config, src)
		if err != nil {
			return config, err
		}
		parser.MustParse(args)
		return config, nil
	}

	// Load JSON (files then inline); later wins. Track source type from JSON.
	var jsonSourceType string
	applyJSON := func(data []byte, origin string) error {
		if err := xtemplate.CheckLegacyTemplateKeys(data); err != nil {
			return fmt.Errorf("%s: %w", origin, err)
		}
		st, err := peekSourceType(data)
		if err != nil {
			return fmt.Errorf("%s: %w", origin, err)
		}
		if st != "" {
			jsonSourceType = st
		}
		if err := json.Unmarshal(data, config); err != nil {
			return fmt.Errorf("%s: %w", origin, err)
		}
		return nil
	}
	for _, name := range p0.configFiles {
		data, err := os.ReadFile(name)
		if err != nil {
			return config, fmt.Errorf("failed to read config file %q: %w", name, err)
		}
		if err := applyJSON(data, fmt.Sprintf("config file %q", name)); err != nil {
			return config, err
		}
	}
	for _, conf := range p0.configs {
		if err := applyJSON([]byte(conf), "--config"); err != nil {
			return config, err
		}
	}

	// Effective type: JSON source.type if set, else --source-type, else default.
	effectiveType := defaultSourceType
	if jsonSourceType != "" {
		effectiveType = jsonSourceType
	}
	if p0.sourceType != "" {
		if jsonSourceType != "" && p0.sourceType != jsonSourceType {
			return config, fmt.Errorf("xtemplate: --source-type %q does not match JSON source.type %q", p0.sourceType, jsonSourceType)
		}
		effectiveType = p0.sourceType
	}

	source, err := xtemplate.NewSource(effectiveType)
	if err != nil {
		if p0.sourceType == "" && jsonSourceType == "" {
			// Build default (e.g. release "watchfs") was not blank-imported in
			// this binary (custom Main / embedded examples). Fall back to os so
			// WithTemplateFS overrides and plain disk templates still work.
			source = &xtemplate.OsFsSource{}
			effectiveType = "os"
		} else {
			// Explicit --source-type or JSON source.type that is not linked.
			return config, err
		}
	}

	// Materialize Source from SourceRaw if present, then clear Raw.
	if len(config.SourceRaw) > 0 {
		s, err := xtemplate.ResolveSource(config.SourceRaw)
		if err != nil {
			return config, err
		}
		source = s
		config.SourceRaw = nil
	}

	// Pass B: CLI wins over JSON for app + source fields.
	parser, err := arg.NewParser(arg.Config{}, config, source)
	if err != nil {
		return config, err
	}
	if err := parser.Parse(args); err != nil {
		return config, fmt.Errorf("failed to parse cli flags: %w", err)
	}

	config.Source = source
	config.SourceRaw = nil
	config.SourceType = effectiveType

	// Rebuild logger after flags/JSON so --loglevel applies.
	config.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(config.LogLevel)}))
	config.Logger.Debug("loaded configuration", slog.String("source_type", effectiveType), slog.Any("listen", config.Listen))
	return config, nil
}

// Serve sets up the xtemplate server from config and serves it.
// Serve blocks until the server stops.
func Serve(config *Config, overrides ...xtemplate.Option) {
	server, err := config.Server(overrides...)
	if err != nil {
		config.Logger.Error("failed to start server", slog.Any("error", err))
		os.Exit(2)
	}
	config.Logger.Info("server stopped", slog.Any("exit", server.Serve(config.Listen)))
}
