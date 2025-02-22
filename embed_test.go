package main

import (
	_ "embed"
	"os"
	"testing"
)

//go:embed testdata/test.zip
var testzip []byte

func prepare(t *testing.T) string {
	fp, err := os.CreateTemp("", "zip")
	if err != nil {
		t.Error("CreateTemp", err)
		return ""
	}
	defer fp.Close()
	written, err := fp.Write(testzip)
	if err != nil {
		t.Error("WriteTmp", err)
		return ""
	}
	if written != len(testzip) {
		t.Error("short write?", written, len(testzip))
		return ""
	}
	return fp.Name()
}
