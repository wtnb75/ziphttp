package main

import (
	"archive/zip"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/foobaz/go-zopfli/zopfli"
)

type DeflateWriteCloser struct {
	opts   zopfli.Options
	output io.Writer
}

func (d *DeflateWriteCloser) Write(in []byte) (int, error) {
	err := zopfli.DeflateCompress(&d.opts, in, d.output)
	return len(in), err
}

func (d *DeflateWriteCloser) Close() error {
	return nil
}

func archive_single(path string, archivepath string, store_pat []string, w *zip.Writer) error {
	hdr := zip.FileHeader{
		Name:   archivepath,
		Method: zip.Deflate,
	}
	st, err := os.Stat(path)
	if err != nil {
		slog.Info("stat error", "path", path, "error", err)
	} else {
		hdr.Modified = st.ModTime()
		hdr.SetMode(st.Mode())
	}
	rd, err := os.Open(path)
	if err != nil {
		slog.Error("OpenFile", "path", path, "error", err)
		return err
	}
	buf := make([]byte, 512)
	buflen, err := rd.Read(buf)
	if err != nil {
		slog.Error("ReadFile", "path", path, "error", err)
		return err
	}
	_, err = rd.Seek(0, io.SeekStart)
	if err != nil {
		slog.Error("seek", "path", path, "error", err)
		return err
	}
	slog.Debug("read head", "length", buflen)
	if ispat(archivepath, buf[0:buflen], store_pat) {
		slog.Debug("not deflate", "name", archivepath)
		hdr.Method = zip.Store
	}
	fp, err := w.CreateHeader(&hdr)
	if err != nil {
		slog.Error("zipCreate", "path", archivepath, "error", err)
		return err
	}
	written, err := io.Copy(fp, rd)
	if err != nil {
		slog.Error("Copy", "path", path, "archivepath", archivepath, "error", err, "written", written)
		return err
	}
	slog.Debug("written", "path", path, "archivepath", archivepath, "written", written)
	if err = rd.Close(); err != nil {
		slog.Error("fileClose", "path", path, "archivepath", archivepath, "error", err)
		return err
	}
	if err = w.Flush(); err != nil {
		slog.Error("zipFlush", "path", path, "archivepath", archivepath, "error", err)
		return err
	}
	return nil
}

func from_dir(root string, striproot bool, exclude []string, store_pat []string, w *zip.Writer) error {
	return filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
		slog.Info("walk", "root", root, "path", path, "type", info.Type(), "name", info.Name(), "error", err)
		if info.IsDir() {
			slog.Debug("isdir", "root", root, "path", path)
			return nil
		}
		if ismatch(path, exclude) {
			return nil
		}
		var archivepath string
		if striproot {
			archivepath, err = filepath.Rel(root, path)
			if err != nil {
				slog.Error("Relpath", "root", root, "path", path, "error", err)
				return err
			}
		} else {
			archivepath = path
		}
		if err = archive_single(path, archivepath, store_pat, w); err != nil {
			slog.Error("archive", "root", root, "path", path, "archivepath", archivepath, "error", err)
			return err
		}
		return nil
	})
}

func from_file(root string, striproot bool, store_pat []string, w *zip.Writer) error {
	var archivepath string
	if striproot {
		archivepath = filepath.Base(root)
	} else {
		archivepath = root
	}
	return archive_single(root, archivepath, store_pat, w)
}

