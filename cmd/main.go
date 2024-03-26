// Default CLI package. To customize, copy this file to a new unique package and
// import dbs and provide config overrides.
package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/infogulch/xtemplate"

	"github.com/alexflint/go-arg"
	"github.com/infogulch/watch"

	_ "github.com/infogulch/xtemplate/providers"
	_ "modernc.org/sqlite"
)

func main() {
	Main()
}

// Main can be called from your func main() if you want your program to act like
// the default xtemplate cli, or use it as a reference for making your own.
// Provide configs to override the defaults like: `xtemplate.Main(xtemplate.WithFooConfig())`
func Main(overrides ...xtemplate.ConfigOverride) {
	var args struct {
		xtemplate.Config `arg:"-w"`
		Watch            []string
		WatchTemplates   bool   `default:"true"`
		Listen           string `arg:"-l" default:"0.0.0.0:8080"`
		LogLevel         int    `default:"-2"`
	}
	arg.MustParse(&args)
	args.Defaults()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(args.LogLevel)}))
	args.Config.Logger = log

	for _, o := range overrides {
		o(&args.Config)
	}

	server, err := args.Config.Server()
	if err != nil {
		log.Error("failed to load xtemplate", slog.Any("error", err))
		os.Exit(2)
	}

	if args.WatchTemplates {
		args.Watch = append(args.Watch, args.Config.TemplatesDir)
	}
	if len(args.Watch) != 0 {
		_, err := watch.Watch(args.Watch, 200*time.Millisecond, log.WithGroup("fswatch"), func() bool {
			server.Reload()
			return true
		})
		if err != nil {
			log.Info("failed to watch directories", slog.Any("error", err), slog.Any("directories", args.Watch))
			os.Exit(4)
		}
	}

	log.Info("server stopped", slog.Any("exit", server.Serve(args.Listen)))
}
