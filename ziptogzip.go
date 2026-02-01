package main

import (
	"archive/tar"
	"archive/zip"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type ZiptoGzip struct {
	All       bool   `short:"a" long:"all" description:"extract non-deflate file too"`
	Tar       string `short:"t" long:"tar" description:"create single .gz.tar"`
	TarFormat string `long:"tar-format" description:"format of tar file" choice:"GNU" choice:"PAX" choice:"USTAR" default:"GNU"`
}

func (cmd *ZiptoGzip) namesize(fi *zip.File) (string, uint64) {
	switch fi.Method {
	case zip.Deflate:
		return fi.Name + ".gz", fi.CompressedSize64 + GzipHeaderSize + GzipFooterSize
	case Brotli:
		return fi.Name + ".br", fi.CompressedSize64
	case Zstd:
		return fi.Name + ".zstd", fi.CompressedSize64
	case Lzma:
		return fi.Name + ".lzma", fi.CompressedSize64
	case Bzip2:
		return fi.Name + ".bz2", fi.CompressedSize64
	case Xz:
		return fi.Name + ".xz", fi.CompressedSize64
	case Jpeg:
		return fi.Name + ".jpeg", fi.CompressedSize64
	case Mp3:
		return fi.Name + ".mp3", fi.CompressedSize64
	case Webpack:
		return fi.Name + ".wv", fi.CompressedSize64
	}
	return fi.Name, fi.UncompressedSize64
}

func (cmd *ZiptoGzip) output(fi *zip.File, ofp io.Writer) (int64, error) {
	switch fi.Method {
	case zip.Deflate:
		return CopyGzip(ofp, fi)
	case Brotli, Zstd, Mp3, Xz, Lzma, Jpeg, Webpack:
		arcfile, err := fi.OpenRaw()
		if err != nil {
			slog.Error("open zip", "name", fi.Name, "error", err)
			return 0, err
		}
		return io.Copy(ofp, arcfile)
	default:
		arcfile, err := fi.Open()
		if err != nil {
			slog.Error("open zip", "name", fi.Name, "error", err)
			return 0, err
		}
		defer arcfile.Close()
		return io.Copy(ofp, arcfile)
	}
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
		if len(args) != 0 && !ismatch(i.Name, args) {
			slog.Debug("skip", "name", i.Name, "method", i.Method)
			continue
		}
		if strings.Contains(i.Name, "..") {
			slog.Warn("skip suspicious file", "name", i.Name)
			continue
		}
		fname, size := cmd.namesize(i)
		if fname == i.Name && !cmd.All {
			continue
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
			written, err := cmd.output(i, tarfile)
			if err != nil {
				slog.Error("copy", "error", err, "written", written)
				return err
			}
			slog.Debug("written", "name", fname, "written", written)
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
			written, err := cmd.output(i, outfp)
			if err != nil {
				slog.Error("copy", "error", err, "written", written)
				return err
			}
			slog.Debug("written", "name", fname, "written", written)
			err = outfp.Close()
			if err != nil {
				slog.Error("close", "error", err)
				return err
			}
		}
	}
	return nil
}
