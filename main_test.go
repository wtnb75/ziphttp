package main

import (
	"testing"
)

func TestArchive(t *testing.T) {
	globalOption.Archive = "hello.zip"
	globalOption.Self = false
	if archiveFilename() != "hello.zip" {
		t.Error("hello.zip")
	}
	globalOption.Archive = ""
	globalOption.Self = false
}

func TestSelf(t *testing.T) {
	globalOption.Archive = "hello.zip"
	globalOption.Self = true
	if archiveFilename() == "hello.zip" {
		t.Error("hello.zip(false)")
	}
	globalOption.Archive = ""
	globalOption.Self = false
}
