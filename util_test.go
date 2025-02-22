package main

import (
	"archive/zip"
	"compress/gzip"
	"io"
	"os"

	"bytes"
	"testing"
)

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

func TestCopyGzip(t *testing.T) {
	fpname := prepare(t)
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
	if ispat("hello.txt", []byte{}, []string{"*.txt"}) != true {
		t.Error("hello.txt")
	}
	if ispat("hello.txt", []byte{}, []string{"*.html", "*.css", "*.txt"}) != true {
		t.Error("hello.txt(multiple)")
	}
	if ispat("hello.txt.gz", []byte{}, []string{"*.html", "*.css", "*.txt"}) != false {
		t.Error("hello.txt.gz")
	}
}

func Test_ispat2(t *testing.T) {
	if ispat("hello.txt", []byte("hello.txt"), []string{"text/*"}) != true {
		t.Error("hello.txt")
	}
	if ispat("hello.txt", []byte("hello.txt"), []string{"application/json", "text/*"}) != true {
		t.Error("hello.txt(multiple)")
	}
	if ispat("hello.txt", []byte("hello.txt"), []string{"image/*", "application/*"}) != false {
		t.Error("hello.txt(image)")
	}
}

func Test_ismatch(t *testing.T) {
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
