package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"testing"
)

func TestCopyGzip(t *testing.T) {
	t.Parallel()
	fpname := prepare_testzip(t)
	if fpname == "" {
		t.Error("prepare")
		return
	}
	defer os.Remove(fpname)
	zf, err := zip.OpenReader(fpname)
	if err != nil {
		t.Error("open zip", err)
		return
	}
	defer zf.Close()
	for _, zff := range zf.File {
		buf := &bytes.Buffer{}
		res, err := CopyGzip(buf, zff)
		if err != nil {
			t.Error("CopyGzip", zff.Name, err)
		}
		if res < int64(zff.CompressedSize64) {
			t.Error("short copy?", zff.Name, res)
		}
		rd, err := gzip.NewReader(buf)
		if err != nil {
			t.Error("gzip reader", zff.Name, err)
		}
		defer rd.Close()
		data, err := io.ReadAll(rd)
		if err != nil {
			t.Error("gzip read", zff.Name, err)
		}
		if len(data) != int(zff.UncompressedSize64) {
			t.Error("length mismatch", zff.Name, len(data), zff.UncompressedSize64)
		}
	}
}

func Test_ispat(t *testing.T) {
	t.Parallel()
	if ispat([]byte("hello.txt"), []string{"text/*"}) != true {
		t.Error("hello.txt")
	}
	if ispat([]byte("hello.txt"), []string{"application/json", "text/*"}) != true {
		t.Error("hello.txt(multiple)")
	}
	if ispat([]byte("hello.txt"), []string{"image/*", "application/*"}) != false {
		t.Error("hello.txt(image)")
	}
}

func Test_ismatch(t *testing.T) {
	t.Parallel()
	if ismatch("hello.txt", []string{"*.html", "hello.*"}) != true {
		t.Error("hello.txt")
	}
	if ismatch("hello.txt", []string{"*.html", "abcde.*", "image.jpg"}) != false {
		t.Error("hello.txt(mismatch)")
	}
	if ismatch("hello.txt", []string{""}) != false {
		t.Error("hello.txt(empty)")
	}
	if ismatch("", []string{"abcde"}) != false {
		t.Error("empty")
	}
}

func TestArchiveOffset(t *testing.T) {
	t.Parallel()
	fpname := prepare_testzip(t)
	if fpname == "" {
		t.Error("prepare")
		return
	}
	defer os.Remove(fpname)
	res, err := ArchiveOffset(fpname)
	if err != nil {
		t.Error("error", err)
	}
	if res != 0 {
		t.Error("offset", res, "!=", 0)
	}
}

func TestArchiveOffset2(t *testing.T) {
	t.Parallel()
	tmpf, err := os.CreateTemp(t.TempDir(), "")
	if err != nil {
		t.Error("tempfile", err)
		panic(err)
	}
	defer os.Remove(tmpf.Name())
	if err = tmpf.Truncate(1024 * 1024); err != nil {
		t.Error("truncate", err)
		panic(err)
	}
	if _, err = tmpf.Seek(1024*1024, io.SeekStart); err != nil {
		t.Error("seek", err)
		panic(err)
	}
	zw := zip.NewWriter(tmpf)
	zw.SetOffset(1024 * 1024)
	cr, err := zw.Create("hello.txt")
	if err != nil {
		t.Error("error", err)
		panic(err)
	}
	if _, err = cr.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9}); err != nil {
		t.Error("write", err)
		panic(err)
	}
	err = zw.Flush()
	if err != nil {
		t.Error("flush", err)
	}
	err = zw.Close()
	if err != nil {
		t.Error("close", err)
	}
	tmpf.Close()
	res, err := ArchiveOffset(tmpf.Name())
	if err != nil {
		t.Error("error", err)
	}
	if res != 1024*1024 {
		t.Error("offset", res, "!=", 1024*1024)
	}
}
