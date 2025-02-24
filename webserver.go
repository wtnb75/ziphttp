package main

import (
	"bytes"
	"context"
	_ "embed"
	"io/fs"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/fsnotify/fsnotify"
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
	stripprefix string
	addprefix   string
	indexname   string
	headers     map[string]string
	deflmap     map[string]int
	storemap    map[string]int
	rwlock      sync.RWMutex
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
	} else if fname == "" {
		fname = "/" + h.indexname
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

func (h *ZipHandler) handle_normal(w http.ResponseWriter, urlpath string, idx int, etag string) {
	filestr := h.zipfile.File(idx)
	if etag != "" {
		w.Header().Add("Etag", etag)
	}
	f, err := filestr.Open()
	if err != nil {
		slog.Info("open failed", "path", urlpath, "error", err)
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
		return
	}
	defer f.Close()
	if filestr.FileInfo().IsDir() {
		slog.Info("redirect directory", "path", urlpath)
		w.Header().Add("Location", urlpath+"/")
		w.WriteHeader(http.StatusMovedPermanently)
		return
	}
	slog.Debug("normal response", "length", filestr.UncompressedSize64)
	w.Header().Add("Last-Modified", filestr.Modified.Format(http.TimeFormat))
	w.Header().Add("Content-Length", strconv.FormatUint(filestr.UncompressedSize64, 10))
	written, err := io.Copy(w, f)
	if err != nil {
		slog.Error("copy error", "error", err, "written", written)
		return
	}
	slog.Debug("copy success", "written", written)
}

func (h *ZipHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.rwlock.RLock()
	defer h.rwlock.RUnlock()
	encodings, has_gzip := h.accept_encoding(r)
	if has_gzip {
		slog.Debug("gzip encoding supported", "header", encodings)
	}
	fname := h.filename(r)
	slog.Debug("name", "uri", r.URL.Path, "name", fname)
	if has_gzip {
		if idx, ok := h.deflmap[fname]; ok {
			if h.zipfile.File(idx).Flags&0x1 == 1 {
				// encrypted
				slog.Warn("encrypted", "name", fname, "flag", h.zipfile.File(idx).Flags)
			}
			// fast path
			etag := "W/" + strconv.FormatUint(uint64(h.zipfile.File(idx).CRC32), 16)
			for k, v := range h.headers {
				w.Header().Set(k, v)
			}
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			h.handle_gzip(w, idx, etag)
			return
		}
		// pass through
	}
	// slow path
	idx, ok := h.deflmap[fname]
	if !ok {
		idx, ok = h.storemap[fname]
	}
	if ok {
		if h.zipfile.File(idx).Flags&0x1 == 1 {
			// encrypted
			slog.Warn("encrypted", "name", fname, "flag", h.zipfile.File(idx).Flags)
		}
		etag := "W/" + strconv.FormatUint(uint64(h.zipfile.File(idx).CRC32), 16)
		for k, v := range h.headers {
			w.Header().Set(k, v)
		}
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		h.handle_normal(w, r.URL.Path, idx, etag)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, "not found")
}

func (h *ZipHandler) init2() {
	h.deflmap = map[string]int{}
	h.storemap = map[string]int{}
	for i := 0; i < h.zipfile.Files(); i++ {
		fi := h.zipfile.File(i)
		offset, err := fi.DataOffset()
		slog.Debug("file", "n", i, "offset", offset, "error", err)
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

func (h *ZipHandler) initialize_file(archive string) error {
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

func (h *ZipHandler) initialize(filename string, inmemory bool) error {
	h.rwlock.Lock()
	defer h.rwlock.Unlock()
	if inmemory {
		offs, err := ArchiveOffset(filename)
		if err != nil {
			slog.Error("archiveoffset", "file", filename, "error", err)
			return err
		}
		fp, err := os.Open(filename)
		if err != nil {
			slog.Error("open file to memory", "file", filename, "error", err)
			return err
		}
		_, err = fp.Seek(offs, io.SeekStart)
		if err != nil {
			slog.Error("seek", "file", filename, "error", err)
			return err
		}
		buf, err := io.ReadAll(fp)
		if err != nil {
			slog.Error("read file to memory", "file", filename, "error", err)
			return err
		}
		fp.Close()
		slog.Debug("memory size", "file", filename, "size", len(buf))
		err = h.initialize_memory(buf)
		if err != nil {
			slog.Error("initialize failed", "err", err)
			return err
		}
	} else {
		err := h.initialize_file(filename)
		if err != nil {
			slog.Error("initialize failed", "err", err)
			return err
		}
	}
	return nil
}

type WebServer struct {
	Listen            string   `short:"l" long:"listen" default:":3000" description:"listen address:port"`
	IndexFilename     string   `long:"index" description:"index filename" default:"index.html"`
	StripPrefix       string   `long:"stripprefix" description:"strip prefix from archive"`
	AddPrefix         string   `long:"addprefix" description:"add prefix to URL path"`
	ReadTimeout       string   `long:"read-timeout" default:"10s"`
	ReadHeaderTimeout string   `long:"read-header-timeout" default:"10s"`
	InMemory          bool     `long:"in-memory" description:"load zip to memory"`
	Headers           []string `short:"H" long:"header" description:"custom response headers"`
	AutoReload        bool     `long:"autoreload" description:"detect zip file change and reload"`
	server            http.Server
	handler           ZipHandler
}

func (cmd *WebServer) Execute(args []string) (err error) {
	init_log()
	slog.Info("args", "args", args)
	cmd.handler = ZipHandler{
		zipfile:     nil,
		stripprefix: cmd.StripPrefix,
		addprefix:   cmd.AddPrefix,
		indexname:   cmd.IndexFilename,
		deflmap:     make(map[string]int),
		storemap:    make(map[string]int),
		headers:     make(map[string]string),
	}

	err = cmd.handler.initialize(archiveFilename(), cmd.InMemory)
	if err != nil {
		slog.Error("initialize failed", "error", err)
		return err
	}
	defer cmd.handler.Close()
	slog.Info("open success", "files", cmd.handler.zipfile.Files(), "deflate", len(cmd.handler.deflmap))
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
	for _, hdr := range cmd.Headers {
		kv := strings.SplitN(hdr, ":", 2)
		if len(kv) != 2 {
			slog.Error("invalid header spec", "header", hdr)
			return fmt.Errorf("invalid header: %s", hdr)
		}
		cmd.handler.headers[kv[0]] = strings.TrimSpace(kv[1])
	}
	cmd.server = http.Server{
		Addr:              cmd.Listen,
		Handler:           nil,
		ReadTimeout:       rto,
		ReadHeaderTimeout: rhto,
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo),
	}
	http.Handle("/", &cmd.handler)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		var err error
		for {
			sig := <-sigs
			slog.Info("caught signal", "signal", sig)
			switch sig {
			case syscall.SIGHUP:
				if err = cmd.Reload(); err != nil {
					slog.Error("reload failed", "error", err)
					return
				}
			case syscall.SIGINT, syscall.SIGTERM:
				if err = cmd.Shutdown(); err != nil {
					slog.Error("terminate failed", "error", err)
				}
				return
			}
		}
	}()

	if cmd.AutoReload {
		wt, err := fsnotify.NewWatcher()
		if err != nil {
			slog.Error("watcher", "error", err)
		}
		defer wt.Close()
		go func() {
			for {
				select {
				case event, ok := <-wt.Events:
					if !ok {
						slog.Error("cannot process event", "event", event)
						return
					}
					slog.Info("got watcher event", "event", event)
					if event.Has(fsnotify.Write) {
						slog.Info("modified", "name", event.Name)
						if err = cmd.Reload(); err != nil {
							slog.Error("reload error", "error", err)
						}
					}
				case err, ok := <-wt.Errors:
					if !ok {
						slog.Error("cannot process error", "error", err)
						return
					}
					slog.Info("got watcher error", "error", err)
				}
			}
		}()

		if err = wt.Add(archiveFilename()); err != nil {
			slog.Error("watcher add", "error", err)
			return err
		}
	}

	slog.Info("server starting", "listen", cmd.server.Addr, "pid", os.Getpid())
	err = cmd.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		slog.Error("listen error", "error", err)
		return err
	}
	slog.Info("server closed", "msg", err)
	return nil
}

func (cmd *WebServer) Shutdown() error {
	slog.Info("graceful shutdown")
	return cmd.server.Shutdown(context.TODO())
}

func (cmd *WebServer) Reload() error {
	slog.Info("reloading archive", "name", archiveFilename(), "inmemory", cmd.InMemory)
	return cmd.handler.initialize(archiveFilename(), cmd.InMemory)
}
