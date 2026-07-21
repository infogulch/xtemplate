# Shared helpers for xtemplate's mise tasks.
#
# This file is NOT a task (it lives outside tasks/ so mise won't load it as
# one). Import it from a task with:
#
#   use ../lib.nu *
#
# The `use` runs the `export-env` block below, which sets $env.ROOT/DIST_DIR/
# TEST_DIR for the task, and brings every exported `def` into scope.

export-env {
    # Use mise's root when present, else the git toplevel.
    $env.ROOT = ($env.MISE_PROJECT_ROOT? | default (^git rev-parse --show-toplevel | str trim))
    $env.DIST_DIR = ($env.ROOT | path join dist)
    $env.TEST_DIR = ($env.ROOT | path join test)
    mkdir $env.DIST_DIR
}

# Emit a clean release version only when building exactly at a v* tag. Otherwise
# return an empty string: the binary's Version() falls back to the module/VCS
# info the Go toolchain embeds (see app.version), so non-tag builds get a useful
# commit-based version without us inventing one here.
export def xt-version []: nothing -> string {
    let r = (do { ^git -C $env.ROOT describe --tags --exact-match --match 'v*' } | complete)
    if $r.exit_code == 0 { $r.stdout | str trim } else { "" }
}

# Build the -ldflags string. When xt-version is empty (non-tag build) this
# returns an empty string, leaving app.version unset so the ReadBuildInfo
# fallback takes over.
export def xt-ldflags []: nothing -> string {
    let v = (xt-version)
    if ($v | is-empty) { "" } else { $"-X 'github.com/infogulch/xtemplate/app.version=($v)'" }
}

# Docker image tag: the release version when building at a v* tag, otherwise a
# commit-based dev tag. Unlike xt-version this is always non-empty, so
# `docker build -t infogulch/xtemplate:<tag>` is always valid.
export def xt-image-tag []: nothing -> string {
    let v = (xt-version)
    if ($v | is-empty) {
        $"dev-(^git -C $env.ROOT rev-parse --short HEAD | str trim)"
    } else {
        $v
    }
}

# The most recent v* tag, used to decide whether to also tag/push :latest.
export def xt-latest-tag []: nothing -> string {
    ^git -C $env.ROOT tag -l --sort=-version:refname | lines | get 0? | default ""
}

# Print the Docker image tags to build/push: always the version tag, plus
# :latest when building at the most recent v* release tag. Centralized so
# build-docker and push-docker can't disagree on which tags exist.
export def xt-image-tags []: nothing -> list<string> {
    let ver = (xt-image-tag)
    let tags = [$"infogulch/xtemplate:($ver)"]
    if $ver == (xt-latest-tag) {
        $tags | append "infogulch/xtemplate:latest"
    } else {
        $tags
    }
}

# Create a working/log directory for one test run, grouped under dist/_runs/
# and prefixed with a sortable timestamp so runs list in chronological order
# (newest last). Maintains a `latest-<target>` symlink and prunes all but the
# newest few runs per target. Returns the new directory path.
#
# The dir is named with a leading underscore (dist/_runs) so the Go toolchain
# ignores it: `go test ./...` and `golangci-lint run ./...` skip directories
# whose names begin with `.` or `_`. Without that, those tree walks race with
# the concurrent create/delete churn here when run in parallel under `ci`.
export def new-run-dir [target: string]: nothing -> string {
    let runs = ($env.DIST_DIR | path join _runs)
    mkdir $runs

    let stamp = (date now | format date '%Y%m%d-%H%M%S')
    let dir = (^mktemp -d ($runs | path join $"($stamp)-($target)-XXXX") | str trim)

    ^cp -r ...[
        ($env.TEST_DIR | path join templates)
        ($env.TEST_DIR | path join data)
        ($env.TEST_DIR | path join migrations)
        ($env.TEST_DIR | path join caddy.json)
        ($env.TEST_DIR | path join config.json)
        ($env.TEST_DIR | path join Caddyfile)
        $dir
    ]

    # Convenience pointer to the most recent run for this target.
    ^ln -sfn $dir ($runs | path join $"latest-($target)")

    # Keep only the 5 newest runs per target. The timestamped dirs start with a
    # digit (so this never matches the latest-* symlink) and sort
    # chronologically by name, so sort+reverse puts the newest first.
    glob $"($runs)/[0-9]*-($target)-*"
    | sort
    | reverse
    | skip 5
    | each { |old| rm -rf $old }

    $dir
}

# Wait for a server to accept requests on /. curl retries through the brief
# startup window, where connection errors are expected; capture curl's output
# and surface it only if the probe ultimately fails, so those transient retry
# messages don't clutter the log.
export def wait-ready [port: int, retries: int = 10] {
    let probe = (^curl -fsS --retry $retries --retry-all-errors --retry-connrefused --retry-delay 1 $"http://localhost:($port)/" | complete)
    if $probe.exit_code != 0 {
        print -e $probe.stderr
        error make --unspanned { msg: $"readiness probe for port ($port) failed" }
    }
}

# Wait for a server started on an ephemeral port (`:0`) to log its chosen
# address, then return the port it bound, so integration tests never collide
# with whatever else happens to be running on the host. Matches both xtemplate's
# slog output (`...address=[::]:PORT`) and Caddy's JSON (`"actual_address":
# "[::]:PORT"`).
export def wait-listen-port [log: string] {
    for _ in 1..50 {
        if ($log | path exists) {
            let m = (open --raw $log | parse --regex '(?:actual_address":"|address=)[^" \n]*:(?<port>[0-9]+)')
            if not ($m | is-empty) { return ($m | last | get port | into int) }
        }
        sleep 100ms
    }
    error make --unspanned { msg: $"server did not report a listen address in ($log)" }
}

