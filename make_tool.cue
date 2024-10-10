package xtemplate

import (
	"strings"
	"list"

	"tool/exec"
	"tool/file"
	"tool/os"
)

#vars: {
	rootdir: string
	testdir: string
	gitver:  string
	version: string
	ldflags: string
	latest:  string
	env: {[string]: string}
}

meta: {
	vars: #vars & {
		rootdir: strings.TrimSpace(_commands.reporoot.stdout)
		testdir: "\(rootdir)/test"
		gitver:  strings.TrimSpace(_commands.gitver.stdout)
		version: strings.TrimSpace(_commands.version.stdout)
		ldflags: "-X 'github.com/infogulch/xtemplate/app.version=\(version)'"
		latest:  strings.TrimSpace(_commands.latest.stdout)
		env: {for k, v in _commands.env if k != "$id" {(k): v}}
	}
	_commands: {
		reporoot: exec.Run & {
			cmd: ["bash", "-c", "git rev-parse --show-toplevel"]
			stdout: string
		}
		gitver: exec.Run & {
			cmd: ["bash", "-c", "git describe --exact-match --tags --match='v*' 2> /dev/null || git rev-parse --short HEAD"]
			dir:    vars.rootdir
			stdout: string
		}
		version: exec.Run & {
			cmd: ["bash", "-c", "go list -f {{.Version}} -m github.com/infogulch/xtemplate@\(vars.gitver)"]
			dir:    vars.rootdir
			stdout: string
		}
		latest: exec.Run & {
			cmd: ["bash", "-c", "git tag -l --sort -version:refname | head -n 1"]
			dir:    vars.rootdir
			stdout: string
		}
		env: os.Environ
	}
}

task: build: {
	vars: #vars

	outfile: *"xtemplate" | string

	gobuild: exec.Run & {
		env: {[string]: string}
		cmd: ["go", "build", "-ldflags", vars.ldflags, "-buildmode", "exe", "-o", outfile, "./cmd"]
		dir:         vars.rootdir
		mustSucceed: true
	}
}

task: run: {
	vars: #vars

	rmdataw: file.RemoveAll & {path: "\(vars.testdir)/dataw"}
	mkdataw: file.Mkdir & {path: "\(vars.testdir)/dataw", $after: rmdataw.$done}

	start: exec.Run & {
		cmd: ["bash", "-c", "./xtemplate --loglevel -4 -d DB:sql:sqlite3:file:./dataw/test.sqlite -d FS:fs:./data --config-file config.json &>xtemplate.log &"]
		dir:    vars.testdir
		$after: mkdataw.$done
	}
}

task: test: {
	vars: #vars

	port: int | *8080

	list: file.Glob & {glob: "\(vars.testdir)/tests/*.hurl"}
	ready: exec.Run & {cmd: "curl -X GET --retry-all-errors --retry 5 --retry-connrefused --retry-delay 1 http://localhost:\(port)/ready --silent", stdout: "OK"}
	hurl: exec.Run & {
		cmd: ["hurl", "--continue-on-error", "--no-output", "--test", "--report-html", "report", "--connect-to", "localhost:8080:localhost:\(port)"] + list.files
		dir:   vars.testdir
		after: ready.$done
	}
}

task: gotest: {
	vars: #vars

	gotest: exec.Run & {
		cmd: ["bash", "-c", "go test -v ./... >\(vars.testdir)/gotest.log"]
		dir: vars.rootdir
	}
}

task: build_test: {
	vars: #vars

	build: task.build & {"vars": vars, outfile: "\(vars.testdir)/xtemplate"}
	run: task.run & {"vars": vars, start: $after: build.gobuild.$done}
	test: task.test & {"vars": vars, ready: $after: run.start.$done}
	kill: exec.Run & {cmd: "pkill xtemplate", $after: test.hurl.$done}
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

			mkdir: file.MkdirAll & {path: dir, $after: rmdist.$done}
			build: task.build & {"vars": vars, outfile: "\(dir)/\(exe)", gobuild: {$after: mkdir.$done, "env": env & vars.env}}
			cp: exec.Run & {cmd: ["cp", "README.md", "LICENSE", "\(dir)"], $after: mkdir.$done}
			zip: exec.Run & {cmd: ["zip", "-jqr6", "\(dir)_\(vars.version).zip", dir], $after: cp.$done && build.$done}
		}
	}
}

