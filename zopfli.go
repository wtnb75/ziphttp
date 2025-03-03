package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
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

type ZopfliZip struct {
	StripRoot bool     `short:"s" long:"strip-root" description:"strip root path"`
	Exclude   []string `short:"x" long:"exclude" description:"exclude files"`
	Stored    []string `short:"n" long:"stored" description:"non compress patterns"`
	MinSize   uint     `short:"m" long:"min-size" description:"compress minimum size" default:"512"`
	UseNormal bool     `long:"no-zopfli" description:"do not use zopfli compress"`
	UseAsIs   bool     `long:"asis" description:"copy as-is from zipfile"`
	BaseURL   string   `long:"baseurl" description:"rewrite html link to relative"`
	SiteMap   string   `long:"sitemap" description:"generate sitemap.xml"`
}

func (cmd *ZopfliZip) archive_single(path string, archivepath string, w *zip.Writer, sitemap *SiteMapRoot) error {
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
	if st.Size() < int64(cmd.MinSize) || ismatch(path, cmd.Stored) {
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
		if ispat(buf[0:buflen], cmd.Stored) {
			slog.Debug("not deflate", "name", archivepath)
			hdr.Method = zip.Store
		}
	}
	fp, err := w.CreateHeader(&hdr)
	if err != nil {
		slog.Error("zipCreate", "path", archivepath, "error", err)
		return err
	}
	new_url, err := url.JoinPath(cmd.BaseURL, archivepath)
	if err != nil {
		slog.Error("urljoin", "base", cmd.BaseURL, "path", archivepath, "error", err)
		return err
	}
	slog.Debug("url", "baseurl", cmd.BaseURL, "path", archivepath, "new_url", new_url)
	written, err := filtercopy(fp, rd, new_url)
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
	if cmd.SiteMap != "" {
		if err = sitemap.AddZip(cmd.SiteMap, &zip.File{FileHeader: hdr}); err != nil {
			slog.Error("sitemap addzip", "name", archivepath, "error", err)
		}
	}
	return nil
}

func (cmd *ZopfliZip) from_dir(root string, w *zip.Writer, sitemap *SiteMapRoot) error {
	slog.Debug("from_dir", "exclude", cmd.Exclude, "store", cmd.Stored)
	return filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
		if info.IsDir() {
			slog.Debug("isdir", "root", root, "path", path)
			return nil
		}
		if ismatch(path, cmd.Exclude) {
			slog.Debug("exclude-match", "path", path, "exclude", cmd.Exclude)
			return nil
		}
		slog.Info("walk", "root", root, "path", path, "type", info.Type(), "name", info.Name(), "error", err)
		var archivepath string
		if cmd.StripRoot {
			archivepath, err = filepath.Rel(root, path)
			if err != nil {
				slog.Error("Relpath", "root", root, "path", path, "error", err)
				return err
			}
		} else {
			archivepath = path
		}
		if err = cmd.archive_single(path, archivepath, w, sitemap); err != nil {
			slog.Error("archive", "root", root, "path", path, "archivepath", archivepath, "error", err)
			return err
		}
		return nil
	})
}

func (cmd *ZopfliZip) from_file(root string, w *zip.Writer, sitemap *SiteMapRoot) error {
	slog.Debug("from_file", "store", cmd.Stored)
	var archivepath string
	if cmd.StripRoot {
		archivepath = filepath.Base(root)
	} else {
		archivepath = root
	}
	return cmd.archive_single(root, archivepath, w, sitemap)
}

