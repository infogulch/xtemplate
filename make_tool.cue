package xtemplate

import (
	"tool/exec"
	"strings"
	"tool/file"
	"list"
	// "tool/cli"
	// "encoding/json"
)

#vars: {
	rootdir: string
	testdir: string
	gitver:  string
	version: string
	ldflags: string
	latest:  string
}

meta: {
	vars: #vars & {
		rootdir: strings.TrimSpace(_commands.reporoot.stdout)
		testdir: "\(rootdir)/test"
		gitver:  strings.TrimSpace(_commands.gitver.stdout)
		version: strings.TrimSpace(_commands.version.stdout)
		ldflags: "-X 'github.com/infogulch/xtemplate/app.version=\(version)'"
		latest:  strings.TrimSpace(_commands.latest.stdout)
	}
	_commands: {
		reporoot: exec.Run & {
			cmd: ["bash", "-c", "git rev-parse --show-toplevel; >&2 echo reporoot"]
			stdout: string
		}
		gitver: exec.Run & {
			cmd: ["bash", "-c", "git describe --exact-match --tags --match='v*' 2> /dev/null || git rev-parse --short HEAD; >&2 echo gitver"]
			dir:    vars.rootdir
			stdout: string
		}
		version: exec.Run & {
			cmd: ["bash", "-c", "go list -f {{.Version}} -m github.com/infogulch/xtemplate@\(vars.gitver); >&2 echo version"]
			dir:    vars.rootdir
			stdout: string
		}
		latest: exec.Run & {
			cmd: ["bash", "-c", "git tag -l --sort -version:refname | head -n 1; >&2 echo latest"]
			dir:    vars.rootdir
			stdout: string
		}
	}
}

task: build: {
	vars: #vars

	OutFile: *"xtemplate" | string

	gobuild: exec.Run & {
		env: {[string]: string}
		cmd: ["go", "build", "-ldflags", vars.ldflags, "-buildmode", "exe", "-o", OutFile, "./cmd"]
		dir:     vars.rootdir
		success: true
	}
}

task: run: {
	vars: #vars

	gobuild: task.build & {"vars": vars, OutFile: "\(vars.testdir)/xtemplate"}
	rmdataw: file.RemoveAll & {path: "\(vars.testdir)/dataw"}
	mkdataw: file.Mkdir & {path: "\(vars.testdir)/dataw", $dep: rmdataw.$done}
	mklog: file.Create & {filename: "\(vars.testdir)/xtemplate.log", contents: ""}

	ready: exec.Run & {
		$dep: mklog.$done
		cmd: ["bash", "-c", "grep -q 'starting server' <(tail -f xtemplate.log)"]
		dir: vars.testdir
	}

	start: exec.Run & {
		$dep: mkdataw.$done && mklog.$done && gobuild.$done
		cmd: ["bash", "-c", "./xtemplate --loglevel -4 -d DB:sql:sqlite3:file:./dataw/test.sqlite -d FS:fs:./data --config-file config.json >xtemplate.log 2>&1"]
		dir: vars.testdir
	}
}

task: test: {
	vars: #vars

	port: int | *8080

	list: file.Glob & {glob: "\(vars.testdir)/tests/*.hurl"}
	hurl: exec.Run & {
		cmd: ["hurl", "--continue-on-error", "--test", "--report-html", "report", "--connect-to", "localhost:8080:localhost:\(port)"] + list.files
		dir: vars.testdir
	}
}

task: gotest: {
	vars: #vars

	gotest: exec.Run & {
		cmd: "go test -v ./..."
		dir: vars.rootdir
	}
}

task: dist: {
	vars: #vars

	rmdist: file.RemoveAll & {path: "\(vars.rootdir)/dist"}

	oses: ["linux", "darwin", "windows"]
	arches: ["amd64", "arm64"]
	matrix: [for os in oses for arch in arches {GOOS: os, GOARCH: arch}]

	for env in matrix {
		(env.GOOS + "_" + env.GOARCH): {
			dir: "\(vars.rootdir)/dist/xtemplate-\(env.GOARCH)-\(env.GOOS)"
			exe: string | *"xtemplate"
			if env.GOOS == "windows" {
				exe: "xtemplate.exe"
			}

			mkdir: file.MkdirAll & {path: dir, $dep: rmdist.$done}
			build: task.build.gobuild & {"vars": vars, "env": env, OutFile: "\(dir)/\(exe)", $dep: mkdir.$done}
			cp: exec.Run & {cmd: ["cp", "README.md", "LICENSE", "\(dir)"], $dep: mkdir.$done}
			// tar: exec.Run & {cmd: ["tar", "czf", "\(dir)_\(vars.version).tar.gz", "-C", dir, "."], $dep: cp.$done && build.$done}
			zip: exec.Run & {cmd: ["zip", "-jqr6", "\(dir)_\(vars.version).zip", dir], $dep: cp.$done && build.$done}
			// rm: file.RemoveAll & {path: dir, $dep: zip.$done}
		}
	}

	wait: {$dep: and([for name, step in command.dist if name =~ "_" {step.$done}])}
}

task: test_docker: {
	vars: #vars

	build: exec.Run & {
		cmd: ["docker", "build", "-t", "xtemplate-test", "--target", "test", "--build-arg", "LDFLAGS=\(vars.ldflags)", "."]
		dir: vars.rootdir
	}
	run: exec.Run & {
		cmd:  "docker run -d --rm --name xtemplate-test -p 8081:80 xtemplate-test"
		$dep: build.$done
	}
	ready: exec.Run & {
		cmd: ["bash", "-c", "grep -q 'starting server' <(docker logs -f xtemplate-test)"]
		$dep: run.$done
	}
	test: task.test & {port: 8081, hurl: $dep: ready.$done}
	stop: exec.Run & {cmd: "docker stop xtemplate-test", $dep: test.hurl.$done} // be nice if we can always run this even if previous steps fail
}

task: build_docker: {
	vars: #vars

	_latest: [...string] | *[]
	if vars.version == vars.latest {
		_latest: ["infogulch/xtemplate:latest"]
	}
	tags: ["infogulch/xtemplate:\(vars.version)"] + _latest

	build: exec.Run & {
		cmd: ["docker", "build"] + list.FlattenN([for t in tags {["-t", t]}], 1) + ["--build-arg", "LDFLAGS=\(vars.ldflags)", "."]
		dir: vars.rootdir
	}
}

command: {
	for k, t in task {
		(k): {cfg: meta, vars: cfg.vars, t}
	}
}

command: run_test: {
	cfg: meta

	run: task.run & {"vars": cfg.vars, start: mustSucceed: false}
	test: task.test & {"vars": cfg.vars, hurl: $dep: run.ready.$done}
	kill: exec.Run & {$dep: test.hurl.$done, cmd: "pkill xtemplate"} // better way?
}

command: ci: {
	cfg: meta

	gotest: task.gotest & {"vars": cfg.vars}

	run: task.run & {"vars": cfg.vars, start: mustSucceed: false}
	test: task.test & {"vars": cfg.vars, hurl: $dep: run.ready.$done}
	kill: exec.Run & {cmd: "pkill xtemplate", $dep: test.hurl.$done} // better way?

	dist: task.dist & {"vars": cfg.vars, rmdist: $dep: kill.$done}

	test_docker: task.test_docker & {"vars": cfg.vars}
	build_docker: task.build_docker & {"vars": cfg.vars, build: $dep: test_docker.stop.$done}
	push: exec.Run & {
		cmd: ["echo", "docker", "push"] + build_docker.tags
		$dep: build_docker.build.$done
	}
}
