package main

import (
	"os"
	"strings"
	"testing"
)

func TestZipList(t *testing.T) {
	fname := prepare_testzip(t)
	defer os.Remove(fname)
	stdout, _ := runcmd_test(t, []string{"ziphttp", "ziplist", "-f", fname}, 0)
	if !strings.Contains(stdout, "D 128mb.txt") {
		t.Error("not found 128mb", stdout)
	}
	if !strings.Contains(stdout, "! 512b.txt") {
		t.Error("not found 512b", stdout)
	}
}

func TestZipListError(t *testing.T) {
	_, stderr := runcmd_test(t, []string{"ziphttp", "ziplist", "-f", "not-found.zip"}, 1)
	if !strings.Contains(stderr, "no such file or directory") {
		t.Error("not found")
	}
}
