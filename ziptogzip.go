package main

import (
	"archive/zip"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jessevdk/go-flags"
)

type ZiptoGzip struct {
	Archive flags.Filename `short:"f" long:"archive" description:"archive file"`
}

func (cmd *ZiptoGzip) Execute(args []string) (err error) {
	if globalOption.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	zipfile, err := zip.OpenReader(string(cmd.Archive))
	if err != nil {
		slog.Error("open error", "error", err)
		return err
	}
	defer zipfile.Close()
	for _, fn := range args {
		for _, i := range zipfile.File {
			if i.Name != fn || i.Method != zip.Deflate {
				continue
			}
			fname := i.Name + ".gz"
			if err = os.MkdirAll(filepath.Dir(fname), 0o777); err != nil {
				slog.Error("mkdir", "error", err)
				return err
			}
			outfp, err := os.OpenFile(fname, os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				slog.Error("open file", "error", err)
				return err
			}
			_, err = CopyGzip(outfp, i)
			outfp.Close()
			if err != nil {
				slog.Error("copy gzip", "error", err)
				return err
			}
		}
	}
	return nil
}
