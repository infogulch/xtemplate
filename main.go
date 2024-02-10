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
	config         kvflags
}

func parseflags() (f flags) {
	flag.StringVar(&f.listen_addr, "listen", "0.0.0.0:8080", "Listen address")
	flag.StringVar(&f.template_root, "template-root", "templates", "Template root directory")
	flag.StringVar(&f.context_root, "context-root", "", "Context root directory")
	flag.BoolVar(&f.watch_template, "watch-template", true, "Watch the template directory and reload if changed")
	flag.BoolVar(&f.watch_context, "watch-context", false, "Watch the context directory and reload if changed")
	flag.StringVar(&f.l_delim, "ldelim", "{{", "Left template delimiter")
	flag.StringVar(&f.r_delim, "rdelim", "}}", "Right template delimiter")
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

// Main can be called from your func main() if you want your program to act like
// the default xtemplate cli, or use it as a reference for making your own.
// Provide configs to override the defaults like: `xtemplate.Main(xtemplate.New().WithFooConfig())`
func Main(userConfig ...*config) {
	flags := parseflags()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(flags.log_level)}))

	configs := New()
	configs.WithDelims(flags.l_delim, flags.r_delim)
	configs.WithLogger(log.WithGroup("xtemplate"))

	if flags.db_driver != "" {
		db, err := sql.Open(flags.db_driver, flags.db_connstr)
		if err != nil {
			log.Error("failed to open db", "error", err)
			os.Exit(1)
		}
		configs.WithDB(db)
	}

	if flags.context_root != "" {
		configs.WithContextFS(os.DirFS(flags.context_root))
	}

	{
		config := make(map[string]string)
		for _, kv := range flags.config {
			config[kv.Key] = kv.Value
		}
		if len(config) > 0 {
			configs.WithConfig(config)
		}
	}

	for _, c := range userConfig {
		*configs = append(*configs, *c...)
	}

	handler, err := configs.Build()
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
				temphandler, err := configs.Build()
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
	fmt.Printf("server stopped: %v\n", http.ListenAndServe(flags.listen_addr, h))
}

type kv struct{ Key, Value string }

func (entry kv) String() string { return entry.Key + "=" + entry.Value }

func (entry *kv) Set(arg string) error {
	s := strings.SplitN(arg, "=", 2)
	if len(s) != 2 {
		return fmt.Errorf("config arg must be in the form `k=v`, got: `%s`", arg)
	}
	*entry = kv{Key: s[0], Value: s[1]}
	return nil
}

type kvflags []kv

func (s *kvflags) String() string {
	if s == nil {
		return ""
	}
	r := fmt.Sprint(*s)
	return r[1 : len(r)-1]
}

func (s *kvflags) Set(arg string) error {
	var entry kv
	if err := entry.Set(arg); err != nil {
		return err
	}
	*s = append(*s, entry)
	return nil
}
