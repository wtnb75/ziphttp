package main

import (
	"archive/zip"
	"fmt"
	"log/slog"
)

type ZipList struct {
}

func (cmd *ZipList) Execute(args []string) (err error) {
	init_log()
	var zipfile *zip.ReadCloser
	filename := archiveFilename()
	zipfile, err = zip.OpenReader(filename)
	if err != nil {
		slog.Error("open error", "error", err)
		return err
	}
	defer zipfile.Close()
	typemap := map[uint16]string{
		zip.Store:   "S",
		zip.Deflate: "D",
		Brotli:      "B",
		Bzip2:       "b",
		Lzma:        "L",
		Xz:          "X",
		Zstd:        "Z",
		Mp3:         "M",
		Jpeg:        "J",
		Webpack:     "W",
	}
	for _, i := range zipfile.File {
		pfx := "?"
		if i.FileInfo().IsDir() {
			pfx = "/"
		}
		if v, ok := typemap[i.Method]; ok {
			pfx = v
		}
		fmt.Println(pfx, i.Name, i.CompressedSize64, i.UncompressedSize64, i.Comment)
	}
	return nil
}
