This package is a tiny wrapper around [`xtemplate.Main()`](../main.go) which is
the only code in the package.

If you want to customize the xtemplate build with a specific database driver,
custom template funcs, or to have more control over application startup copy
[main.go](main.go) to a new package and provide override funcs to call to
`xtemplate.Main()`.

### Build

```sh
# build from ./cmd
go build -o xtemplate

# build from repo root
go build -o xtemplate ./cmd

# build with sqlite3 driver and json extensions
GOFLAGS='-tags="sqlite_json"' CGO_ENABLED=1 go build -o xtemplate ./cmd
```

### Usage

```shell
$ ./xtemplate --help
xtemplate is a hypertext preprocessor and html templating http server

Usage: ./xtemplate [options]

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
    $ ./xtemplate -listen :80

    Specify a context directory and reload when it changes:
    $ ./xtemplate -context-path context/ -watch-context

    Parse template files matching a custom extension and minify them:
    $ ./xtemplate -template-extension ".go.html" -minify

    Open the specified db and makes it available to template files as '.DB':
    $ ./xtemplate -db-driver sqlite3 -db-connstr 'file:rss.sqlite?_journal=WAL'
```
