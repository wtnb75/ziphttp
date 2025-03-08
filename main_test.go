package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestArchive(t *testing.T) {
	defer func() {
		globalOption.Archive = ""
		globalOption.Self = false
	}()
	globalOption.Archive = "hello.zip"
	globalOption.Self = false
	if archiveFilename() != "hello.zip" {
		t.Error("hello.zip")
	}
}

func TestSelf(t *testing.T) {
	defer func() {
		globalOption.Archive = ""
		globalOption.Self = false
	}()
	globalOption.Archive = "hello.zip"
	globalOption.Self = true
	if archiveFilename() == "hello.zip" {
		t.Error("hello.zip(false)")
	}
}

func runcmd_test(t *testing.T, args []string, rval int) (string, string) {
	t.Helper()
	oldargs := os.Args
	oldstdout := os.Stdout
	oldstderr := os.Stderr
	oldglobalopts := globalOption
	defer func() {
		os.Args = oldargs
		os.Stdout = oldstdout
		os.Stderr = oldstderr
		globalOption = oldglobalopts
	}()
	stdio_r, stdio_w, err := os.Pipe()
	if err != nil {
		t.Error("pipe")
		return "", ""
	}
	stderr_r, stderr_w, err := os.Pipe()
	if err != nil {
		t.Error("pipe")
		return "", ""
	}
	os.Stdout = stdio_w
	os.Stderr = stderr_w
	os.Args = args
	res := realMain()
	if res != rval {
		t.Error("return value", "expected", rval, "actual", res)
	}
	if err = stdio_w.Close(); err != nil {
		t.Error("write close(stdout)")
	}
	if err = stderr_w.Close(); err != nil {
		t.Error("write close(stderr)")
	}
	stdout_output, err := io.ReadAll(stdio_r)
	if err != nil {
		t.Error("readall(stdout)")
	}
	if err = stdio_r.Close(); err != nil {
		t.Error("read close(stdout)")
	}
	stderr_output, err := io.ReadAll(stderr_r)
	if err != nil {
		t.Error("readall(stderro)")
	}
	if err = stderr_r.Close(); err != nil {
		t.Error("read close(stderr)")
	}
	stdout := string(stdout_output)
	stderr := string(stderr_output)
	t.Log("output", "stdout", stdout, "stderr", stderr)
	return stdout, stderr
}

func TestHelp(t *testing.T) {
	stdout, _ := runcmd_test(t, []string{"ziphttp", "--help"}, 0)
	if !strings.Contains(stdout, "ziphttp") {
		t.Error("no help")
	}
	if !strings.Contains(stdout, "--verbose") {
		t.Error("no --verbose option")
	}
	if !strings.Contains(stdout, "--json-log") {
		t.Error("no --json-log option")
	}
}

func TestWebServerHelp(t *testing.T) {
	stdout, _ := runcmd_test(t, []string{"ziphttp", "webserver", "--help"}, 0)
	if !strings.Contains(stdout, "index.html") {
		t.Error("help does not contains --indexname")
	}
}

func TestNoCommand(t *testing.T) {
	_, stderr := runcmd_test(t, []string{"ziphttp"}, 0)
	if !strings.Contains(stderr, "Please specify") {
		t.Error("no subcommand")
	}
	if !strings.Contains(stderr, " zip") {
		t.Error("zip")
	}
	if !strings.Contains(stderr, " webserver") {
		t.Error("webserver")
	}
}
