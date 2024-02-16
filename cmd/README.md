This package simply calls [`xtemplate.Main()`](../main.go) in `func main()`
which exposes xtemplate's Go API into a CLI app. If you want a custom version of
the xtemplate cli with a specific database driver, custom template funcs, or
more control over application startup, this is a good place to start looking.

### Build

```sh
# build from ./cmd
go build -o xtemplate

# build from repo root
go build -o xtemplate ./cmd

# build with sqlite3 driver and json extensions
GOFLAGS='-tags="sqlite_json"' CGO_ENABLED=1 go build -o xtemplate
```

### Usage

```
$ ./xtemplate --help
xtemplate is a hypertext preprocessor and http templating web server

  -context-root string
        Context root directory
  -db-connstr string
        Database connection string
  -db-driver string
        Database driver name
  -help
        Display help
  -ldelim string
        Left template delimiter (default "{{")
  -listen string
        Listen address (default "0.0.0.0:80")
  -log int
        Log level, DEBUG=-4, INFO=0, WARN=4, ERROR=8
  -rdelim string
        Right template delimiter (default "{{")
  -template-root string
        Template root directory (default "templates")
  -template-extension
        File extension to look for to identify template files (default ".html")
  -watch-context
        Watch the context directory and reload if changed
  -watch-template
        Watch the template directory and reload if changed (default true)
```

### Example

```
xtemplate -template-root templates -watch-template -log -4
```
