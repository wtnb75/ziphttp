package main

import (
	"archive/zip"
	"fmt"
	"log/slog"
)

type ZipList struct {
}

func (cmd *ZipList) Execute(args []string) (err error) {
	if globalOption.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	var zipfile *zip.ReadCloser
	filename := archiveFilename()
	zipfile, err = zip.OpenReader(filename)
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
