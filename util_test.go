package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
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

func TestArchiveOffsetOld(t *testing.T) {
	t.Parallel()
	fname := prepare_testzip(t)
	res, err := ArchiveOffset_Old(fname)
	if err != nil {
		t.Error("ArchiveOffset_Old", err)
		return
	}
	if res < 0 {
		t.Error("offset", res)
	}
}

func TestLinkRelativeXML(t *testing.T) {
	t.Parallel()
	in := strings.NewReader(`<urlset><url><loc>http://example.com/path/to/a</loc><image src="http://example.com/path/to/image.png"/></url></urlset>`)
	out := &bytes.Buffer{}
	err := LinkRelative_xml("http://example.com/path/to/index.xml", in, out)
	if err != nil {
		t.Error("LinkRelative_xml", err)
	}
}

func TestLinkRelativeXMLPassthrough(t *testing.T) {
	t.Parallel()
	input := "<root><a href=\"http://example.com/abc\"/></root>"
	in := strings.NewReader(input)
	out := &bytes.Buffer{}
	err := LinkRelative_xml("http://example.com/path/to/index.txt", in, out)
	if err != nil {
		t.Error("passthrough", err)
		return
	}
	if out.String() != input {
		t.Error("unexpected output", out.String())
	}
}

func TestFilterCopy(t *testing.T) {
	t.Parallel()
	t.Run("with baseurl", func(t *testing.T) {
		src := strings.NewReader(`<html><body><a href="http://example.com/path/to/a">a</a></body></html>`)
		dst := &bytes.Buffer{}
		written, err := filtercopy(dst, src, "http://example.com/path/to/index.html")
		if err != nil {
			t.Error("filtercopy", err)
			return
		}
		if written == 0 {
			t.Error("no bytes written")
		}
		if !strings.Contains(dst.String(), `href="a"`) {
			t.Error("link not rewritten", dst.String())
		}
	})
	t.Run("without baseurl", func(t *testing.T) {
		srcText := "plain text"
		src := strings.NewReader(srcText)
		dst := &bytes.Buffer{}
		written, err := filtercopy(dst, src, "")
		if err != nil {
			t.Error("filtercopy no baseurl", err)
			return
		}
		if written != int64(len(srcText)) {
			t.Error("written", written)
		}
		if dst.String() != srcText {
			t.Error("mismatch", dst.String())
		}
	})
}

func TestFixLink(t *testing.T) {
	t.Parallel()
	t.Run("same host to relative", func(t *testing.T) {
		got := fix_link("http://example.com/path/to/index.html", "http://example.com/path/to/a.html")
		if got != "a.html" {
			t.Error("unexpected", got)
		}
	})
	t.Run("different host unchanged", func(t *testing.T) {
		in := "http://other.example.com/path/to/a.html"
		if got := fix_link("http://example.com/path/to/index.html", in); got != in {
			t.Error("unexpected", got)
		}
	})
	t.Run("with userinfo unchanged", func(t *testing.T) {
		in := "http://u:p@example.com/path/to/a.html"
		if got := fix_link("http://example.com/path/to/index.html", in); got != in {
			t.Error("unexpected", got)
		}
	})
	t.Run("invalid base unchanged", func(t *testing.T) {
		in := "/a.html"
		if got := fix_link("://invalid", in); got != in {
			t.Error("unexpected", got)
		}
	})
}

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf("forced read error")
}

func TestLinkRelativeHTMLParseError(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	err := LinkRelative_html("http://example.com/index.html", errReader{}, buf)
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestZipPassThru(t *testing.T) {
	t.Parallel()
	name := prepare_testzip(t)
	zr, err := zip.OpenReader(name)
	if err != nil {
		t.Error("open zip", err)
		return
	}
	defer zr.Close()

	t.Run("success", func(t *testing.T) {
		buf := &bytes.Buffer{}
		zw := zip.NewWriter(buf)
		if err := ZipPassThru(zw, zr.File[:1]); err != nil {
			t.Error("ZipPassThru", err)
		}
		if err := zw.Close(); err != nil {
			t.Error("close", err)
		}
		if buf.Len() == 0 {
			t.Error("empty output")
		}
	})

	t.Run("writer closed", func(t *testing.T) {
		zw := zip.NewWriter(&bytes.Buffer{})
		if err := zw.Close(); err != nil {
			t.Error("close", err)
			return
		}
		_ = ZipPassThru(zw, zr.File[:1])
	})
}
