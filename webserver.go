package main

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

type ZipFile interface {
	File(index int) *zip.File
	Open(name string) (fs.File, error)
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
	accesslog   *slog.Logger
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

func (h *ZipHandler) handle_gzip(w http.ResponseWriter, filestr *zip.File, etag string) {
	slog.Debug("compressed response", "length", filestr.CompressedSize64, "original", filestr.UncompressedSize64)
	w.Header().Add("Content-Encoding", "gzip")
	w.Header().Add("Last-Modified", filestr.Modified.Format(http.TimeFormat))
	w.Header().Add("Content-Length", strconv.FormatUint(filestr.CompressedSize64+18, 10))
	if etag != "" {
		w.Header().Add("Etag", etag)
	}
	if written, err := CopyGzip(w, filestr); err != nil {
		slog.Error("copygzip", "written", written, "error", err)
	} else {
		slog.Debug("written", "written", written)
	}
}

func (h *ZipHandler) handle_normal(w http.ResponseWriter, urlpath string, filestr *zip.File, etag string) {
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
	if written, err := io.Copy(w, f); err != nil {
		slog.Error("copy error", "error", err, "written", written)
	} else {
		slog.Debug("copy success", "written", written)
	}
}

func conditional(r *http.Request, etag string, fi *zip.File) bool {
	ifnonematch := r.Header.Get("If-None-Match")
	if ifnonematch == etag {
		return true
	}
	if ifnonematch == "" {
		ifmodified, err := time.Parse(http.TimeFormat, r.Header.Get("If-Modified-Since"))
		if err == nil {
			return !fi.Modified.After(ifmodified)
		}
	}
	return false
}

func make_contenttype(ctype string) string {
	if mtype, param, err := mime.ParseMediaType(ctype); err == nil {
		return mime.FormatMediaType(mtype, param)
	}
	return ""
}

func make_contentbyext(fname string) string {
	return mime.TypeByExtension(path.Ext(fname))
}

func (h *ZipHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	statuscode := http.StatusOK
	if h.accesslog != nil {
		start := time.Now()
		defer func() {
			headers := []any{
				"remote", r.RemoteAddr, "elapsed", time.Since(start),
				"method", r.Method, "path", r.URL.Path,
				"status", statuscode, "protocol", r.Proto,
			}
			if r.URL.User.Username() != "" {
				headers = append(headers, "user", r.URL.User.Username())
			}
			for k, v := range w.Header() {
				switch strings.ToLower(k) {
				case "etag", "content-type", "content-encoding":
					headers = append(headers, strings.ToLower(k), v[0])
				case "content-length":
					if val, err := strconv.Atoi(v[0]); err != nil {
						headers = append(headers, "length", v[0])
					} else {
						headers = append(headers, "length", val)
					}
				case "last-modified":
					if ts, err := time.Parse(http.TimeFormat, v[0]); err != nil {
						headers = append(headers, "last-modified", v[0])
					} else {
						headers = append(headers, "last-modified", ts)
					}
				}
			}
			for k, v := range r.Header {
				switch strings.ToLower(k) {
				case "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto":
					headers = append(headers, strings.TrimPrefix(strings.ToLower(k), "x-"), v[0])
				case "forwarded", "user-agent", "if-none-match", "referer":
					headers = append(headers, strings.ToLower(k), v[0])
				}
			}
			h.accesslog.Info(
				http.StatusText(statuscode), headers...)
		}()
	}
	h.rwlock.RLock()
	defer h.rwlock.RUnlock()
	fname := h.filename(r)
	if strings.HasSuffix(fname, ".gz") {
		if idx, ok := h.deflmap[strings.TrimSuffix(fname, ".gz")]; ok {
			slog.Debug("gzip file", "name", fname)
			fi := h.zipfile.File(idx)
			etag := "W/" + strconv.FormatUint(uint64(fi.CRC32), 16)
			w.Header().Set("Content-Type", "application/gzip")
			w.Header().Set("Etag", etag+"_gz")
			CopyGzip(w, fi)
			return
		}
	}
	encodings, has_gzip := h.accept_encoding(r)
	if has_gzip {
		slog.Debug("gzip encoding supported", "header", encodings)
	}
	slog.Debug("name", "uri", r.URL.Path, "name", fname)
	if has_gzip {
		if idx, ok := h.deflmap[fname]; ok {
			fi := h.zipfile.File(idx)
			if fi.Flags&0x1 == 1 {
				// encrypted
				slog.Warn("encrypted", "name", fname, "flag", fi.Flags)
			}
			// fast path
			ctype := make_contenttype(fi.Comment)
			if ctype == "" {
				ctype = make_contentbyext(fname)
			}
			if ctype != "" {
				w.Header().Set("Content-Type", ctype)
			}
			for k, v := range h.headers {
				w.Header().Set(k, v)
			}
			etag := "W/" + strconv.FormatUint(uint64(fi.CRC32), 16)
			if conditional(r, etag, fi) {
				statuscode = http.StatusNotModified
				w.Header().Add("Etag", etag)
				w.Header().Add("Last-Modified", fi.Modified.Format(http.TimeFormat))
				w.WriteHeader(statuscode)
				return
			}
			h.handle_gzip(w, fi, etag)
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
		fi := h.zipfile.File(idx)
		if fi.Flags&0x1 == 1 {
			// encrypted
			slog.Warn("encrypted", "name", fname, "flag", fi.Flags)
		}
		ctype := make_contenttype(fi.Comment)
		if ctype != "" {
			w.Header().Set("Content-Type", ctype)
		}
		for k, v := range h.headers {
			w.Header().Set(k, v)
		}
		etag := "W/" + strconv.FormatUint(uint64(fi.CRC32), 16)
		if conditional(r, etag, fi) {
			statuscode = http.StatusNotModified
			w.Header().Add("Etag", etag)
			w.Header().Add("Last-Modified", fi.Modified.Format(http.TimeFormat))
			w.WriteHeader(statuscode)
			return
		}
		h.handle_normal(w, r.URL.Path, fi, etag)
		return
	}
	statuscode = http.StatusNotFound
	w.WriteHeader(statuscode)
	fmt.Fprint(w, "not found")
}

func (h *ZipHandler) init2() {
	h.deflmap = map[string]int{}
	h.storemap = map[string]int{}
	for i := 0; i < h.zipfile.Files(); i++ {
		fi := h.zipfile.File(i)
		offset, err := fi.DataOffset()
		slog.Debug("file", "n", i, "offset", offset, "error", err)
		switch {
		case fi.FileInfo().IsDir():
			slog.Debug("isdir", "name", fi.Name)
			continue
		case fi.Method == zip.Deflate:
			slog.Debug("isdeflate", "name", fi.Name)
			h.deflmap[fi.Name] = i
		default:
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
		if _, err = fp.Seek(offs, io.SeekStart); err != nil {
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
		if err = h.initialize_memory(buf); err != nil {
			slog.Error("initialize failed", "err", err)
			return err
		}
	} else {
		if err := h.initialize_file(filename); err != nil {
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
	SupportGzip       bool     `long:"support-gz" description:"support *.gz URL"`
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
		accesslog:   slog.With("type", "accesslog"),
	}

	if err = cmd.handler.initialize(archiveFilename(), cmd.InMemory); err != nil {
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
		if kv := strings.SplitN(hdr, ":", 2); len(kv) != 2 {
			slog.Error("invalid header spec", "header", hdr)
			return fmt.Errorf("invalid header: %s", hdr)
		} else {
			cmd.handler.headers[kv[0]] = strings.TrimSpace(kv[1])
		}
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
					slog.Info("got watcher event", "event", event, "op", event.Op.String())
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
