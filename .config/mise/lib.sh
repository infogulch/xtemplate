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

# Derive the version from git only (no network / `go list`), matching the CUE
# build's intent without its unconditional module resolution.
xt_version() {
	git -C "$ROOT" describe --tags --exact-match --match='v*' 2>/dev/null ||
		echo "detached-$(git -C "$ROOT" rev-parse --short HEAD)"
}

xt_ldflags() {
	printf -- "-X 'github.com/infogulch/xtemplate/app.version=%s'" "$(xt_version)"
}

# The most recent v* tag, used to decide whether to also tag/push :latest.
xt_latest_tag() {
	git -C "$ROOT" tag -l --sort=-version:refname | head -n1
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

# copy_fixtures DEST — stage the test fixtures into a working directory, the way
# each server target expects to be run.
copy_fixtures() {
	local dest="$1"
	cp -r \
		"$TEST_DIR/templates" \
		"$TEST_DIR/data" \
		"$TEST_DIR/migrations" \
		"$TEST_DIR/caddy.json" \
		"$TEST_DIR/config.json" \
		"$dest"
}

# wait_ready PORT — block until the server answers /ready (or give up).
wait_ready() {
	local port="$1"
	curl -fsS --retry 10 --retry-all-errors --retry-connrefused --retry-delay 1 \
		"http://localhost:${port}/ready" >/dev/null
}

# run_hurl PORT REPORT_DIR — run the whole hurl suite against a running server.
# The .hurl files hardcode localhost:8080, so --connect-to remaps that to the
# target's actual port.
run_hurl() {
	local port="$1" report="$2"
	mkdir -p "$report"
	hurl --continue-on-error --no-output --test \
		--report-html "$report" \
		--connect-to "localhost:8080:localhost:${port}" \
		"$TEST_DIR"/tests/*.hurl
}
