package main

import (
	"archive/zip"
	"bytes"
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
