package main

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
)

type testZipWriter struct {
	method uint16
	comp   zip.Compressor
}

func (t *testZipWriter) RegisterCompressor(method uint16, comp zip.Compressor) {
	t.method = method
	t.comp = comp
}

func TestMakeZstdWriter(t *testing.T) {
	t.Parallel()
	tw := &testZipWriter{}
	MakeZstdWriter(tw, -1)
	if tw.comp == nil {
		// !cgo build uses no-op implementation.
		return
	}
	if tw.method != Zstd {
		t.Error("method", tw.method)
	}
	wr, err := tw.comp(&bytes.Buffer{})
	if err != nil {
		t.Error("compressor", err)
		return
	}
	if _, err = wr.Write([]byte("hello")); err != nil {
		t.Error("write", err)
	}
	if err = wr.Close(); err != nil {
		t.Error("close", err)
	}
}

func TestMakeZstdWriterExplicitLevel(t *testing.T) {
	t.Parallel()
	tw := &testZipWriter{}
	MakeZstdWriter(tw, 3)
	if tw.comp == nil {
		// !cgo build uses no-op implementation.
		return
	}
	wr, err := tw.comp(&bytes.Buffer{})
	if err != nil {
		t.Error("compressor", err)
		return
	}
	if _, err = wr.Write([]byte("hello")); err != nil {
		t.Error("write", err)
	}
	if err = wr.Close(); err != nil {
		t.Error("close", err)
	}
}

func TestZstdRoundTrip(t *testing.T) {
	t.Parallel()
	tw := &testZipWriter{}
	MakeZstdWriter(tw, -1)
	if tw.comp == nil {
		// !cgo build uses no-op implementation; init() also never registers
		// a real decompressor, so there's nothing to round-trip.
		return
	}
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	zw.RegisterCompressor(Zstd, tw.comp)
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: "hello.txt", Method: Zstd})
	if err != nil {
		t.Fatal("create header", err)
	}
	want := []byte("hello world, zstd round trip")
	if _, err := fw.Write(want); err != nil {
		t.Fatal("write", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal("close writer", err)
	}
	// init()'s zip.RegisterDecompressor(Zstd, ...) is global, so opening
	// through the standard archive/zip.Reader exercises it directly.
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal("open reader", err)
	}
	if len(zr.File) != 1 {
		t.Fatalf("expected 1 file, got %d", len(zr.File))
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatal("open entry", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal("read entry", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("round trip mismatch: got %q, want %q", got, want)
	}
}
