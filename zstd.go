//go:build cgo

package main

import (
	"archive/zip"
	"io"
	"log/slog"

	"github.com/DataDog/zstd"
)

func MakeZstdWriter(zipfile MyZipWriter, level int) {
	slog.Debug("set compression level for zstd(1 to 20)", "level", level)
	zipfile.RegisterCompressor(Zstd, func(out io.Writer) (io.WriteCloser, error) {
		if level != -1 {
			return zstd.NewWriterLevel(out, level), nil
		}
		return zstd.NewWriter(out), nil
	})
}

func init() {
	zip.RegisterDecompressor(Zstd, func(input io.Reader) io.ReadCloser {
		return zstd.NewReader(input)
	})
}
