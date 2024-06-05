package xtemplate

import (
	"strings"
	"list"

	"tool/exec"
	"tool/file"
	"tool/os"

	// "encoding/json"
	// "tool/cli"
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

	gobuild: task.build & {"vars": vars, outfile: "\(vars.testdir)/xtemplate"}
	rmdataw: file.RemoveAll & {path: "\(vars.testdir)/dataw"}
	mkdataw: file.Mkdir & {path: "\(vars.testdir)/dataw", $after: rmdataw.$done}
	mklog: file.Create & {filename: "\(vars.testdir)/xtemplate.log", contents: ""}

	start: exec.Run & {
		$after: mkdataw.$done && mklog.$done && gobuild.gobuild.$done
		cmd: ["bash", "-c", "./xtemplate --loglevel -4 -d DB:sql:sqlite3:file:./dataw/test.sqlite -d FS:fs:./data --config-file config.json >\(mklog.filename) 2>&1"]
		dir: vars.testdir
	}

	ready: exec.Run & {
		$after: mklog.$done
		cmd: ["bash", "-c", "grep -q 'starting server' <(tail -f \(mklog.filename))"]
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
	osenv: os.Environ

	for env in matrix {
		(env.GOOS + "_" + env.GOARCH): {
			dir: "\(vars.rootdir)/dist/xtemplate-\(env.GOARCH)-\(env.GOOS)"
			exe: string | *"xtemplate"
			if env.GOOS == "windows" {
				exe: "xtemplate.exe"
			}

			mkdir: file.MkdirAll & {path: dir, $after: rmdist.$done}
			build: task.build & {"vars": vars, outfile: "\(dir)/\(exe)", gobuild: {$after: mkdir.$done, "env": env & osenv}}
			cp: exec.Run & {cmd: ["cp", "README.md", "LICENSE", "\(dir)"], $after: mkdir.$done}
			zip: exec.Run & {cmd: ["zip", "-jqr6", "\(dir)_\(vars.version).zip", dir], $after: cp.$done && build.$done}
		}
	}
}

task: test_docker: {
	vars: #vars

	build: exec.Run & {
		cmd: ["docker", "build", "-t", "xtemplate-test", "--target", "test", "--build-arg", "LDFLAGS=\(vars.ldflags)", "."]
		dir: vars.rootdir
	}
	run: exec.Run & {
		cmd:    "docker run -d --rm --name xtemplate-test -p 8081:80 xtemplate-test"
		$after: build.$done
	}
	ready: exec.Run & {
		cmd: ["bash", "-c", "grep -q 'starting server' <(docker logs -f xtemplate-test)"]
		$after: run.$done
	}
	test: task.test & {"vars": vars, port: 8081, hurl: $after: ready.$done}
	stop: exec.Run & {cmd: "docker stop xtemplate-test", $after: test.hurl.$done} // be nice if we can always run this even if previous steps fail
}

task: build_docker: {
	vars: #vars

	tags: [
		"infogulch/xtemplate:\(vars.version)",
		if vars.version == vars.latest {"infogulch/xtemplate:latest"},
	]

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
	test: task.test & {"vars": cfg.vars, hurl: $after: run.ready.$done}
	kill: exec.Run & {cmd: "pkill xtemplate", $after: test.hurl.$done}
}

command: ci: {
	cfg: meta

	gotest: task.gotest & {"vars": cfg.vars}

	run: task.run & {"vars": cfg.vars, start: mustSucceed: false} // it's killed so it will never succeed
	test: task.test & {"vars": cfg.vars, hurl: $after: run.ready.$done}
	kill: exec.Run & {cmd: "pkill xtemplate", $after: test.hurl.$done} // is there a better way?

	dist: task.dist & {"vars": cfg.vars, rmdist: $after: kill.$done}

	test_docker: task.test_docker & {"vars": cfg.vars}
	build_docker: task.build_docker & {"vars": cfg.vars, build: $after: test_docker.stop.$done}
	push: exec.Run & {
		cmd: ["docker", "push"] + build_docker.tags
		$after: build_docker.build.$done
	}
}