func (cmd *ZopfliZip) from_zip(root string, w *zip.Writer, sitemap *SiteMapRoot) error {
	slog.Debug("from_zip", "exclude", cmd.Exclude, "store", cmd.Stored)
	zf, err := zip.OpenReader(root)
	if err != nil {
		slog.Error("openreader", "root", root, "error", err)
		return err
	}
	defer zf.Close()
	for _, f := range zf.File {
		if ismatch(f.Name, cmd.Exclude) {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		slog.Info("walk(zip)", "name", f.Name, "fileinfo", f.FileInfo())
		method := zip.Deflate
		if f.UncompressedSize64 < uint64(cmd.MinSize) || ismatch(f.Name, cmd.Stored) {
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
			if ispat(buf[0:buflen], cmd.Stored) {
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
		new_url := cmd.BaseURL
		if cmd.BaseURL != "" {
			new_url, _ = url.JoinPath(cmd.BaseURL, fh.Name)
		}
		slog.Debug("url", "baseurl", cmd.BaseURL, "name", fh.Name, "new_url", new_url)
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
		if cmd.SiteMap != "" {
			if err = sitemap.AddZip(cmd.SiteMap, &zip.File{FileHeader: fh}); err != nil {
				slog.Error("sitemap", "name", f.Name, "error", err)
			}
		}
	}
	return nil
}

func (cmd *ZopfliZip) from_zip_asis(root string, w *zip.Writer, sitemap *SiteMapRoot) error {
	slog.Debug("from_zip", "exclude", cmd.Exclude, "store", cmd.Stored)
	zf, err := zip.OpenReader(root)
	if err != nil {
		slog.Error("openreader", "root", root, "error", err)
		return err
	}
	defer zf.Close()
	for _, f := range zf.File {
		if ismatch(f.Name, cmd.Exclude) {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		wr, err := w.CreateRaw(&f.FileHeader)
		if err != nil {
			slog.Error("CreateRaw", "error", err, "name", f.Name)
			return err
		}
		rd, err := f.OpenRaw()
		if err != nil {
			slog.Error("OpenRaw", "error", err, "name", f.Name)
			return err
		}
		written, err := io.Copy(wr, rd)
		if err != nil && err != io.EOF {
			slog.Error("copy", "error", err, "name", f.Name, "written", written)
			return err
		}
		slog.Debug("done copy", "written", written, "name", f.Name, "error", err)
		if err = w.Flush(); err != nil {
			slog.Error("flush", "error", err, "name", f.Name)
			return err
		}
		if cmd.SiteMap != "" {
			if err = sitemap.AddZip(cmd.SiteMap, &zip.File{FileHeader: f.FileHeader}); err != nil {
				slog.Error("sitemap", "name", f.Name, "error", err)
			}
		}
	}
	return nil
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
		cmd_exc, err := os.Executable()
		if err != nil {
			slog.Error("cmd", "error", err)
			return err
		}
		cmd_fp, err := os.Open(cmd_exc)
		if err != nil {
			slog.Error("cmd open", "name", cmd_exc, "error", err)
			return err
		}
		defer cmd_fp.Close()
		written, err = io.Copy(ofp, cmd_fp)
		if err != nil {
			slog.Error("cmd copy", "name", cmd_exc, "dest", globalOption.Archive, "error", err, "written", written)
			return err
		}
		slog.Debug("copy", "written", written)
		err = ofp.Sync()
		if err != nil {
			slog.Error("sync", "name", cmd_exc, "error", err)
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
	sitemap := SiteMapRoot{}
	if cmd.SiteMap != "" {
		if err = sitemap.initialize(); err != nil {
			slog.Error("sitemap initialize", "error", err)
		}
	}
	for _, dirname := range args {
		slog.Debug("adding", "path", dirname)
		st, err := os.Stat(dirname)
		if err != nil {
			slog.Error("stat", "path", dirname, "error", err)
		}
		if st.IsDir() {
			err = cmd.from_dir(dirname, zipfile, &sitemap)
			if err != nil {
				slog.Error("from_dir", "path", dirname, "error", err)
				return err
			}
			slog.Debug("done", "path", dirname)
		} else if filepath.Ext(dirname) == ".zip" {
			if cmd.UseAsIs {
				err = cmd.from_zip_asis(dirname, zipfile, &sitemap)
			} else {
				err = cmd.from_zip(dirname, zipfile, &sitemap)
			}
			if err != nil {
				slog.Error("from_zip", "path", dirname, "error", err)
			}
		} else if st.Mode().IsRegular() {
			err = cmd.from_file(dirname, zipfile, &sitemap)
			if err != nil {
				slog.Error("from_file", "path", dirname, "error", err)
			}
		}
	}
	if cmd.SiteMap != "" {
		if err != nil {
			slog.Error("baseurl + sitemap.xml", "error", err)
			return err
		}
		path, err := os.CreateTemp("", "sitemap-*.xml")
		if err != nil {
			return err
		}
		defer path.Close()
		defer os.Remove(path.Name())
		data, err := xml.MarshalIndent(sitemap, "", "  ")
		if err != nil {
			slog.Error("encode sitemap.xml", "error", err)
		}
		_, err = path.Write([]byte(xml.Header))
		if err != nil {
			slog.Error("tmp write sitemap.xml header", "error", err)
		}
		written, err := path.Write(data)
		if err != nil {
			slog.Error("tmp write sitemap.xml", "error", err)
		}
		if written != len(data) {
			slog.Error("tmp short write sitemap.xml", "written", written, "length", len(data))
		}
		err = cmd.archive_single(path.Name(), "sitemap.xml", zipfile, &SiteMapRoot{})
		if err != nil {
			slog.Error("write sitemap", "path", path.Name(), "error", err)
		}
	}
	return nil
}
