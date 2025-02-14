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
			cmd: ["bash", "-c", "go list -f {{.Version}} -m github.com/infogulch/xtemplate@\(vars.gitver) 2> /dev/null || git describe --tags --match='v*'"]
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

RunBash: exec.Run & {
	bcmd: [...string] | string
	_cmd: string
	_cmd: bcmd | strings.Join([for arg in bcmd {
		if !strings.ContainsAny(arg, "\"'$ \t\n") {
			arg
		}
		if strings.ContainsAny(arg, "\"'$ \t\n") {
			"'"+strings.Replace(arg, "'", "''", -1)+"'"
		}
	}], " ")
	cmd: ["bash", "-c", _cmd]
}

MkTemp: {
	vars: #vars

	name: string

	remove: file.RemoveAll & {path: vars.testdir+"/temp-"+name}
	mktemp: file.Mkdir & {path: remove.path, $after: remove.$done}
	copy: RunBash & {
		bcmd: ["cp", "-r", "templates/", "data/", "migrations/", "config.json", "caddy.json", remove.path]
		dir: vars.testdir
		$after: mktemp.$done
	}
}

task: testbash: {
	vars: #vars

	mktemp: MkTemp & {"vars": vars, "name": "testbash"}
	create: file.Create & {filename: mktemp.path + "/hello.txt"}
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

	mktemp: MkTemp & {"vars": vars, name: "native"}

	start: RunBash & {
		bcmd: "../xtemplate --loglevel -4 --config-file config.json &>xtemplate.log &"
		dir:    mktemp.mktemp.path
		$after: mktemp.copy.$done
	}
}

task: test: {
	vars: #vars

	port:       int | *8080
	reportpath: string | *"report"

	testfiles: file.Glob & {glob: "\(vars.testdir)/tests/*.hurl"}
	ready: RunBash & {bcmd: "curl -X GET --retry-all-errors --retry 5 --retry-connrefused --retry-delay 1 http://localhost:\(port)/ready --silent", stdout: "OK"}
	hurl: exec.Run & {
		cmd: list.Concat([["hurl", "--continue-on-error", "--no-output", "--test", "--report-html", reportpath, "--connect-to", "localhost:8080:localhost:\(port)"], testfiles.files])
		dir:   vars.testdir
		after: ready.$done
	}
}

task: gotest: {
	vars: #vars

	gotest: RunBash & {
		bcmd: "go test -v ./... >'\(vars.testdir)/gotest.log'"
		dir: vars.rootdir
		env: vars.env
	}
}

task: build_test: {
	vars: #vars

	build: task.build & {"vars": vars, outfile: "\(vars.testdir)/xtemplate"}
	run: task.run & {"vars": vars, start: $after: build.gobuild.$done}
	test: task.test & {"vars": vars, reportpath: "\(run.mktemp.mktemp.path)/report", ready: $after: run.start.$done}
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
			cp: RunBash & {bcmd: ["cp", "README.md", "LICENSE", dir], $after: mkdir.$done}
			zip: RunBash & {bcmd: ["zip", "-jqr6", "\(dir)_\(vars.version).zip", dir], $after: cp.$done & build.gobuild.$done}
		}
	}
}

task: build_docker: {
	vars: #vars

	tags: [
		"infogulch/xtemplate:\(vars.version)",
		if vars.version == vars.latest {"infogulch/xtemplate:latest"},
	]

	build: RunBash & {
		bcmd: list.Concat([["docker", "build"], list.FlattenN([for t in tags {["-t", t]}], 1), ["--build-arg", "LDFLAGS=\(vars.ldflags)", "--progress=plain", "."]])
		dir: vars.rootdir
	}
}

task: test_docker: {
	vars: #vars

	mktemp: MkTemp & {"vars": vars, name: "docker"}

	build: RunBash & {
		bcmd: ["docker", "build", "-t", "xtemplate-test", "--target", "test", "--build-arg", "LDFLAGS=\(vars.ldflags)", "."]
		dir: vars.rootdir
		env: vars.env
	}
	run: RunBash & {
		bcmd: ["docker", "run", "-d", "--rm", "--name", "xtemplate-test", "-p", "8081:80", "-v", mktemp.mktemp.path+":/app/dataw", "xtemplate-test"]
		$after: build.$done && mktemp.copy.$done
	}
	logs: RunBash & {
		bcmd: "docker logs xtemplate-test &>docker.log"
		dir:    mktemp.mktemp.path
		$after: run.$done
	}
	test: task.test & {"vars": vars, port: 8081, reportpath: "\(mktemp.mktemp.path)/report", ready: $after: run.$done}
	stop: exec.Run & {cmd: "docker stop xtemplate-test", $after: test.hurl.$done} // be nice if we can always run this even if previous steps fail
}

task: push_docker: {
	tags: [...string]

	for tag in tags {
		("push-" + tag): exec.Run & {
			cmd: ["docker", "push", tag]
			mustSucceed: false
		}
	}
}

task: build_caddy: {
	vars: #vars

	flag: *"" | string @tag(debug,short=debug)

	xbuild: RunBash & {
		bcmd: "xcaddy build "+
			"--with github.com/infogulch/xtemplate-caddy "+
			"--with github.com/infogulch/xtemplate=. "+
			"--with github.com/mattn/go-sqlite3 "+
			"--output '\(vars.testdir)/caddy' "+
			"&>'\(vars.testdir)/xcaddy.log'"
		dir: vars.rootdir
		env: vars.env & {
			CGO_ENABLED: "1"
			if flag == "debug" {XCADDY_DEBUG: "1"}
		}
	}
}

task: run_caddy: {
	vars: #vars

	mktemp: MkTemp & {"vars": vars, name: "caddy"}

	start: RunBash & {
		bcmd: "../caddy start --config ./caddy.json &>caddy.log"
		dir:    mktemp.mktemp.path
		$after: mktemp.copy.$don
	}
}

task: build_test_caddy: {
	vars: #vars

	build: task.build_caddy & {"vars": vars}
	run: task.run_caddy & {"vars": vars, start: $after: build.xbuild.$done}
	test: task.test & {"vars": vars, port: 8082, reportpath: "\(run.mktemp.mktemp.path)/report", ready: $after: run.start.$done}
	kill: exec.Run & {cmd: "pkill caddy", $after: test.hurl.$done} // is there a better way?
}

command: {
	for k, t in task {
		(k): {cfg: meta, vars: cfg.vars, t}
	}
}

command: ci: {
	cfg: meta
	vars: #vars & cfg.vars

	gotest: task.gotest & {"vars": vars}

	build_test: task.build_test & {"vars": vars}
	build_test_caddy: task.build_test_caddy & {"vars": vars}
	test_docker: task.test_docker & {"vars": vars}

	pass: build_test.kill.$done && build_test_caddy.kill.$done && test_docker.stop.$done

	dist: task.dist & {"vars": vars, rmdist: $after: pass}
	build_docker: task.build_docker & {"vars": vars, build: $after: pass}
	push_docker: task.push_docker & {tags: build_docker.tags} & {[=~"^push"]: $after: build_docker.build.$done}
}
