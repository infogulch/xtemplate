package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Hellseher/go-shellquote"
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("exiting because: %v\n", err)
		}
	}()

	_, file, _, _ := runtime.Caller(0)
	testdir := filepath.Dir(file)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hurl":
			goto hurl
		}
	}

	// Build xtemplate
	{
		args := split(`go build -o xtemplate ../cmd`)
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stdout
		cmd.Dir = testdir
		try0(cmd.Run(), "go build")
		fmt.Println("~ Build ~")
	}

	// Run xtemplate, wait until its ready, exit test if it fails early
	{
		args := split(`./xtemplate --loglevel -4 -d DB:sql:sqlite:file:test.sqlite -d FS:fs:./context`)
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = testdir

		logpath := filepath.Join(testdir, "xtemplate.log")
		log := try(os.Create(logpath))("open log file")
		try(log.Seek(0, 0))("seek to beginning")
		defer log.Close()
		cmd.Stdout = log
		cmd.Stderr = log

		try0(cmd.Start(), "start xtemplate")
		defer kill(cmd)

		go func() {
			try0(cmd.Wait(), "wait for xtemplate")
			time.Sleep(time.Second)
			panic("xtemplate exited")
		}()

		waitUntilFileContainsString(logpath, "starting server")

		fmt.Println("~ Run xtemplate ~")
	}

hurl:
	{
		files := try(fs.Glob(os.DirFS(testdir), "tests/*.hurl"))("glob files")
		args := split("hurl --continue-on-error --test --report-html report")
		cmd := exec.Command(args[0], append(args[1:], files...)...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = testdir
		defer kill(cmd)
		try0(cmd.Run(), "run hurl")
		fmt.Println("~ Run hurl ~")
	}
}

func split(a string) []string { return try(shellquote.Split(a))("split args") }

func kill(c *exec.Cmd) {
	err := c.Process.Kill()
	if err != nil && err != os.ErrProcessDone {
		panic(fmt.Sprintf("failed to kill %s: %v", c.Path, err))
	}
}

func try[T any](t T, err error) func(string) T {
	return func(desc string) T {
		if err != nil {
			panic(fmt.Sprintf("failed to %s: %v", desc, err))
		}
		return t
	}
}

func try0(err error, desc string) {
	if err != nil {
		panic(fmt.Sprintf("failed to %s: %v\n", desc, err))
	}
}

func waitUntilFileContainsString(filename string, needle string) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		for {
			if strings.Contains(string(try(os.ReadFile(filename))("read file")), needle) {
				wg.Done()
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	wg.Wait()
}
