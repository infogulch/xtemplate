package xtemplate

import (
	"flag"
	"fmt"
	"log/slog"
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

var helptext = `xtemplate is a hypertext preprocessor and html templating http server

Usage: %[1]s [options]

Options:
  -listen string              Listen address (default "0.0.0.0:8080")

  -template-path string       Directory where templates are loaded from (default "templates")
  -watch-template bool        Watch the template directory and reload if changed (default true)
  -template-extension string  File extension to look for to identify templates (default ".html")
  -minify bool                Preprocess the template files to minimize their size at load time (default false)
  -ldelim string              Left template delimiter (default "{{")
  -rdelim string              Right template delimiter (default "}}")

  -context-path string        Directory that template definitions are given direct access to. No access is given if empty (default "")
  -watch-context bool         Watch the context directory and reload if changed (default false)

  -db-driver string           Name of the database driver registered as a Go 'sql.Driver'. Not available if empty. (default "")
  -db-connstr string          Database connection string

  -c string                   Config values, in the form 'x=y'. Can be used multiple times

  -log int                    Log level. Log statements below this value are omitted from log output, DEBUG=-4, INFO=0, WARN=4, ERROR=8 (Default: 0)
  -help                       Display help

Examples:
    Listen on port 80:
    $ %[1]s -listen :80

    Specify a context directory and reload when it changes:
    $ %[1]s -context-path context/ -watch-context

    Parse template files matching a custom extension and minify them:
    $ %[1]s -template-extension ".go.html" -minify

    Open the specified db and makes it available to template files as '.DB':
    $ %[1]s -db-driver sqlite3 -db-connstr 'file:rss.sqlite?_journal=WAL'
`

func parseflags() (f flags) {
	f.config = *New()
	flag.StringVar(&f.listen_addr, "listen", "0.0.0.0:8080", "Listen address")
	flag.StringVar(&f.config.Template.Path, "template-path", "templates", "Directory where templates are loaded from")
	flag.BoolVar(&f.watch_template_path, "watch-template", true, "Watch the template directory and reload if changed")
	flag.StringVar(&f.config.Template.TemplateExtension, "template-extension", ".html", "File extension to look for to identify templates")
	flag.BoolVar(&f.config.Template.Minify, "minify", false, "Preprocess the template files to minimize their size at load time")
	flag.StringVar(&f.config.Template.Delimiters.Left, "ldelim", "{{", "Left template delimiter")
	flag.StringVar(&f.config.Template.Delimiters.Right, "rdelim", "}}", "Right template delimiter")

	flag.StringVar(&f.config.Context.Path, "context-path", "", "Directory that template definitions are given direct access to. No access is given if empty (default \"\")")
	flag.BoolVar(&f.watch_context_path, "watch-context", false, "Watch the context directory and reload if changed (default false)")

	flag.StringVar(&f.config.Database.Driver, "db-driver", "", "Name of the database driver registered as a Go 'sql.Driver'. Not available if empty. (default \"\")")
	flag.StringVar(&f.config.Database.Connstr, "db-connstr", "", "Database connection string")

	flag.Var(&f.config.UserConfig, "c", "Config values, in the form 'x=y', can be specified multiple times")

	flag.IntVar(&f.config.LogLevel, "log", 0, "Log level, DEBUG=-4, INFO=0, WARN=4, ERROR=8")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), helptext, os.Args[0])
	}

	flag.Parse()
	return
}

// Main can be called from your func main() if you want your program to act like
// the default xtemplate cli, or use it as a reference for making your own.
// Provide configs to override the defaults like: `xtemplate.Main(xtemplate.WithFooConfig())`
func Main(overrides ...ConfigOverride) {
	flags := parseflags()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(flags.config.LogLevel)}))
	flags.config.Logger = log
	for _, o := range overrides {
		o(&flags.config)
	}

	server, err := flags.config.Server()
	if err != nil {
		log.Error("failed to load xtemplate", slog.Any("error", err))
		os.Exit(2)
	}

	// set up fswatch
	{
		var dirs []string
		if flags.watch_template_path {
			dirs = append(dirs, flags.config.Template.Path)
		}
		if flags.watch_context_path {
			dirs = append(dirs, flags.config.Context.Path)
		}
		if len(dirs) != 0 {
			_, err := watch.Watch(dirs, 200*time.Millisecond, log.WithGroup("fswatch"), func() bool {
				server.Reload()
				return true
			})
			if err != nil {
				log.Info("failed to watch directories", slog.Any("error", err), slog.Any("directories", dirs))
				os.Exit(4)
			}
		}
	}

	log.Info("server stopped", server.Serve(flags.listen_addr))
}

// Implement flag.Value.String to use UserConfigs as a flag.
func (c TemplateConfig) String() string {
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
func (c *TemplateConfig) Set(arg string) error {
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