task: build_docker: {
	vars: #vars

	tags: [
		"infogulch/xtemplate:\(vars.version)",
		if vars.version == vars.latest {"infogulch/xtemplate:latest"},
	]

	build: exec.Run & {
		cmd: ["docker", "build"] + list.FlattenN([for t in tags {["-t", t]}], 1) + ["--build-arg", "LDFLAGS=\(vars.ldflags)", "--progress=plain", "."]
		dir: vars.rootdir
	}
}

task: test_docker: {
	vars: #vars

	build: exec.Run & {
		cmd: ["docker", "build", "-t", "xtemplate-test", "--target", "test", "--build-arg", "LDFLAGS=\(vars.ldflags)", "."]
		dir: vars.rootdir
	}
	run: exec.Run & {
		cmd: ["docker", "run", "-d", "--rm", "--name", "xtemplate-test", "-p", "8081:80", "xtemplate-test"]
		$after: build.$done
	}
	test: task.test & {"vars": vars, port: 8081, ready: $after: run.$done}
	stop: exec.Run & {cmd: "docker stop xtemplate-test", $after: test.hurl.$done} // be nice if we can always run this even if previous steps fail
}

task: build_caddy: {
	vars: #vars

	xbuild: exec.Run & {
		cmd: ["bash", "-c",
			"xcaddy build " +
			"--with github.com/infogulch/xtemplate-caddy " +
			"--with github.com/infogulch/xtemplate=. " +
			"--with github.com/infogulch/xtemplate/providers=./providers " +
			"--with github.com/infogulch/xtemplate/providers/nats=./providers/nats " +
			"--with github.com/mattn/go-sqlite3 " +
			"--output \(vars.testdir)/caddy " +
			"&>\(vars.testdir)/xcaddy.log",
		]
		dir: vars.rootdir
		env: vars.env & {CGO_ENABLED: "1"}
	}
}

task: run_caddy: {
	vars: #vars

	rmdataw: file.RemoveAll & {path: "\(vars.testdir)/dataw"}
	mkdataw: file.Mkdir & {path: "\(vars.testdir)/dataw", $after: rmdataw.$done}

	start: exec.Run & {
		cmd: ["bash", "-c", "./caddy start --config caddy.json &>xtemplate.caddy.log"]
		dir:    vars.testdir
		$after: mkdataw.$done
	}
}

task: build_test_caddy: {
	vars: #vars

	build: task.build_caddy & {"vars": vars}
	run: task.run_caddy & {"vars": vars, start: $after: build.xbuild.$done}
	test: task.test & {"vars": vars, port: 8082, ready: $after: run.start.$done}
	kill: exec.Run & {cmd: "pkill caddy", $after: test.hurl.$done} // is there a better way?
}

command: {
	for k, t in task {
		(k): {cfg: meta, vars: cfg.vars, t}
	}
}

command: ci: {
	cfg: meta

	gotest: task.gotest & {"vars": cfg.vars}

	build_test: task.build_test & {"vars": cfg.vars}
	build_test_caddy: task.build_test_caddy & {"vars": cfg.vars, run: rmdataw: $after: build_test.kill.$done}

	dist: task.dist & {"vars": cfg.vars, rmdist: $after: build_test.kill.$done}

	test_docker: task.test_docker & {"vars": cfg.vars}
	build_docker: task.build_docker & {"vars": cfg.vars, build: $after: test_docker.stop.$done}

	push: exec.Run & {
		cmd: ["docker", "push", build_docker.tags[0]]
		$after: build_docker.build.$done && build_test.kill.$done && build_test_caddy.kill.$done
	}
}
