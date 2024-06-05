package xtemplate

import (
	"tool/exec"
	"strings"
	"tool/file"
	"tool/cli"
	"list"
)

whencuefmtimports: cli.Print

meta: {
	vars: {
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

command: build: {
	cfg: meta

	gobuild: exec.Run & {
		OutFile: string | *"xtemplate"
		env: {[string]: string}
		cmd: ["go", "build", "-ldflags", cfg.vars.ldflags, "-buildmode", "exe", "-o", OutFile, "./cmd"]
		dir:     cfg.vars.rootdir
		success: true
	}
}

command: run: {
	cfg: meta

	gobuild: command.build & {gobuild: {OutFile: "\(cfg.vars.testdir)/xtemplate"}}
	rmdataw: file.RemoveAll & {path: "\(cfg.vars.testdir)/dataw"}
	mkdataw: file.Mkdir & {path: "\(cfg.vars.testdir)/dataw", $dep: rmdataw.$done}
	mklog: file.Create & {filename: "\(cfg.vars.testdir)/xtemplate.log", contents: ""}

	ready: exec.Run & {
		$dep: mklog.$done
		cmd: ["bash", "-c", "grep -q 'starting server' <(tail -f xtemplate.log)"]
		dir: cfg.vars.testdir
	}

	start: exec.Run & {
		$dep: mkdataw.$done && mklog.$done && gobuild.$done
		cmd: ["bash", "-c", "./xtemplate --loglevel -4 -d DB:sql:sqlite3:file:./dataw/test.sqlite -d FS:fs:./data --config-file config.json >xtemplate.log 2>&1"]
		dir: cfg.vars.testdir
	}
}

command: test: {
	cfg: meta

	list: file.Glob & {glob: "\(cfg.vars.testdir)/tests/*.hurl"}
	hurl: exec.Run & {
		port: int | *8080
		cmd: ["hurl", "--continue-on-error", "--test", "--report-html", "report", "--connect-to", "localhost:8080:localhost:\(port)"] + list.files
		dir: cfg.vars.testdir
	}
}

command: gotest: {
	cfg: meta

	gotest: exec.Run & {
		cmd: "go test -v ./..."
		dir: cfg.vars.rootdir
	}
}

command: run_test: {
	run: command.run & {start: mustSucceed: false}

	test: command.test & {hurl: {$dep: run.ready.$done}}

	kill: exec.Run & {$dep: test.hurl.$done, cmd: "pkill xtemplate"} // better way?
}

command: dist: {
	cfg: meta

	rmdist: file.RemoveAll & {path: "\(cfg.vars.rootdir)/dist"}

	oses: ["linux", "darwin", "windows"]
	arches: ["amd64"]
	matrix: [for os in oses for arch in arches {GOOS: os, GOARCH: arch}]

	for env in matrix {
		(env.GOOS + "_" + env.GOARCH): {
			dir: "\(cfg.vars.rootdir)/dist/xtemplate-\(env.GOARCH)-\(env.GOOS)"
			exe: string | *"xtemplate"
			if env.GOOS == "windows" {
				exe: "xtemplate.exe"
			}

			mkdir: file.MkdirAll & {path: dir, $dep: rmdist.$done}
			build: command.build.gobuild & {env: env, OutFile: "\(dir)/\(exe)", $dep: mkdir.$done}
			cp: exec.Run & {cmd: ["cp", "README.md", "LICENSE", "\(dir)"], $dep: mkdir.$done}
			// tar: exec.Run & {cmd: ["tar", "czf", "\(dir)_\(cfg.vars.version).tar.gz", "-C", dir, "."], $dep: cp.$done && build.$done}
			zip: exec.Run & {cmd: ["zip", "-jqr6", "\(dir)_\(cfg.vars.version).zip", dir], $dep: cp.$done && build.$done}
			// rm: file.RemoveAll & {path: dir, $dep: zip.$done}
		}
	}

	wait: {$dep: and([for name, step in command.dist if name =~ "_" {step.$done}])}
}

command: test_docker: {
	cfg: meta

	build: exec.Run & {
		cmd: ["docker", "build", "-t", "xtemplate-test", "--target", "test", "--build-arg", "LDFLAGS=\(cfg.vars.ldflags)", "."]
		dir: cfg.vars.rootdir
	}
	run: exec.Run & {
		cmd: ["bash", "-c", "docker run -d --rm --name xtemplate-test -p 8081:80 xtemplate-test"]
		$dep: build.$done
	}
	ready: exec.Run & {
		cmd: ["bash", "-c", "grep -q 'starting server' <(docker logs -f xtemplate-test)"]
		$dep: run.$done
	}
	test: command.test & {hurl: {port: 8081, $dep: ready.$done}}
	stop: exec.Run & {cmd: "docker stop xtemplate-test", $dep: test.hurl.$done} // be nice if we can always run this even if previous steps fail
}

command: build_docker: {
	cfg: meta

	tags: [...string] | *["infogulch/xtemplate:\(cfg.vars.version)"]
	if cfg.vars.version == cfg.vars.latest {
		tags: ["infogulch/xtemplate:\(cfg.vars.version)", "infogulch/xtemplate:latest"]
	}

	build: exec.Run & {
		cmd: ["docker", "build"] + list.FlattenN([for t in tags {["-t", t]}], 1) + ["--build-arg", "LDFLAGS=\(cfg.vars.ldflags)", "."]
		dir: cfg.vars.rootdir
	}
}

command: ci: {
	gotest:   command.gotest
	run_test: command.run_test

	dist: command.dist & {rmdist: $dep: run_test.kill.$done}

	test_docker: command.test_docker

	build_docker: command.build_docker & {build: $dep: test_docker.stop.$done}

	push: exec.Run & {
		cmd: ["docker", "push"] + build_docker.tags
		$dep: build_docker.build.$done
	}
}
