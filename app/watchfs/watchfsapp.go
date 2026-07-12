// watchfs reloads the server upon changes to the templates directory or other
// configured directories. This is xtemplate's default app.
package watchfs

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/infogulch/watch"
	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/app"
)

type Config struct {
	app.Config

	// Watch lists extra directories to watch for changes, in addition to the
	// templates directory.
	Watch []string `json:"watch_dirs" arg:",separate"`
}

func (c *Config) Epilogue() string {
	return c.Config.Epilogue() + fmt.Sprintf(`

    Watch extra directories and reduce log verbosity:
    ❯ %[1]s --watch data --loglevel -4`, os.Args[0])
}

var _ app.Configurable = (*Config)(nil)

func Main(options ...xtemplate.Option) {
	config, err := app.LoadConfig(&Config{}, nil)
	if err != nil {
		config.Logger.Info("failed to load config", slog.Any("error", err))
		os.Exit(1)
	}
	err = setReload(config)
	if err != nil {
		config.Logger.Info("failed to watch directories", slog.Any("error", err), slog.Any("directories", config.Watch))
		os.Exit(4)
	}
	app.Serve(&config.Config, options...)
}

func setReload(config *Config) error {
	config.Watch = append(config.Watch, config.TemplatesDir)

	watchCh := make(chan []xtemplate.Option)
	_, err := watch.Watch(config.Watch, 200*time.Millisecond, config.Logger.WithGroup("fswatch"), func() bool {
		watchCh <- nil
		return true
	})
	if err != nil {
		return err
	}
	config.Reload = watchCh
	return nil
}
