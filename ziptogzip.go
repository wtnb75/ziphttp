package main

import (
	"archive/tar"
	"archive/zip"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type ZiptoGzip struct {
	All       bool   `short:"a" long:"all" description:"extract non-deflate file too"`
	Tar       string `short:"t" long:"tar" description:"create single .gz.tar"`
	TarFormat string `long:"tar-format" description:"format of tar file" choice:"GNU" choice:"PAX" choice:"USTAR" default:"GNU"`
}

func (cmd *ZiptoGzip) Execute(args []string) (err error) {
	init_log()
	filename := archiveFilename()
	zipfile, err := zip.OpenReader(filename)
	if err != nil {
		slog.Error("open error", "error", err)
		return err
	}
	defer zipfile.Close()
	var tarfile *tar.Writer
	var tarfp io.WriteCloser
	if cmd.Tar != "" {
		if cmd.Tar == "-" {
			slog.Debug("tar: output to stdout")
			tarfp = os.Stdout
		} else {
			slog.Debug("tar: output to file", "name", cmd.Tar)
			tarfp, err = os.Create(string(cmd.Tar))
			if err != nil {
				slog.Error("open tar", "name", cmd.Tar, "error", err)
				return err
			}
			defer tarfp.Close()
		}
		tarfile = tar.NewWriter(tarfp)
		defer tarfile.Close()
	}
	var tarformat tar.Format
	switch cmd.TarFormat {
	case "GNU":
		tarformat = tar.FormatGNU
	case "PAX":
		tarformat = tar.FormatPAX
	case "USTAR":
		tarformat = tar.FormatUSTAR
	}
	for _, i := range zipfile.File {
		if !ismatch(i.Name, args) || (!cmd.All && i.Method != zip.Deflate) {
			slog.Debug("skip", "name", i.Name, "method", i.Method)
			continue
		}
		fname := i.Name
		size := i.UncompressedSize64
		if i.Method == zip.Deflate {
			fname = i.Name + ".gz"
			size = i.CompressedSize64 + 18
		}
		if tarfile != nil {
			slog.Debug("tar write", "name", fname)
			err = tarfile.WriteHeader(&tar.Header{
				Name:    fname,
				Mode:    int64(i.Mode()),
				ModTime: i.Modified,
				Size:    int64(size),
				Format:  tarformat,
			})
			if err != nil {
				slog.Error("tar header", "name", fname, "error", err)
				return err
			}
			if i.Method == zip.Deflate {
				written, err := CopyGzip(tarfile, i)
				if err != nil {
					slog.Error("copy gzip", "error", err, "written", written)
					return err
				}
				slog.Debug("written(gzip)", "name", fname, "written", written)
			} else {
				arcfile, err := i.Open()
				if err != nil {
					slog.Error("open zip", "name", fname, "error", err)
					return err
				}
				written, err := io.Copy(tarfile, arcfile)
				if err != nil {
					slog.Error("copy", "name", fname, "error", err, "written", written)
				}
				slog.Debug("written", "name", fname, "written", written)
			}
		} else {
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
			if err != nil {
				slog.Error("copy gzip", "error", err)
				return err
			}
			err = outfp.Close()
			if err != nil {
				slog.Error("close", "error", err)
				return err
			}
		}
	}
	return nil
}
