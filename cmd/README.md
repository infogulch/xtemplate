This package is a tiny wrapper around [`xtemplate.Main()`](../main.go) which is
the only code in the package.

If you want to customize the xtemplate build with a specific database driver,
custom template funcs, or to have more control over application startup, this is
a good place to start looking.

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

```
$ ./xtemplate --help
xtemplate is a hypertext preprocessor and http templating web server

  -listen string
        Listen address (default "0.0.0.0:8080")

  -template-path string
        Directory where templates are loaded from (default "templates")
  -watch-template
        Watch the template directory and reload if changed (default true)
  -template-extension
        File extension to look for to identify templates (default ".html")
  -ldelim string
        Left template delimiter (default "{{")
  -rdelim string
        Right template delimiter (default "}}")

  -context-path string
        Directory that template definitions are given direct access to. No access is given if empty (default "")
  -watch-context
        Watch the context directory and reload if changed (default false)

  -db-driver string
        Name of the database driver registered as a Go `sql.Driver`. Not available if empty. (default "")
  -db-connstr string
        Database connection string

  -c string
        Config values, in the form `x=y`. This arg can be specified multiple times

  -log int
        Log level, DEBUG=-4, INFO=0, WARN=4, ERROR=8
  -help
        Display help
```

### Example

```
xtemplate -template-path my-templates-folder/ -watch-template -log -4
```
