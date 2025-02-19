package main

import (
	"archive/zip"
	"fmt"
	"log/slog"

	"github.com/jessevdk/go-flags"
)

type ZipList struct {
	Archive flags.Filename `short:"f" long:"archive" description:"archive file"`
}

func (cmd *ZipList) Execute(args []string) (err error) {
	if globalOption.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	zipfile, err := zip.OpenReader(string(cmd.Archive))
	if err != nil {
		slog.Error("open error", "error", err)
		return err
	}
	defer zipfile.Close()
	for _, i := range zipfile.File {
		if i.FileInfo().IsDir() {
			fmt.Println("/", i.Name)
		} else if i.Method != zip.Deflate {
			fmt.Println("!", i.Name, i.CompressedSize64, i.UncompressedSize64)
		} else {
			fmt.Println("D", i.Name, i.CompressedSize64, i.UncompressedSize64)
		}
	}
	return nil
}
