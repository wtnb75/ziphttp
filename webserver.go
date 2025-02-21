package main

import (
	"bytes"
	_ "embed"
	"io/fs"
	"os"
	"time"

	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

type ZipFile interface {
	File(int) *zip.File
	Open(string) (fs.File, error)
	Close() error
	Files() int
}

type ZipFileBytes struct {
	z *zip.Reader
}

func (z *ZipFileBytes) Open(name string) (fs.File, error) {
	return z.z.Open(name)
}

func (z *ZipFileBytes) File(idx int) *zip.File {
	return z.z.File[idx]
}

func (z *ZipFileBytes) Files() int {
	return len(z.z.File)
}

func (z *ZipFileBytes) Close() error {
	return nil
}

func NewZipFileBytes(input []byte) (*ZipFileBytes, error) {
	buf := bytes.NewReader(input)
	z, err := zip.NewReader(buf, int64(len(input)))
	if err != nil {
		return nil, err
	}
	res := ZipFileBytes{z: z}
	return &res, nil
}

type ZipFileFile struct {
	z *zip.ReadCloser
}

func (z *ZipFileFile) Open(name string) (fs.File, error) {
	return z.z.Open(name)
}

func (z *ZipFileFile) File(idx int) *zip.File {
	return z.z.File[idx]
}

func (z *ZipFileFile) Files() int {
	return len(z.z.File)
}

func (z *ZipFileFile) Close() error {
	return z.z.Close()
}

func NewZipFileFile(name string) (*ZipFileFile, error) {
	z, err := zip.OpenReader(name)
	if err != nil {
		return nil, err
	}
	res := ZipFileFile{z: z}
	return &res, nil
}

type ZipHandler struct {
	zipfile     ZipFile
	autoindex   bool
	stripprefix string
	addprefix   string
	indexname   string
	deflmap     map[string]int
	storemap    map[string]int
}

func (h *ZipHandler) accept_encoding(r *http.Request) ([]string, bool) {
	has_gzip := false
	encodings := strings.Split(r.Header.Get("Accept-Encoding"), ",")
	for i := range encodings {
		encodings[i] = strings.TrimSpace(encodings[i])
		if encodings[i] == "gzip" {
			has_gzip = true
		}
	}
	return encodings, has_gzip
}

func (h *ZipHandler) filename(r *http.Request) string {
	fname := r.URL.Path
	fname = strings.TrimPrefix(fname, h.addprefix)
	fname = h.stripprefix + fname
	if strings.HasSuffix(fname, "/") {
		fname += h.indexname
	}
	fname = strings.ReplaceAll(fname, "//", "/")
	return strings.TrimPrefix(fname, "/")
}

func (h *ZipHandler) handle_gzip(w http.ResponseWriter, idx int, etag string) {
	filestr := h.zipfile.File(idx)
	slog.Debug("compressed response", "length", filestr.CompressedSize64, "original", filestr.UncompressedSize64)
	w.Header().Add("Content-Encoding", "gzip")
	w.Header().Add("Last-Modified", filestr.Modified.Format(http.TimeFormat))
	w.Header().Add("Content-Length", strconv.FormatUint(filestr.CompressedSize64+18, 10))
	if etag != "" {
		w.Header().Add("Etag", etag)
	}
	written, err := CopyGzip(w, filestr)
	if err != nil {
		slog.Error("copygzip", "written", written, "error", err)
	} else {
		slog.Debug("written", "written", written)
	}
}

func (h *ZipHandler) handle_normal(w http.ResponseWriter, urlpath string, fname string) {
	idx, ok := h.deflmap[fname]
	if !ok {
		idx, ok = h.storemap[fname]
	}
	if ok {
		etag := "W/" + strconv.FormatUint(uint64(h.zipfile.File(idx).CRC32), 16)
		w.Header().Add("Etag", etag)
	}
	f, err := h.zipfile.Open(fname)
	if err != nil {
		slog.Info("cannot read file", "name", fname, "error", err)
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		slog.Info("stat failed", "name", fname, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal server error")
		return
	}
	if fi.IsDir() {
		if h.autoindex {
			entries, err := f.(fs.ReadDirFile).ReadDir(0)
			if err != nil {
				slog.Warn("readir", "name", fname, "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "internal server error")
				return
			}
			for _, entry := range entries {
				slog.Info("file", "name", entry.Name())
				einfo, err := entry.Info()
				if err != nil {
					slog.Warn("info", "parent", fname, "name", entry.Name, "error", err)
					continue
				}
				fmt.Fprintln(w, entry.Name(), einfo.Size())
			}
			return
		}
		slog.Info("redirect directory", "path", urlpath)
		w.Header().Add("Location", urlpath+"/")
		w.WriteHeader(http.StatusMovedPermanently)
		return
	}
	slog.Debug("normal response", "length", fi.Size())
	w.Header().Add("Last-Modified", fi.ModTime().Format(http.TimeFormat))
	w.Header().Add("Content-Length", strconv.FormatInt(fi.Size(), 10))
	written, err := io.Copy(w, f)
	if err != nil {
		slog.Error("copy error", "error", err, "written", written)
		return
	}
	slog.Debug("copy success", "written", written)
}

func (h *ZipHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	encodings, has_gzip := h.accept_encoding(r)
	if has_gzip {
		slog.Debug("gzip encoding supported", "header", encodings)
	}
	fname := h.filename(r)
	slog.Debug("name", "uri", r.URL.Path, "name", fname)
	if has_gzip {
		if idx, ok := h.deflmap[fname]; ok {
			// fast path
			etag := "W/" + strconv.FormatUint(uint64(h.zipfile.File(idx).CRC32), 16)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			h.handle_gzip(w, idx, etag)
			return
		}
	}
	// slow path
	h.handle_normal(w, r.URL.Path, fname)
}

func (h *ZipHandler) init2() {
	for i := 0; i < h.zipfile.Files(); i++ {
		fi := h.zipfile.File(i)
		if fi.FileInfo().IsDir() {
			slog.Debug("isdir", "name", fi.Name)
			continue
		} else if fi.Method == zip.Deflate {
			slog.Debug("isdeflate", "name", fi.Name)
			h.deflmap[fi.Name] = i
		} else {
			slog.Debug("store", "name", fi.Name, "method", fi.Method)
			h.storemap[fi.Name] = i
		}
	}
}

func (h *ZipHandler) initialize_memory(input []byte) error {
	var err error
	h.zipfile, err = NewZipFileBytes(input)
	if err != nil {
		return err
	}
	h.init2()
	return nil
}

func (h *ZipHandler) initialize(archive string) error {
	var err error
	h.zipfile, err = NewZipFileFile(archive)
	if err != nil {
		return err
	}
	h.init2()
	return nil
}

func (h *ZipHandler) Close() error {
	if h.zipfile != nil {
		return h.zipfile.Close()
	}
	return nil
}

type webserver struct {
	Listen            string `short:"l" long:"listen" default:":3000" description:"listen address:port"`
	AutoIndex         bool   `long:"autoindex" description:"autoindex directory"`
	IndexFilename     string `long:"index" description:"index filename" default:"index.html"`
	StripPrefix       string `long:"stripprefix" description:"strip prefix in archive"`
	AddPrefix         string `long:"addprefix" description:"add prefix in URL path"`
	ReadTimeout       string `long:"read-timeout" default:"10s"`
	ReadHeaderTimeout string `long:"read-header-timeout" default:"10s"`
	InMemory          bool   `long:"in-memory" description:"load zip to memory"`
}

func (cmd *webserver) Execute(args []string) (err error) {
	if globalOption.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	slog.Info("args", "args", args)
	hdl := ZipHandler{
		zipfile:     nil,
		autoindex:   cmd.AutoIndex,
		stripprefix: cmd.StripPrefix,
		addprefix:   cmd.AddPrefix,
		indexname:   cmd.IndexFilename,
		deflmap:     make(map[string]int),
		storemap:    make(map[string]int),
	}
	archivefile := archiveFilename()
	if cmd.InMemory {
		fp, err := os.Open(archivefile)
		if err != nil {
			slog.Error("open file to memory", "file", globalOption.Archive, "error", err)
			return err
		}
		buf, err := io.ReadAll(fp)
		if err != nil {
			slog.Error("read file to memory", "file", globalOption.Archive, "error", err)
			return err
		}
		fp.Close()
		err = hdl.initialize_memory(buf)
		if err != nil {
			slog.Error("initialize failed", "err", err)
			return err
		}
	} else {
		err = hdl.initialize(archivefile)
		if err != nil {
			slog.Error("initialize failed", "err", err)
			return err
		}
	}
	defer hdl.Close()
	slog.Info("open success", "files", hdl.zipfile.Files(), "deflate", len(hdl.deflmap))
	rto, err := time.ParseDuration(cmd.ReadTimeout)
	if err != nil {
		slog.Error("read timeout expression", "error", err)
		return err
	}
	rhto, err := time.ParseDuration(cmd.ReadHeaderTimeout)
	if err != nil {
		slog.Error("read header timeout expression", "error", err)
		return err
	}
	server := http.Server{
		Addr:              cmd.Listen,
		Handler:           nil,
		ReadTimeout:       rto,
		ReadHeaderTimeout: rhto,
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo),
	}
	http.Handle("/", &hdl)
	err = server.ListenAndServe()
	if err != nil {
		slog.Error("listen error", "error", err)
		return err
	}
	return nil
}
