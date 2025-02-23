package main

import (
	"archive/zip"
	"bytes"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/foobaz/go-zopfli/zopfli"
)

type DeflateWriteCloser struct {
	opts   zopfli.Options
	output io.Writer
	buf    bytes.Buffer
}

func (d *DeflateWriteCloser) Write(in []byte) (int, error) {
	return d.buf.Write(in)
}

func (d *DeflateWriteCloser) Close() error {
	return zopfli.DeflateCompress(&d.opts, d.buf.Bytes(), d.output)
}

func filtercopy(dst io.Writer, src io.Reader, baseurl string) (int64, error) {
	if baseurl != "" {
		rpipe, wpipe := io.Pipe()
		defer rpipe.Close()
		var wg sync.WaitGroup
		wg.Add(1)
		go func(w *sync.WaitGroup) {
			defer w.Done()
			defer wpipe.Close()
			err := LinkRelative(baseurl, src, wpipe)
			if err != nil {
				slog.Error("linkrelative", "error", err, "baseurl", baseurl)
			}
		}(&wg)
		written, err := io.Copy(dst, rpipe)
		if err != nil {
			slog.Error("Copy", "baseurl", baseurl)
		}
		slog.Debug("written", "baseurl", baseurl, "written", written)
		wg.Wait()
		return written, err
	}
	return io.Copy(dst, src)
}

func archive_single(path string, archivepath string, store_pat []string, minsize uint, baseurl string, w *zip.Writer) error {
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
	if st.Size() < int64(minsize) || ismatch(path, store_pat) {
		hdr.Method = zip.Store
	} else {
		buf := make([]byte, 512)
		buflen, err := rd.Read(buf)
		if err != nil && err != io.EOF {
			slog.Error("ReadFile", "path", path, "error", err, "buflen", buflen)
			return err
		}
		_, err = rd.Seek(0, io.SeekStart)
		if err != nil {
			slog.Error("seek", "path", path, "error", err)
			return err
		}
		slog.Debug("read head", "length", buflen)
		if ispat(buf[0:buflen], store_pat) {
			slog.Debug("not deflate", "name", archivepath)
			hdr.Method = zip.Store
		}
	}
	fp, err := w.CreateHeader(&hdr)
	if err != nil {
		slog.Error("zipCreate", "path", archivepath, "error", err)
		return err
	}
	written, err := filtercopy(fp, rd, baseurl)
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

func from_dir(root string, striproot bool, exclude []string, store_pat []string, minsize uint, baseurl string, w *zip.Writer) error {
	slog.Debug("from_dir", "exclude", exclude, "store", store_pat)
	return filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
		if info.IsDir() {
			slog.Debug("isdir", "root", root, "path", path)
			return nil
		}
		if ismatch(path, exclude) {
			slog.Debug("exclude-match", "path", path, "exclude", exclude)
			return nil
		}
		slog.Info("walk", "root", root, "path", path, "type", info.Type(), "name", info.Name(), "error", err)
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
		new_baseurl := baseurl
		if baseurl != "" {
			new_baseurl, _ = url.JoinPath(baseurl, archivepath)
			slog.Debug("baseurl", "orig", baseurl, "new", new_baseurl, "archive", archivepath, "root", root, "path", path)
		}
		if err = archive_single(path, archivepath, store_pat, minsize, new_baseurl, w); err != nil {
			slog.Error("archive", "root", root, "path", path, "archivepath", archivepath, "error", err)
			return err
		}
		return nil
	})
}

func from_file(root string, striproot bool, store_pat []string, minsize uint, baseurl string, w *zip.Writer) error {
	slog.Debug("from_file", "store", store_pat)
	var archivepath string
	if striproot {
		archivepath = filepath.Base(root)
	} else {
		archivepath = root
	}
	new_baseurl := baseurl
	if baseurl != "" {
		new_baseurl, _ = url.JoinPath(baseurl, archivepath)
		slog.Debug("baseurl", "orig", baseurl, "new", new_baseurl, "archive", archivepath, "root", root)
	}
	return archive_single(root, archivepath, store_pat, minsize, new_baseurl, w)
}

func from_zip(root string, exclude []string, store_pat []string, minsize uint, baseurl string, w *zip.Writer) error {
	slog.Debug("from_zip", "exclude", exclude, "store", store_pat)
	zf, err := zip.OpenReader(root)
	if err != nil {
		slog.Error("openreader", "root", root, "error", err)
		return err
	}
	defer zf.Close()
	for _, f := range zf.File {
		if ismatch(f.Name, exclude) {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		slog.Info("walk(zip)", "name", f.Name, "fileinfo", f.FileInfo())
		method := zip.Deflate
		if f.UncompressedSize64 < uint64(minsize) || ismatch(f.Name, store_pat) {
			method = zip.Store
		} else {
			rd0, err := f.Open()
			if err != nil {
				slog.Error("OpenZip", "root", root, "file", f.Name, "error", err)
				return err
			}
			buf := make([]byte, 512)
			buflen, err := rd0.Read(buf)
			if err != nil && err != io.EOF {
				slog.Error("ReadZip", "root", root, "file", f.Name, "error", err, "buflen", buflen)
				return err
			}
			if ispat(buf[0:buflen], store_pat) {
				slog.Debug("store", "name", f.Name)
				method = zip.Store
			}
			rd0.Close()
		}
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
		new_url := baseurl
		if baseurl != "" {
			new_url, _ = url.JoinPath(baseurl, f.Name)
		}
		written, err := filtercopy(wr, rd, new_url)
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
	MinSize   uint     `short:"m" long:"min-size" description:"compress minimum size" default:"512"`
	UseNormal bool     `long:"no-zopfli" description:"do not use zopfli compress"`
	BaseURL   string   `long:"baseurl" description:"rewrite html link to relative"`
	IndexFile string   `long:"index" default:"index.html"`
}

func (cmd *ZopfliZip) Execute(args []string) (err error) {
	init_log()
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
	if !cmd.UseNormal {
		slog.Info("using zopfli compressor")
		zipfile.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
			opts := zopfli.DefaultOptions()
			dc := DeflateWriteCloser{opts: opts, output: out}
			return &dc, nil
		})
	} else {
		slog.Info("using normal compressor")
	}
	for _, dirname := range args {
		slog.Debug("adding", "path", dirname)
		st, err := os.Stat(dirname)
		if err != nil {
			slog.Error("stat", "path", dirname, "error", err)
		}
		if st.IsDir() {
			err = from_dir(dirname, cmd.StripRoot, cmd.Exclude, cmd.Stored, cmd.MinSize, cmd.BaseURL, zipfile)
			if err != nil {
				slog.Error("from_dir", "path", dirname, "error", err)
				return err
			}
			slog.Debug("done", "path", dirname)
		} else if filepath.Ext(dirname) == ".zip" {
			err = from_zip(dirname, cmd.Exclude, cmd.Stored, cmd.MinSize, cmd.BaseURL, zipfile)
			if err != nil {
				slog.Error("from_zip", "path", dirname, "error", err)
			}
		} else if st.Mode().IsRegular() {
			err = from_file(dirname, cmd.StripRoot, cmd.Stored, cmd.MinSize, cmd.BaseURL, zipfile)
			if err != nil {
				slog.Error("from_file", "path", dirname, "error", err)
			}
		}
	}
	return nil
}
