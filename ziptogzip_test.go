package main

import (
	"archive/zip"
	"bytes"
	"hash/crc32"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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
		{"lzma", Lzma, "d.txt", 40, 400, "d.txt.lzma", 40},
		{"bzip2", Bzip2, "e.txt", 50, 500, "e.txt.bz2", 50},
		{"xz", Xz, "f.txt", 60, 600, "f.txt.xz", 60},
		{"jpeg", Jpeg, "g.txt", 70, 700, "g.txt.jpeg", 70},
		{"mp3", Mp3, "h.txt", 80, 800, "h.txt.mp3", 80},
		{"webpack", Webpack, "i.txt", 90, 900, "i.txt.wv", 90},
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
	zipname := prepare_testzip(t)
	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false
	globalOption.Archive = flags.Filename(zipname)

	for _, tf := range []string{"GNU", "PAX", "USTAR"} {
		out := filepath.Join(t.TempDir(), "out-"+tf+".tar")
		cmd := ZiptoGzip{All: true, Tar: out, TarFormat: tf}
		if err := cmd.Execute([]string{"4kb.txt"}); err != nil {
			t.Error("execute", tf, err)
			continue
		}
		st, err := os.Stat(out)
		if err != nil {
			t.Error("stat", tf, err)
			continue
		}
		if st.Size() == 0 {
			t.Error("empty tar", tf)
		}
	}
}

func TestZiptoGzipExecuteNoArchive(t *testing.T) {
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

func TestZiptoGzipOutputRawMethod(t *testing.T) {
	t.Parallel()
	zipname := filepath.Join(t.TempDir(), "rawmethod.zip")
	raw := []byte("raw-brotli-stream")
	if err := createRawMethodZip(zipname, Brotli, "raw.dat", raw); err != nil {
		t.Error("create raw zip", err)
		return
	}
	zf, err := zip.OpenReader(zipname)
	if err != nil {
		t.Error("open zip", err)
		return
	}
	defer zf.Close()
	if len(zf.File) != 1 {
		t.Error("file count", len(zf.File))
		return
	}
	buf := &bytes.Buffer{}
	cmd := ZiptoGzip{}
	written, err := cmd.output(zf.File[0], buf)
	if err != nil {
		t.Error("output", err)
		return
	}
	if written != int64(len(raw)) {
		t.Error("written", written)
	}
	if !bytes.Equal(buf.Bytes(), raw) {
		t.Error("raw mismatch", buf.Bytes())
	}
}

func createRawMethodZip(path string, method uint16, name string, raw []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	fh := zip.FileHeader{
		Name:               name,
		Method:             method,
		Modified:           time.Unix(1, 0),
		CompressedSize64:   uint64(len(raw)),
		UncompressedSize64: uint64(len(raw)),
		CRC32:              crc32.ChecksumIEEE(raw),
	}
	w, err := zw.CreateRaw(&fh)
	if err != nil {
		_ = zw.Close()
		return err
	}
	if _, err = w.Write(raw); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}
