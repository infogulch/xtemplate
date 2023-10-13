This package wraps `xtemplate` into a usable binary.

Usage:

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
  -watch-context
        Watch the context directory and reload if changed
  -watch-template
        Watch the template directory and reload if changed (default true)
```
