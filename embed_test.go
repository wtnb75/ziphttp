package main

import (
	_ "embed"
	"os"
	"testing"
)

//go:embed testdata/test.zip
var testzip []byte

func prepare_testzip(t *testing.T) string {
	t.Helper()
	fp, err := os.CreateTemp(t.TempDir(), "zip*.zip")
	if err != nil {
		t.Error("CreateTemp", err)
		panic(err)
	}
	defer fp.Close()
	written, err := fp.Write(testzip)
	if err != nil {
		t.Error("WriteTmp", err)
		panic(err)
	}
	if written != len(testzip) {
		t.Error("short write?", written, len(testzip))
		panic(written)
	}
	return fp.Name()
}
