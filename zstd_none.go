//go:build !cgo

package main

import (
	"log/slog"
)

func MakeZstdWriter(zipfile MyZipWriter, level int) {
	slog.Warn("zstd not supported")
}
