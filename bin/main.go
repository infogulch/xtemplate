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
	config         config
}

func parseflags() (f flags) {
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
	flag.Var(&f.config, "c", "Config values, in the form `x=y`, can be specified multiple times")
	flag.Parse()
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "xtemplate is a hypertext preprocessor and http templating web server.\nUsage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}
	return
}

func main() {
	flags := parseflags()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(flags.log_level)}))

	var err error
	var db *sql.DB
	var contextfs fs.FS
	var config map[string]string

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

	for _, kv := range flags.config {
		config[kv.Key] = kv.Value
	}

	x := xtemplate.XTemplate{
		TemplateFS: os.DirFS(flags.template_root),
		ContextFS:  contextfs,
		// ExtraFuncs
		Config: config,
		Delims: struct{ L, R string }{L: flags.l_delim, R: flags.r_delim},
		DB:     db,
		Log:    log.WithGroup("xtemplate"),
	}
	err = x.Reload()
	if err != nil {
		log.Error("failed to load xtemplate", "error", err)
		os.Exit(2)
	}

	// set up fswatch
	{
		var watchDirs []string
		if flags.watch_template {
			watchDirs = append(watchDirs, flags.template_root)
		}
		if flags.watch_context {
			if flags.context_root == "" {
				log.Error("cannot watch context root if it is not specified", "context_root", flags.context_root)
				os.Exit(3)
			}
			watchDirs = append(watchDirs, flags.context_root)
		}
		if len(watchDirs) != 0 {
			changed, halt, err := watch.WatchDirs(watchDirs, 200*time.Millisecond)
			if err != nil {
				slog.Info("failed to watch directories", "error", err, "directories", watchDirs)
				os.Exit(4)
			}
			watch.React(changed, halt, func() (halt bool) {
				err := x.Reload()
				if err != nil {
					log.Info("failed to reload xtemplate", "error", err)
				} else {
					log.Info("reloaded templates after file changed")
				}
				return
			})
		}
	}

	log.Info("serving", "address", flags.listen_addr)
	fmt.Printf("server stopped: %v\n", http.ListenAndServe(flags.listen_addr, &x))
}

type kv struct{ Key, Value string }

func (kv kv) String() string { return kv.Key + "=" + kv.Value }

type config []kv

func (c *config) String() string {
	if c == nil {
		return ""
	}
	s := fmt.Sprint(*c)
	return s[1 : len(s)-1]
}

func (c *config) Set(arg string) error {
	s := strings.SplitN(arg, "=", 2)
	if len(s) != 2 {
		return fmt.Errorf("config arg must be in the form `k=v`, got: `%s`", arg)
	}
	*c = append(*c, kv{Key: s[0], Value: s[1]})
	return nil
}
