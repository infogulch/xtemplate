package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/infogulch/watch"
	"github.com/infogulch/xtemplate"
)

type flags struct {
	help           bool
	listen_addr    string
	template_root  string
	context_root   string
	watch_template bool
	watch_context  bool
	l_delim        string
	r_delim        string
	db_driver      string
	db_connstr     string
	log_level      int
	configFlags    configFlags
}

func parseflags() (f flags) {
	flag.BoolVar(&f.help, "help", false, "Display help")
	flag.StringVar(&f.listen_addr, "listen", "0.0.0.0:8080", "Listen address")
	flag.StringVar(&f.template_root, "template-root", "templates", "Template root directory")
	flag.StringVar(&f.context_root, "context-root", "", "Context root directory")
	flag.BoolVar(&f.watch_template, "watch-template", true, "Watch the template directory and reload if changed")
	flag.BoolVar(&f.watch_context, "watch-context", false, "Watch the context directory and reload if changed")
	flag.StringVar(&f.l_delim, "ldelim", "{{", "Left template delimiter")
	flag.StringVar(&f.r_delim, "rdelim", "{{", "Right template delimiter")
	flag.StringVar(&f.db_driver, "db-driver", "", "Database driver name")
	flag.StringVar(&f.db_connstr, "db-connstr", "", "Database connection string")
	flag.IntVar(&f.log_level, "log", 0, "Log level, DEBUG=-4, INFO=0, WARN=4, ERROR=8")
	flag.Var(&f.configFlags, "c", "Config values, in the form `x=y`, can be specified multiple times")
	flag.Parse()
	if f.help {
		fmt.Printf("%s is a hypertext preprocessor and http templating web server\n\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(0)
	}
	return
}

func main() {
	flags := parseflags()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(flags.log_level)}))

	var err error
	var db *sql.DB
	var contextfs fs.FS

	if flags.db_driver != "" {
		db, err = sql.Open(flags.db_driver, flags.db_connstr)
		if err != nil {
			log.Error("failed to open db", "error", err)
			os.Exit(1)
		}
	}

	if flags.context_root != "" {
		contextfs = os.DirFS(flags.context_root)
	}

	x := xtemplate.XTemplate{
		TemplateFS: os.DirFS(flags.template_root),
		ContextFS:  contextfs,
		// ExtraFuncs
		Delims: struct{ L, R string }{L: flags.l_delim, R: flags.r_delim},
		DB:     db,
		Log:    log,
	}
	err = x.Reload()
	if err != nil {
		log.Error("failed to load xtemplate", "error", err)
		os.Exit(2)
	}

	var watch []string
	if flags.watch_template {
		watch = append(watch, flags.template_root)
	}
	if flags.watch_context {
		if flags.context_root == "" {
			log.Error("cannot watch context root if it is not specified", "context_root", flags.context_root)
			os.Exit(3)
		}
		watch = append(watch, flags.context_root)
	}
	if len(watch) != 0 {
		dowatch(watch, func() error { return x.Reload() }, log)
	}

	log.Info("serving", "address", flags.listen_addr)
	fmt.Println(http.ListenAndServe(flags.listen_addr, &x))
}

func dowatch(dirs []string, do func() error, log *slog.Logger) {
	changed, _, err := watch.WatchDirs([]string{"templates"}, 200*time.Millisecond, log)
	if err != nil {
		slog.Info("failed to watch directories", "error", err)
	}
	go func() {
		for {
			select {
			case _, ok := <-changed:
				if !ok {
					return
				}
				err := do()
				if err != nil {
					log.Info("failed to reload xtemplate", "error", err)
				} else {
					log.Info("reloaded templates after file changed")
				}
			}
		}
	}()
}

type configFlags []struct{ Key, Value string }

func (c *configFlags) String() string {
	if c == nil {
		return ""
	}
	s := new(strings.Builder)
	for i, f := range *c {
		if i > 0 {
			s.WriteRune(' ')
		}
		s.WriteString(f.Key)
		s.WriteRune('=')
		s.WriteString(f.Value)
	}
	return s.String()
}

func (c *configFlags) Set(arg string) error {
	s := strings.SplitN(arg, "=", 2)
	if len(s) != 2 {
		return fmt.Errorf("config arg must be in the form `x=y`, got: `%s`", arg)
	}
	*c = append(*c, struct{ Key, Value string }{Key: s[0], Value: s[1]})
	return nil
}
