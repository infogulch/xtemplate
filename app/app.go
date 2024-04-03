package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/infogulch/xtemplate"

	"github.com/alexflint/go-arg"
	"github.com/infogulch/watch"

	_ "github.com/infogulch/xtemplate/providers"
)

type Args struct {
	xtemplate.Config
	Watch          []string `json:"watch_dirs" arg:",separate"`
	WatchTemplates bool     `json:"watch_templates" default:"true"`
	Listen         string   `json:"listen" arg:"-l" default:"0.0.0.0:8080"`
	LogLevel       int      `json:"log_level" default:"-2"`
	Configs        []string `json:"-" arg:"-c,--config,separate"`
	ConfigFiles    []string `json:"-" arg:"-f,--config-file,separate"`
}

var version = "development"

func (Args) Version() string {
	return version
}

// Main can be called from your func main() if you want your program to act like
// the default xtemplate cli, or use it as a reference for making your own.
// Provide configs to override the defaults like:
//
//	app.Main(xtemplate.WithFooConfig())
func Main(overrides ...xtemplate.Option) {
	var config Args
	var log *slog.Logger

	{
		arg.MustParse(&config)
		config.Defaults()

		level := config.LogLevel
		log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(level)}))

		var jsonConfig Args
		var decoded bool
		for _, name := range config.ConfigFiles {
			func() {
				file, err := os.OpenFile(name, os.O_RDONLY, 0)
				if err != nil {
					log.Error("failed to open config file '%s': %w", name, err)
					os.Exit(1)
				}
				defer file.Close()
				err = json.NewDecoder(file).Decode(&jsonConfig)
				if err != nil {
					log.Error("failed to decode args from json file", slog.String("filename", name), slog.Any("error", err))
					os.Exit(1)
				}
				decoded = true
				log.Debug("incorporated json file", slog.String("filename", name), slog.Any("config", &jsonConfig))
			}() // use func to close file on every iteration
		}

		for _, conf := range config.Configs {
			err := json.NewDecoder(bytes.NewBuffer([]byte(conf))).Decode(&jsonConfig)
			if err != nil {
				log.Error("failed to decode arg from json flag", slog.Any("error", err))
				os.Exit(1)
			}
			decoded = true
			log.Debug("incorporated json value", slog.String("json_string", conf), slog.Any("config", &jsonConfig))
		}

		if decoded {
			arg.MustParse(&jsonConfig)
			config = jsonConfig
		}

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
			server.Reload()
			return true
		})
		if err != nil {
			log.Info("failed to watch directories", slog.Any("error", err), slog.Any("directories", config.Watch))
			os.Exit(4)
		}
	}

	log.Info("server stopped", slog.Any("exit", server.Serve(config.Listen)))
}
