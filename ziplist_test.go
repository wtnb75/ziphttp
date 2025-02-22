package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jessevdk/go-flags"
)

func TestZipList(t *testing.T) {
	fname := prepare(t)
	defer os.Remove(fname)
	orgStdout := os.Stdout
	defer func() {
		os.Stdout = orgStdout
	}()
	r, w, err := os.Pipe()
	if err != nil {
		t.Error("pipe")
		return
	}
	os.Stdout = w
	zl := ZipList{}
	globalOption.Archive = flags.Filename(fname)
	err = zl.Execute([]string{"*"})
	if err != nil {
		t.Error("execute error")
	}
	globalOption.Archive = ""
	w.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Error("readall")
	}
	if !strings.Contains(string(data), "D 128mb.txt") {
		t.Error("not found 128mb", string(data))
	}
	if !strings.Contains(string(data), "! 512b.txt") {
		t.Error("not found 512b", data)
	}
}

func TestZipListError(t *testing.T) {
	zl := ZipList{}
	globalOption.Archive = flags.Filename("not-found.zip")
	err := zl.Execute([]string{"*"})
	globalOption.Archive = ""
	if err == nil {
		t.Error("no error")
	}
}
