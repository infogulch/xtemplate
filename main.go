package xtemplate

import (
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/infogulch/watch"
)

type flags struct {
	config              Config
	listen_addr         string
	watch_template_path bool
	watch_context_path  bool
}

func parseflags() (f flags) {
	flag.StringVar(&f.listen_addr, "listen", "0.0.0.0:8080", "Listen address")
	flag.StringVar(&f.config.Template.Path, "template-path", "templates", "Directory where templates are loaded from")
	flag.BoolVar(&f.watch_template_path, "watch-template", true, "Watch the template directory and reload if changed")
	flag.StringVar(&f.config.Template.TemplateExtension, "template-extension", ".html", "File extension to look for to identify templates")
	flag.StringVar(&f.config.Template.Delimiters.Left, "ldelim", "{{", "Left template delimiter")
	flag.StringVar(&f.config.Template.Delimiters.Right, "rdelim", "}}", "Right template delimiter")

	flag.StringVar(&f.config.Context.Path, "context-path", "", "Directory that template definitions are given direct access to. No access is given if empty (default \"\")")
	flag.BoolVar(&f.watch_context_path, "watch-context", false, "Watch the context directory and reload if changed (default false)")

	flag.StringVar(&f.config.Database.Driver, "db-driver", "", "Name of the database driver registered as a Go `sql.Driver`. Not available if empty. (default \"\")")
	flag.StringVar(&f.config.Database.Connstr, "db-connstr", "", "Database connection string")

	flag.Var(&f.config.UserConfig, "c", "Config values, in the form `x=y`, can be specified multiple times")

	flag.IntVar(&f.config.LogLevel, "log", 0, "Log level, DEBUG=-4, INFO=0, WARN=4, ERROR=8")
	flag.Parse()
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "xtemplate is a hypertext preprocessor and http templating web server.\nUsage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}
	sql.Drivers()
	return
}

// Main can be called from your func main() if you want your program to act like
// the default xtemplate cli, or use it as a reference for making your own.
// Provide configs to override the defaults like: `xtemplate.Main(xtemplate.WithFooConfig())`
func Main(overrides ...override) {
	flags := parseflags()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(flags.config.LogLevel)}))
	WithLogger(log)(&flags.config)
	for _, o := range overrides {
		o(&flags.config)
	}
	handler, err := Build(&flags.config)
	if err != nil {
		log.Error("failed to load xtemplate", "error", err)
		os.Exit(2)
	}

	// set up fswatch
	{
		var watchDirs []string
		if flags.watch_template_path {
			watchDirs = append(watchDirs, flags.config.Template.Path)
		}
		if flags.watch_context_path {
			if flags.config.Context.Path == "" {
				log.Error("cannot watch context root if it is not specified", "context_root", flags.config.Context.Path)
				os.Exit(3)
			}
			watchDirs = append(watchDirs, flags.config.Context.Path)
		}
		if len(watchDirs) != 0 {
			changed, halt, err := watch.WatchDirs(watchDirs, 200*time.Millisecond)
			if err != nil {
				slog.Info("failed to watch directories", "error", err, "directories", watchDirs)
				os.Exit(4)
			}
			watch.React(changed, halt, func() (halt bool) {
				temphandler, err := Build(&flags.config)
				if err != nil {
					log.Info("failed to reload xtemplate", "error", err)
				} else {
					handler, temphandler = temphandler, handler
					temphandler.Cancel()
					log.Info("reloaded templates after file changed")
				}
				return
			})
		}
	}

	log.Info("serving", "address", flags.listen_addr)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r) // wrap handler so it can be changed when reloaded with fswatch
	})

	values := []any{}
	if err := http.ListenAndServe(flags.listen_addr, h); err != nil {
		values = append(values, slog.Any("error", err))
	}
	log.Info("server stopped", values...)
}

// Implement flag.Value.String to use UserConfigs as a flag.
func (c UserConfig) String() string {
	out := ""
	first := true
	for k, v := range c {
		if !first {
			out += " "
		}
		out += k + "=" + v
	}
	return out
}

// Implement flag.Value.Set to use UserConfigs as a flag.
func (c *UserConfig) Set(arg string) error {
	s := strings.SplitN(arg, "=", 2)
	if len(s) != 2 {
		return fmt.Errorf("config arg must be in the form `k=v`, got: `%s`", arg)
	}
	if previous, ok := (*c)[s[0]]; ok {
		return fmt.Errorf("cannot overwrite key in user config. current value for key `%s` is `%s`. attempted to set to `%s`", s[0], previous, s[1])
	}
	(*c)[s[0]] = s[1]
	return nil
}
