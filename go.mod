module github.com/infogulch/xtemplate

go 1.22.1

require (
	github.com/BurntSushi/toml v1.3.2
	github.com/Masterminds/sprig/v3 v3.2.3
	github.com/alecthomas/chroma/v2 v2.13.0
	github.com/alexflint/go-arg v1.4.3
	github.com/andybalholm/brotli v1.1.0
	github.com/dustin/go-humanize v1.0.1
	github.com/felixge/httpsnoop v1.0.4
	github.com/google/uuid v1.6.0
	github.com/infogulch/watch v0.2.0
	github.com/klauspost/compress v1.17.7
	github.com/mattn/go-sqlite3 v1.14.22
	github.com/microcosm-cc/bluemonday v1.0.26
	github.com/nats-io/nats-server/v2 v2.10.12
	github.com/nats-io/nats.go v1.34.1
	github.com/tdewolff/minify/v2 v2.20.19
	github.com/yuin/goldmark v1.7.1
	github.com/yuin/goldmark-highlighting/v2 v2.0.0-20230729083705-37449abec8cc
	gopkg.in/yaml.v3 v3.0.1
)

// Don't drink and drive, and don't change your repo's import path, kids.
// https://github.com/darccio/mergo/issues/244 🙄
replace github.com/imdario/mergo => github.com/imdario/mergo v0.3.16

require (
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.2.1 // indirect
	github.com/alexflint/go-scalar v1.1.0 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/dlclark/regexp2 v1.11.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/huandu/xstrings v1.4.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/minio/highwayhash v1.0.2 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/nats-io/jwt/v2 v2.5.5 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/stretchr/testify v1.8.4 // indirect
	github.com/tdewolff/parse/v2 v2.7.12 // indirect
	golang.org/x/crypto v0.22.0 // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)
