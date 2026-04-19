package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jessevdk/go-flags"
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

func TestZiptoGzipNameSize(t *testing.T) {
	t.Parallel()
	cmd := ZiptoGzip{}
	tdata := []struct {
		name    string
		method  uint16
		fname   string
		csize   uint64
		usize   uint64
		expName string
		expSize uint64
	}{
		{"deflate", zip.Deflate, "a.txt", 10, 100, "a.txt.gz", 10 + GzipHeaderSize + GzipFooterSize},
		{"brotli", Brotli, "b.txt", 20, 200, "b.txt.br", 20},
		{"zstd", Zstd, "c.txt", 30, 300, "c.txt.zstd", 30},
		{"store", zip.Store, "d.txt", 40, 400, "d.txt", 400},
	}
	for _, tt := range tdata {
		t.Run(tt.name, func(t *testing.T) {
			fi := &zip.File{FileHeader: zip.FileHeader{Name: tt.fname, Method: tt.method, CompressedSize64: tt.csize, UncompressedSize64: tt.usize}}
			name, size := cmd.namesize(fi)
			if name != tt.expName {
				t.Error("name", name, tt.expName)
			}
			if size != tt.expSize {
				t.Error("size", size, tt.expSize)
			}
		})
	}
}

func TestZiptoGzipOutput(t *testing.T) {
	t.Parallel()
	zipname := prepare_testzip(t)
	zf, err := zip.OpenReader(zipname)
	if err != nil {
		t.Error("open zip", err)
		return
	}
	defer zf.Close()

	cmd := ZiptoGzip{}
	for _, fi := range zf.File {
		buf := &bytes.Buffer{}
		written, err := cmd.output(fi, buf)
		if err != nil {
			t.Error("output", fi.Name, fi.Method, err)
			continue
		}
		if written == 0 {
			t.Error("no output", fi.Name, fi.Method)
		}
	}
}

func TestZiptoGzipExecuteTar(t *testing.T) {
	t.Parallel()
	zipname := prepare_testzip(t)
	out := filepath.Join(t.TempDir(), "out.tar")

	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false
	globalOption.Archive = flags.Filename(zipname)

	cmd := ZiptoGzip{All: true, Tar: out, TarFormat: "GNU"}
	if err := cmd.Execute([]string{"4kb.txt"}); err != nil {
		t.Error("execute", err)
		return
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Error("stat", err)
		return
	}
	if st.Size() == 0 {
		t.Error("empty tar")
	}
}

func TestZiptoGzipExecuteNoArchive(t *testing.T) {
	t.Parallel()
	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false
	globalOption.Archive = flags.Filename("/not/found/archive.zip")

	cmd := ZiptoGzip{All: true}
	if err := cmd.Execute(nil); err == nil {
		t.Error("expected error")
	}
}
