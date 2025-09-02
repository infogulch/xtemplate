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
	distdir: string
	gitver:  string
	version: string
	ldflags: string
	latest:  string
	env: {[string]: string}
	exeExt:    string | *""
	dockercmd: string
}

meta: {
	vars: #vars & {
		rootdir: strings.TrimSpace(_commands.reporoot.stdout)
		testdir: "\(rootdir)/test/"
		distdir: "\(rootdir)/dist/"
		gitver:  strings.TrimSpace(_commands.gitver.stdout)
		version: strings.TrimSpace(_commands.version.stdout)
		ldflags: "-X 'github.com/infogulch/xtemplate/app.version=\(version)'"
		latest:  strings.TrimSpace(_commands.latest.stdout)
		env: {for k, v in _commands.env if k != "$id" {(k): v}}
		if _commands.os == "windows" {exeExt: ".exe"}
		dockercmd: *"docker" | string @tag(docker,short=docker)
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
			cmd: ["bash", "-c", "go list -f {{.Version}} -m github.com/infogulch/xtemplate@\(vars.gitver) 2> /dev/null || git describe --tags --match='v*' || echo detached-\(vars.gitver)"]
			dir:    vars.rootdir
			stdout: string
		}
		latest: exec.Run & {
			cmd: ["bash", "-c", "git tag -l --sort -version:refname | head -n 1"]
			dir:    vars.rootdir
			stdout: string
		}
		env:  os.Environ
		"os": string @tag(os,var=os)
	}
}

task: mktemp: {
	vars: #vars

	now0: string @tag(now,var=now)
	now1: strings.Replace(strings.SliceRunes(now0, 0, 19), ":", "-", -1)
	mktemp: file.MkdirTemp & {dir: vars.distdir, pattern: "test_\(now1)_"}
	copy: exec.Run & {
		cmd: ["cp", "-r", "templates/", "data/", "migrations/", "caddy.json", "config.json", mktemp.path]
		dir:   vars.testdir
		$done: bool
	}
}

task: test: {
	vars: #vars

	port:       int | *8080
	reportpath: string | *"report"

	testfiles: file.Glob & {glob: "\(vars.testdir)/tests/*.hurl"}
	mkdir: file.Mkdir & {path: reportpath}
	ready: exec.Run & {cmd: "curl -X GET --retry-all-errors --retry 5 --retry-connrefused --retry-delay 1 http://localhost:\(port)/ready --silent", stdout: "OK"}
	hurl: exec.Run & {
		cmd: ["bash", "-xc", "hurl --continue-on-error --no-output --test --report-html '\(reportpath)' --connect-to localhost:8080:localhost:\(port) '\(strings.Join(testfiles.files, "' '"))' &>'\(reportpath)/hurl.log'"]
		dir:   vars.testdir
		after: ready.$done && mkdir.$done
	}
}

task: gotest: {
	vars: #vars

	gotest: exec.Run & {
		cmd: ["bash", "-xc", "go test -v ./... &>'\(vars.distdir)/gotest.log'"]
		dir: vars.rootdir
	}
}

task: build_cli: {
	vars: #vars

	logfile: string | *"gobuild.log"
	outfile: *"\(vars.distdir)/xtemplate\(vars.exeExt)" | string

	gobuild: exec.Run & {
		env: {[string]: string}
		cmd: ["bash", "-xc", "go build -x -ldflags \"\(vars.ldflags)\" -buildmode exe -o '\(outfile)' ./cmd &>'\(vars.distdir)/\(logfile)'"]
		dir:         vars.rootdir
		mustSucceed: true
	}
}

task: run_cli: {
	vars: #vars

	mktemp: task.mktemp & {"vars": vars}

	start: exec.Run & {
		cmd: ["bash", "-xc", "'\(vars.distdir)/xtemplate\(vars.exeExt)' --loglevel -4 --config-file config.json &>xtemplate.log & echo $!"]
		dir:    mktemp.mktemp.path
		stdout: string
		$after: mktemp.copy.$done
	}
}

task: build_test_cli: {
	vars: #vars

	build: task.build_cli & {"vars": vars}
	run: task.run_cli & {"vars": vars, start: $after: build.gobuild.$done}
	test: task.test & {"vars": vars, reportpath: "\(run.mktemp.mktemp.path)/report", ready: $after: run.start.$done}
	kill: exec.Run & {cmd: ["bash", "-c", "kill \(run.start.stdout)"], mustSucceed: false, $after: test.hurl.$done}
}

