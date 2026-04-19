package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSafeOutputPathNormal(t *testing.T) {
	base := t.TempDir()
	got, err := safeOutputPath(base, "a/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	baseResolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(baseResolved, "a", "b.txt")
	if got != want {
		t.Fatalf("path mismatch: got=%s want=%s", got, want)
	}
}

func TestSafeOutputPathTraversal(t *testing.T) {
	base := t.TempDir()
	if _, err := safeOutputPath(base, "../x"); err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestSafeOutputPathAbsolute(t *testing.T) {
	base := t.TempDir()
	if _, err := safeOutputPath(base, "/tmp/x"); err == nil {
		t.Fatal("expected absolute path error")
	}
}

func TestSafeOutputPathRejectSymlinkComponent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on windows")
	}
	base := t.TempDir()
	target := t.TempDir()
	link := filepath.Join(base, "a")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	_, err := safeOutputPath(base, filepath.Join("a", "passwd"))
	if err == nil {
		t.Fatal("expected symlink component error")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("unexpected error: %v", err)
	}
}