# Run the whole hurl suite against a running server. The .hurl files hardcode
# localhost:8080, so --connect-to remaps that to the target's actual port.
export def run-hurl [port: int, report: string] {
    wait-ready $port 10
    mkdir $report
    ^hurl --continue-on-error --no-output --test --report-html $report --connect-to $"localhost:8080:localhost:($port)" ...(glob $"($env.TEST_DIR)/tests/*.hurl")
}

# Copy an example app's directory (examples/<name>/) into a fresh run dir under
# dist/_runs and return its path, so integration runs get a clean working copy
# (fresh sqlite, etc.) and can run in parallel without stepping on each other.
# Mirrors new-run-dir (including the `_runs` name that keeps the Go toolchain
# out of these dirs). Keeps only the 5 newest runs per example.
export def new-example-dir [name: string]: nothing -> string {
    let runs = ($env.DIST_DIR | path join _runs)
    mkdir $runs
    let stamp = (date now | format date '%Y%m%d-%H%M%S')
    let dir = ($runs | path join $"($stamp)-example-($name)")
    ^cp -r ($env.ROOT | path join examples $name) $dir
    # Drop any *.sqlite a dev session left behind so each run starts from an
    # empty database. (The copied Go sources can stay: dist/_runs is invisible
    # to `go test ./...` and `golangci-lint run ./...` thanks to the `_` prefix.)
    glob $"($dir)/**/*.sqlite*" | each { |f| rm -f $f }
    ^ln -sfn $dir ($runs | path join $"latest-example-($name)")
    glob $"($runs)/[0-9]*-example-($name)" | sort | reverse | skip 5 | each { |old| rm -rf $old }
    $dir
}

# Poll an example server's endpoint, then run its hurl suite (the .hurl
# files under <dir>/tests/) against the given port. Example .hurl files hardcode
# localhost:8080; --connect-to remaps that to the example's actual port, the
# same convention run-hurl uses for the main suite.
export def run-example [port: int, dir: string] {
    wait-ready $port 30
    let report = ($dir | path join report)
    mkdir $report
    ^hurl --continue-on-error --no-output --test --report-html $report --connect-to $"localhost:8080:localhost:($port)" ...(glob $"($dir)/tests/*.hurl")
}

# List external dep packages for the current platform, configured by setting
# GOOS/GOARCH in the environment. Uses -deps -test ./... to include packages
# imported by test files in this module, so gotest doesn't need to recompile
# test-only deps from scratch.
export def xt-precache-pkgs []: nothing -> list<string> {
    ^go list -deps -test -f '{{if and .Module (not .Module.Main) (not .ForTest)}}{{.ImportPath}}{{end}}' ./...
    | lines
    | where { |l| ($l | str trim) != "" }
}

# Compile stdlib and external dependencies into the Go build cache. Set
# GOOS/GOARCH in the environment to target a specific platform; pass --race to
# build the race-enabled variant (used for the host platform by gotest).
export def xt-precache-deps [--race] {
    let flags = (if $race { ["-race"] } else { [] })
    ^go build ...$flags std
    let pkgs = (xt-precache-pkgs)
    if not ($pkgs | is-empty) {
        ^go build ...$flags ...$pkgs
    }
}

# Return a content hash of the current Go version plus go.mod and go.sum. Used
# by prebuild tasks to key Go/Docker caches.
export def xt-prebuild-hash []: nothing -> string {
    let go_version = (^go version)
    let go_mod = (open --raw ($env.ROOT | path join go.mod) | decode utf-8)
    let go_sum = (open --raw ($env.ROOT | path join go.sum) | decode utf-8)
    $"($go_version)\n($go_mod)($go_sum)" | hash sha256
}

# Build Caddy with the xtemplate module via xcaddy. Extra args are forwarded to
# xcaddy build. Defined here (not just in the task) so prebuild can warm the
# xcaddy build cache by calling it directly.
export def xt-build-caddy [...extra: string] {
    cd $env.ROOT
    (^xcaddy build
        --with github.com/infogulch/xtemplate=.
        --with github.com/infogulch/xtemplate/caddy/standard=./caddy/standard
        --with github.com/ncruces/go-sqlite3/driver
        --output ($env.DIST_DIR | path join caddy)
        ...$extra) e> ($env.DIST_DIR | path join caddy.build.log)
}

# Build (and optionally tag) the production Docker image. Defined here so the
# build-docker task, test-docker, and prebuild can all share one definition and
# can't disagree on build args.
#
#   --target     build a specific stage instead of the default final image
#   --cache-to   export the BuildKit layer cache (used by prebuild)
#   --tags       explicit image tags; when omitted and no --target is given,
#                defaults to the release version tags from xt-image-tags
export def xt-build-docker [
    --target: string = ""
    --cache-to: string = ""
    --tags: list<string> = []
] {
    cd $env.ROOT

    mut args = []

    let cache_src = ($env.DIST_DIR | path join buildx-cache)
    if ($cache_src | path exists) {
        $args = ($args | append ["--cache-from" $"type=local,src=($cache_src)"])
    }

    $args = ($args | append ["--build-arg" $"LDFLAGS=(xt-ldflags)"])

    if ($target | is-not-empty) { $args = ($args | append ["--target" $target]) }
    if ($cache_to | is-not-empty) { $args = ($args | append ["--cache-to" $cache_to]) }

    # With no explicit tags and no target, build and tag the production image.
    # Otherwise use the given tags (possibly none), so callers can build a
    # different stage without mislabeling it as a release.
    let tag_list = if ($tags | is-empty) and ($target | is-empty) { xt-image-tags } else { $tags }
    for t in $tag_list { $args = ($args | append ["-t" $t]) }

    ^docker build -q ...$args .
}
