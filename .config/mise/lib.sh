#!/usr/bin/env bash
# Shared helpers for xtemplate's mise tasks.
#
# This file is NOT a task (it lives outside tasks/ so mise won't load it as
# one). Source it from a task with:
#
#   # shellcheck source=/dev/null
#   source "$MISE_PROJECT_ROOT/.config/mise/lib.sh"

: "${MISE_PROJECT_ROOT:?MISE_PROJECT_ROOT is not set; run via 'mise run <task>'}"

export ROOT="$MISE_PROJECT_ROOT"
export DIST_DIR="$ROOT/dist"
export TEST_DIR="$ROOT/test"

mkdir -p "$DIST_DIR"

# add an alias for every task in .config/mise/tasks/ so tasks can invoke each
# other by name. Bash only expands aliases in interactive shells unless this is
# set, and mise runs tasks non-interactively, so enable it explicitly.
shopt -s expand_aliases
for task in "$MISE_PROJECT_ROOT/.config/mise/tasks/"*; do
    #shellcheck disable=SC2139
    alias "$(basename "$task")"="$task"
done

# Emit a clean release version only when building exactly at a v* tag. Otherwise
# print nothing: the binary's Version() falls back to the module/VCS info the Go
# toolchain embeds (see app.version), so non-tag builds get a useful commit-based
# version without us inventing one here.
xt_version() {
	git -C "$ROOT" describe --tags --exact-match --match='v*' 2>/dev/null || true
}

# Build the -ldflags string. When xt_version is empty (non-tag build) this emits
# nothing, leaving app.version unset so the ReadBuildInfo fallback takes over.
xt_ldflags() {
	local v
	v="$(xt_version)"
	[ -n "$v" ] || return 0
	printf -- "-X 'github.com/infogulch/xtemplate/app.version=%s'" "$v"
}

# Docker image tag: the release version when building at a v* tag, otherwise a
# commit-based dev tag. Unlike xt_version this is always non-empty, so
# `docker build -t infogulch/xtemplate:<tag>` is always valid.
xt_image_tag() {
	local v
	v="$(xt_version)"
	if [ -n "$v" ]; then
		printf '%s\n' "$v"
	else
		printf 'dev-%s\n' "$(git -C "$ROOT" rev-parse --short HEAD)"
	fi
}

# The most recent v* tag, used to decide whether to also tag/push :latest.
xt_latest_tag() {
	git -C "$ROOT" tag -l --sort=-version:refname | head -n1
}

# xt_image_tags — print the Docker image tags to build/push, one per line:
# always the version tag, plus :latest when building at the most recent v*
# release tag. Centralized so build-docker and push-docker can't disagree on
# which tags exist; read into an array with `mapfile -t`.
xt_image_tags() {
	local ver
	ver="$(xt_image_tag)"
	printf 'infogulch/xtemplate:%s\n' "$ver"
	[ "$ver" = "$(xt_latest_tag)" ] && printf 'infogulch/xtemplate:latest\n'
}

# new_run_dir TARGET — create a working/log directory for one test run, grouped
# under dist/runs/ and prefixed with a sortable timestamp so runs list in
# chronological order (newest last). Maintains a `latest-<target>` symlink and
# prunes all but the newest few runs per target. Prints the new directory path.
new_run_dir() {
	local target="$1"
	local runs="$DIST_DIR/runs"
	mkdir -p "$runs"

	local stamp dir
	stamp="$(date +%Y%m%d-%H%M%S)"
	dir="$(mktemp -d "$runs/${stamp}-${target}-XXXX")"

	cp -r \
		"$TEST_DIR/templates" \
		"$TEST_DIR/data" \
		"$TEST_DIR/migrations" \
		"$TEST_DIR/caddy.json" \
		"$TEST_DIR/config.json" \
		"$dir"

	# Convenience pointer to the most recent run for this target.
	ln -sfn "$dir" "$runs/latest-${target}"

	# Keep only the 5 newest runs per target (timestamped dirs start with a
	# digit, so this never matches the latest-* symlink).
	# shellcheck disable=SC2012
	ls -1dt "$runs/"[0-9]*-"${target}"-* 2>/dev/null | tail -n +6 | while IFS= read -r old; do
		rm -rf "$old"
	done

	printf '%s\n' "$dir"
}

# run_hurl PORT REPORT_DIR — run the whole hurl suite against a running server.
# The .hurl files hardcode localhost:8080, so --connect-to remaps that to the
# target's actual port.
run_hurl() {
	local port="$1" report="$2"
	# Wait for the server to accept requests. curl retries through the brief
	# startup window, during which connection errors are expected; capture its
	# output and surface it only if the probe ultimately fails, so those
	# transient retry messages don't clutter the log.
	local probe_err
	if ! probe_err="$(curl -fsS --retry 10 --retry-all-errors --retry-connrefused --retry-delay 1 \
		"http://localhost:${port}/ready" 2>&1 >/dev/null)"; then
		printf '%s\n' "$probe_err" >&2
		return 1
	fi
	mkdir -p "$report"
	hurl --continue-on-error --no-output --test \
		--report-html "$report" \
		--connect-to "localhost:8080:localhost:${port}" \
		"$TEST_DIR"/tests/*.hurl
}

# xt_precache_pkgs — list external dep packages for the current platform,
# configured by setting GOOS/GOARCH in the environment. Uses -deps -test ./...
# to include packages imported by test files in this module, so gotest doesn't
# need to recompile test-only deps from scratch.
xt_precache_pkgs() {
	go list -deps -test \
		-f '{{if and .Module (not .Module.Main) (not .ForTest)}}{{.ImportPath}}{{end}}' \
		./...
}

# xt_precache_deps [FLAGS] — compile stdlib and external dependencies into the
# Go build cache.  Set GOOS/GOARCH in the environment to target a specific
# platform; any FLAGS are forwarded to other go build invocations (e.g. -race).
xt_precache_deps() {
	go build "$@" std
	xt_precache_pkgs | xargs --no-run-if-empty go build "$@"
}

# xt_prebuild_hash — return a content hash of the current Go version plus
# go.mod and go.sum.  Used by prebuild tasks to key Go/Docker caches.
xt_prebuild_hash() {
	{ go version; cat "$ROOT/go.mod" "$ROOT/go.sum"; } |
		sha256sum | awk '{print $1}'
}
