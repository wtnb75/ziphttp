package main

import (
	_ "embed"
	"io/fs"
	"time"

	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jessevdk/go-flags"
)

type ZipHandler struct {
	zipfile     *zip.ReadCloser
	autoindex   bool
	stripprefix string
	addprefix   string
	indexname   string
	deflmap     map[string]int
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
	filestr := h.zipfile.File[idx]
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
	_, err = io.Copy(w, f)
	if err != nil {
		slog.Error("write error", "error", err)
		return
	}
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
			etag := "W/" + strconv.FormatUint(uint64(h.zipfile.File[idx].CRC32), 16)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			h.handle_gzip(w, idx, etag)
			return
		}
	}
	// slow path
	h.handle_normal(w, fname, r.URL.Path)
}

func (h *ZipHandler) initialize() {
	for i := range h.zipfile.File {
		if h.zipfile.File[i].Method != zip.Deflate {
			continue
		}
		h.deflmap[h.zipfile.File[i].Name] = i
	}
}

type webserver struct {
	Listen            string         `short:"l" long:"listen" default:":8080" description:"listen address:port"`
	AutoIndex         bool           `long:"autoindex" description:"autoindex directory"`
	IndexFilename     string         `long:"index" description:"index filename"`
	Archive           flags.Filename `short:"f" long:"archive" description:"archive file"`
	StripPrefix       string         `long:"stripprefix" description:"strip prefix in archive"`
	AddPrefix         string         `long:"addprefix" description:"add prefix in URL path"`
	ReadTimeout       string         `long:"read-timeout" default:"10s"`
	ReadHeaderTimeout string         `long:"read-header-timeout" default:"10s"`
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
	}
	hdl.zipfile, err = zip.OpenReader(string(cmd.Archive))
	if err != nil {
		slog.Error("open error", "error", err)
		return err
	}
	defer hdl.zipfile.Close()
	hdl.initialize()
	slog.Info("open success", "comment", hdl.zipfile.Comment, "files", len(hdl.zipfile.File), "deflate", len(hdl.deflmap))
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
	}
	http.Handle("/", &hdl)
	err = server.ListenAndServe()
	if err != nil {
		slog.Error("listen error", "error", err)
		return err
	}
	return nil
}