task: dist: {
	vars: #vars

	oses: ["linux", "darwin", "windows"]
	arches: ["amd64", "arm64"]
	matrix: [for os in oses for arch in arches {GOOS: os, GOARCH: arch}]

	for env in matrix {
		("dist_" + env.GOOS + "_" + env.GOARCH): {
			dir: "\(vars.distdir)/xtemplate-\(env.GOARCH)-\(env.GOOS)"
			exe: string | *"xtemplate"
			if env.GOOS == "windows" {
				exe: "xtemplate.exe"
			}

			rmdir: file.RemoveAll & {path: dir}
			mkdir: file.MkdirAll & {path: dir, $after: rmdir.$done}
			build: task.build_cli & {"vars": vars, outfile: "\(dir)/\(exe)", logfile: "gobuild-\(env.GOARCH)-\(env.GOOS).log", gobuild: {$after: mkdir.$done, "env": env & vars.env}}
			cp: exec.Run & {cmd: ["cp", "README.md", "LICENSE", "\(dir)"], $after: mkdir.$done}
			zip: exec.Run & {cmd: ["zip", "-jqr6", "\(dir)_\(vars.version).zip", dir], $after: cp.$done & build.gobuild.$done}
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
		cmd: ["bash", "-xc", "\(vars.dockercmd) build \(strings.Join(list.FlattenN([for t in tags {["-t", t]}], 1), " ")) --build-arg 'LDFLAGS=\(vars.ldflags)' --progress=plain \".\" &>'\(vars.distdir)/docker-build.log'"]
		dir: vars.rootdir
	}
}

task: build_test_docker: {
	vars: #vars

	mktemp: task.mktemp & {"vars": vars}

	build: exec.Run & {
		cmd: ["bash", "-xc", "\(vars.dockercmd) build -t xtemplate-test --target test --build-arg 'LDFLAGS=\(vars.ldflags)' --progress=plain . &>'\(vars.distdir)/docker-build-test.log'"]
		dir:    vars.rootdir
		$after: mktemp.mktemp.$done
	}
	del: exec.Run & {
		cmd: ["bash", "-c", "\(vars.dockercmd) rm xtemplate-test"]
		mustSucceed: false
		stdout:      string
		stderr:      string
	}
	run: exec.Run & {
		cmd: ["bash", "-xc", "\(vars.dockercmd) run -d --name xtemplate-test -p 8081:80 -v \(mktemp.mktemp.path):/app/dataw xtemplate-test"]
		$after: build.$done && del.$done && mktemp.copy.$done
	}
	logs: exec.Run & {
		cmd: ["bash", "-c", "\(vars.dockercmd) logs xtemplate-test &>docker.log"]
		dir:    mktemp.mktemp.path
		$after: run.$done
	}
	test: task.test & {"vars": vars, port: 8081, reportpath: "\(mktemp.mktemp.path)/report", ready: $after: run.$done}
	stop: exec.Run & {cmd: "\(vars.dockercmd) stop xtemplate-test", $after: test.hurl.$done} // be nice if we can always run this even if previous steps fail
}

task: push_docker: {
	vars: #vars

	tags: [...string]

	for tag in tags {
		("push-" + tag): exec.Run & {
			cmd: [vars.dockercmd, "push", tag]
			mustSucceed: false
		}
	}
}

task: build_caddy: {
	vars: #vars

	flag: *"" | string @tag(debug,short=debug)

	xbuild: exec.Run & {
		cmd: ["bash", "-xc",
			"xcaddy build " +
			"--with github.com/infogulch/xtemplate/caddy=./caddy " +
			"--with github.com/infogulch/xtemplate=. " +
			"--with github.com/ncruces/go-sqlite3/driver " +
			"--with github.com/ncruces/go-sqlite3/embed " +
			"--output '\(vars.distdir)/caddy\(vars.exeExt)' " +
			"&>'\(vars.distdir)/xcaddy.log'",
		]
		dir: vars.rootdir
		env: vars.env & {
			if flag == "debug" {XCADDY_DEBUG: "1"}
		}
	}
}

task: run_caddy: {
	vars: #vars

	mktemp: task.mktemp & {"vars": vars}

	start: exec.Run & {
		cmd: ["bash", "-c", "\(vars.distdir)/caddy\(vars.exeExt) start --config caddy.json &>caddy.log & echo $!"]
		dir:    mktemp.mktemp.path
		$after: mktemp.copy.$done
		stdout: string
	}
}

task: build_test_caddy: {
	vars: #vars

	build: task.build_caddy & {"vars": vars}
	run: task.run_caddy & {"vars": vars, mktemp: mktemp: $after: build.xbuild.$done}
	test: task.test & {"vars": vars, port: 8082, reportpath: "\(run.mktemp.mktemp.path)/report", ready: $after: run.start.$done}
	kill: exec.Run & {cmd: ["bash", "-c", "kill \(run.start.stdout)"], mustSucceed: false, stdout: string, stderr: string, $after: test.hurl.$done} // is there a better way?
	killps: exec.Run & {cmd: ["powershell", "-c", "kill $(ps caddy)"], mustSucceed: false, stdout: string, stderr: string, $after: test.hurl.$done} // is there a better way?
}

command: {
	for k, t in task {
		(k): {cfg: meta, vars: cfg.vars, t}
	}
}

command: ci: {
	cfg: meta

	rmdist: file.RemoveAll & {path: cfg.vars.distdir}
	mkdist: file.Mkdir & {path: cfg.vars.distdir, $after: rmdist.$done}

	gotest: task.gotest & {"vars": cfg.vars, gotest: $after: mkdist.$done}

	build_test_cli: task.build_test_cli & {"vars": cfg.vars, run: mktemp: mktemp: $after: mkdist.$done}
	build_test_caddy: task.build_test_caddy & {"vars": cfg.vars, build: xbuild: $after: mkdist.$done}
	build_test_docker: task.build_test_docker & {"vars": cfg.vars, mktemp: mktemp: $after: mkdist.$done}

	pass: build_test_cli.kill.$done && build_test_caddy.kill.$done && build_test_docker.stop.$done

	dist: task.dist & {"vars": cfg.vars, [=~"^dist"]: rmdir: $after: pass}
	build_docker: task.build_docker & {"vars": cfg.vars, build: $after: pass}
	push_docker: task.push_docker & {"vars": cfg.vars, tags: build_docker.tags} & {[=~"^push"]: $after: build_docker.build.$done}
}