func from_zip(root string, exclude []string, store_pat []string, w *zip.Writer) error {
	zf, err := zip.OpenReader(root)
	if err != nil {
		slog.Error("openreader", "root", root, "error", err)
		return err
	}
	defer zf.Close()
	for _, f := range zf.File {
		slog.Debug("processing", "name", f.Name, "fileinfo", f.FileInfo())
		if ismatch(f.Name, exclude) {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		rd0, err := f.Open()
		if err != nil {
			slog.Error("OpenZip", "root", root, "file", f.Name, "error", err)
			return err
		}
		buf := make([]byte, 512)
		buflen, err := rd0.Read(buf)
		if err != nil {
			slog.Error("ReadZip", "root", root, "file", f.Name, "error", err)
			return err
		}
		method := zip.Deflate
		if ispat(f.Name, buf[0:buflen], store_pat) {
			method = zip.Store
		}
		rd0.Close()
		rd, err := f.Open()
		if err != nil {
			slog.Error("OpenZip", "root", root, "file", f.Name, "error", err)
			return err
		}
		fh := zip.FileHeader{
			Name:     f.FileHeader.Name,
			Comment:  f.FileHeader.Comment,
			NonUTF8:  f.FileHeader.NonUTF8,
			Flags:    f.FileHeader.Flags,
			Method:   method,
			Modified: f.FileHeader.Modified,
		}
		wr, err := w.CreateHeader(&fh)
		if err != nil {
			slog.Error("CreateHeader", "root", root, "file", f.Name, "error", err)
			return err
		}
		written, err := io.Copy(wr, rd)
		if err != nil {
			slog.Error("Copy", "root", root, "file", f.Name, "error", err, "written", written)
			return err
		}
		err = rd.Close()
		if err != nil {
			slog.Error("Close", "root", root, "file", f.Name, "error", err)
			return err
		}
		slog.Debug("copied", "root", root, "file", f.Name, "written", written)
		w.Flush()
	}
	return nil
}

type ZopfliZip struct {
	StripRoot bool     `short:"s" long:"strip-root" description:"strip root path"`
	Exclude   []string `short:"x" long:"exclude" description:"exclude files"`
	Stored    []string `short:"n" long:"stored" description:"non compress patterns"`
}

func (cmd *ZopfliZip) Execute(args []string) (err error) {
	if globalOption.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	var mode os.FileMode
	if globalOption.Self {
		mode = 0755
	} else {
		mode = 0644
	}
	ofp, err := os.OpenFile(string(globalOption.Archive), os.O_RDWR|os.O_CREATE, mode)
	if err != nil {
		slog.Error("openOutput", "path", globalOption.Archive, "error", err)
		return err
	}
	defer ofp.Close()
	err = ofp.Truncate(0)
	if err != nil {
		slog.Error("truncate", "path", globalOption.Archive, "error", err)
		return err
	}
	var written int64
	if globalOption.Self {
		self_exc, err := os.Executable()
		if err != nil {
			slog.Error("self", "error", err)
			return err
		}
		self_fp, err := os.Open(self_exc)
		if err != nil {
			slog.Error("self open", "name", self_exc, "error", err)
			return err
		}
		defer self_fp.Close()
		written, err = io.Copy(ofp, self_fp)
		if err != nil {
			slog.Error("self copy", "name", self_exc, "dest", globalOption.Archive, "error", err, "written", written)
			return err
		}
		slog.Debug("copy", "written", written)
		err = ofp.Sync()
		if err != nil {
			slog.Error("sync", "name", self_exc, "error", err)
		}
	}
	zipfile := zip.NewWriter(ofp)
	defer zipfile.Close()
	slog.Debug("setoffiset", "written", written)
	zipfile.SetOffset(written)
	zipfile.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		opts := zopfli.DefaultOptions()
		dc := DeflateWriteCloser{opts: opts, output: out}
		return &dc, nil
	})
	for _, dirname := range args {
		slog.Debug("adding", "path", dirname)
		st, err := os.Stat(dirname)
		if err != nil {
			slog.Error("stat", "path", dirname, "error", err)
		}
		if st.IsDir() {
			err = from_dir(dirname, cmd.StripRoot, cmd.Exclude, cmd.Stored, zipfile)
			if err != nil {
				slog.Error("from_dir", "path", dirname, "error", err)
				return err
			}
			slog.Debug("done", "path", dirname)
		} else if filepath.Ext(dirname) == ".zip" {
			err = from_zip(dirname, cmd.Exclude, cmd.Stored, zipfile)
			if err != nil {
				slog.Error("from_zip", "path", dirname, "error", err)
			}
		} else if st.Mode().IsRegular() {
			err = from_file(dirname, cmd.StripRoot, cmd.Stored, zipfile)
			if err != nil {
				slog.Error("from_file", "path", dirname, "error", err)
			}
		}
	}
	return nil
}
