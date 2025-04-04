package main

import (
	"archive/zip"
	"io"
	"log/slog"

	"github.com/andybalholm/brotli"
)

func MakeBrotliWriter(zipfile MyZipWriter, level int) {
	slog.Debug("set compression level for brotli(0 to 11)", "level", level)
	zipfile.RegisterCompressor(Brotli, func(out io.Writer) (io.WriteCloser, error) {
		if level != -1 {
			return brotli.NewWriterLevel(out, level), nil
		}
		return brotli.NewWriter(out), nil
	})
}

func init() {
	zip.RegisterDecompressor(Brotli, func(input io.Reader) io.ReadCloser {
		return io.NopCloser(brotli.NewReader(input))
	})
}
