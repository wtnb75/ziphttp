package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jessevdk/go-flags"
)

func TestStored(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		zipfiles:    nil,
		stripprefix: "",
		addprefix:   "",
		indexname:   "index.html",
		methodmap:   make(map[string]map[uint16]int),
	}
	if err := hdl.initialize_memory([][]byte{testzip}); err != nil {
		t.Error("initialize", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/512b.txt", bytes.NewBuffer([]byte{}))
	req.Header.Add("Accept-Encoding", "br, deflate,gzip ")
	got := httptest.NewRecorder()
	hdl.ServeHTTP(got, req)
	if got.Code != http.StatusOK {
		t.Error("got error", got.Code)
	}
	if got.Result().ContentLength != 512 {
		t.Error("length", got.Result().ContentLength)
	}
	if !strings.HasPrefix(got.Result().Header.Get("etag"), "W/") {
		t.Error("etag", got.Result().Header.Get("etag"))
	}
	if got.Result().Header.Get("Content-Encoding") == "gzip" {
		t.Error("not stored", got.Result().Header.Get("Content-Encoding"))
	}
}

func TestZipFileBytes(t *testing.T) {
	t.Parallel()
	zf, err := NewZipFileBytes(testzip)
	if err != nil {
		t.Error("new zip bytes", err)
		return
	}
	if zf.Files() == 0 {
		t.Error("empty files")
		return
	}
	fi := zf.File(0)
	if fi == nil {
		t.Error("nil file")
		return
	}
	rd, err := zf.Open(fi.Name)
	if err != nil {
		t.Error("open", err)
		return
	}
	_ = rd.Close()
	if err = zf.Close(); err != nil {
		t.Error("close", err)
	}
}

func TestZipFileFile(t *testing.T) {
	t.Parallel()
	name := prepare_testzip(t)
	zf, err := NewZipFileFile(name)
	if err != nil {
		t.Error("new zip file", err)
		return
	}
	if zf.Files() == 0 {
		t.Error("empty files")
		return
	}
	fi := zf.File(0)
	if fi == nil {
		t.Error("nil file")
		return
	}
	rd, err := zf.Open(fi.Name)
	if err != nil {
		t.Error("open", err)
		return
	}
	_ = rd.Close()
	if err = zf.Close(); err != nil {
		t.Error("close", err)
	}
}

func TestDeflate(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		zipfiles:    nil,
		stripprefix: "",
		addprefix:   "",
		indexname:   "index.html",
		methodmap:   make(map[string]map[uint16]int),
	}
	if err := hdl.initialize_memory([][]byte{testzip}); err != nil {
		t.Error("initialize", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/4kb.txt", bytes.NewBuffer([]byte{}))
	req.Header.Add("Accept-Encoding", "br, gzip")
	got := httptest.NewRecorder()
	hdl.ServeHTTP(got, req)
	if got.Code != http.StatusOK {
		t.Error("got error", got.Code)
	}
	if got.Result().ContentLength == 4096 {
		t.Error("length", got.Result().ContentLength)
	}
	if !strings.HasPrefix(got.Result().Header.Get("Etag"), "W/") {
		t.Error("etag", got.Result().Header.Get("Etag"))
	}
	if got.Result().Header.Get("Content-Encoding") != "gzip" {
		t.Error("gzip", got.Result().Header.Get("Content-Encoding"))
	}

	// not accept encoding
	req2 := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/4kb.txt", bytes.NewBuffer([]byte{}))
	got2 := httptest.NewRecorder()
	hdl.ServeHTTP(got2, req2)
	if got2.Code != http.StatusOK {
		t.Error("got error(decompress)", got2.Code)
	}
	if got2.Result().ContentLength != 4096 {
		t.Error("length(decompress)", got2.Result().ContentLength)
	}
	if !strings.HasPrefix(got2.Result().Header.Get("Etag"), "W/") {
		t.Error("etag(decompress)", got2.Result().Header.Get("Etag"))
	}
	if got2.Result().Header.Get("Content-Encoding") == "gzip" {
		t.Error("gzip(decompress)", got2.Result().Header.Get("Content-Encoding"))
	}
}

func TestIndex(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		zipfiles:    nil,
		stripprefix: "",
		addprefix:   "",
		indexname:   "512b.txt",
		methodmap:   make(map[string]map[uint16]int),
	}
	if err := hdl.initialize_memory([][]byte{testzip}); err != nil {
		t.Error("initialize", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/", bytes.NewBuffer([]byte{}))
	req.Header.Add("Accept-Encoding", "br, gzip")
	got := httptest.NewRecorder()
	hdl.ServeHTTP(got, req)
	if got.Code != http.StatusOK {
		t.Error("got error", got.Code)
	}
	if got.Result().ContentLength != 512 {
		t.Error("length", got.Result().ContentLength)
	}
}

func TestNotFound(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		zipfiles:    nil,
		stripprefix: "",
		addprefix:   "",
		indexname:   "index.html",
		methodmap:   make(map[string]map[uint16]int),
	}
	if err := hdl.initialize_memory([][]byte{testzip}); err != nil {
		t.Error("initialize", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/", bytes.NewBuffer([]byte{}))
	req.Header.Add("Accept-Encoding", "br, gzip")
	got := httptest.NewRecorder()
	hdl.ServeHTTP(got, req)
	if got.Code != http.StatusNotFound {
		t.Error("got error", got.Code)
	}
	if !strings.Contains(got.Body.String(), "not found") {
		t.Error("content", got.Body.String())
	}
}

func TestConditional(t *testing.T) {
	t.Parallel()
	etag_true := "W/12345678"
	etag_false := "W/00000000"
	r_both_etag_false := &http.Request{
		Header: http.Header{
			"If-None-Match":     []string{etag_false},
			"If-Modified-Since": []string{"Wed, 01 Jan 2025 00:00:00 GMT"},
		},
	}
	r_both := &http.Request{
		Header: http.Header{
			"If-None-Match":     []string{etag_true},
			"If-Modified-Since": []string{"Wed, 01 Jan 2025 00:00:00 GMT"},
		},
	}
	r_modified := &http.Request{
		Header: http.Header{
			"If-Modified-Since": []string{"Wed, 01 Jan 2025 00:00:00 GMT"},
		},
	}
	r_etag := &http.Request{
		Header: http.Header{
			"If-None-Match": []string{etag_true},
		},
	}
	r_etag_false := &http.Request{
		Header: http.Header{
			"If-None-Match": []string{etag_false},
		},
	}
	r_none := &http.Request{
		Header: http.Header{},
	}
	r_modified_invalid := &http.Request{
		Header: http.Header{
			"If-Modified-Since": []string{"invalid-date"},
		},
	}

	fi_old := &zip.File{
		FileHeader: zip.FileHeader{
			Modified: time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC),
		},
	}
	fi_eq := &zip.File{
		FileHeader: zip.FileHeader{
			Modified: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	fi_new := &zip.File{
		FileHeader: zip.FileHeader{
			Modified: time.Date(2025, 1, 1, 0, 0, 1, 0, time.UTC),
		},
	}
	type td struct {
		req      *http.Request
		etag     string
		fl       *zip.File
		expected bool
	}
	tdata := []td{
		// both -> check etag true
		{r_both, etag_true, fi_old, true},
		{r_both, etag_false, fi_old, false},
		{r_both, etag_true, fi_new, true},
		{r_both, etag_false, fi_new, false},
		{r_both, etag_true, fi_eq, true},
		{r_both, etag_false, fi_eq, false},
		// both_etag_false -> check etag false
		{r_both_etag_false, etag_true, fi_old, false},
		{r_both_etag_false, etag_false, fi_old, true},
		{r_both_etag_false, etag_true, fi_new, false},
		{r_both_etag_false, etag_false, fi_new, true},
		{r_both_etag_false, etag_true, fi_eq, false},
		{r_both_etag_false, etag_false, fi_eq, true},
		// etag -> check etag true
		{r_etag, etag_true, fi_old, true},
		{r_etag, etag_false, fi_old, false},
		{r_etag, etag_true, fi_new, true},
		{r_etag, etag_false, fi_new, false},
		{r_etag, etag_true, fi_eq, true},
		{r_etag, etag_false, fi_eq, false},
		// etag_false -> check etag false
		{r_etag_false, etag_true, fi_old, false},
		{r_etag_false, etag_false, fi_old, true},
		{r_etag_false, etag_true, fi_new, false},
		{r_etag_false, etag_false, fi_new, true},
		{r_etag_false, etag_true, fi_eq, false},
		{r_etag_false, etag_false, fi_eq, true},
		// modified -> check old or eq
		{r_modified, etag_true, fi_old, true},
		{r_modified, etag_false, fi_old, true},
		{r_modified, etag_true, fi_new, false},
		{r_modified, etag_false, fi_new, false},
		{r_modified, etag_true, fi_eq, true},
		{r_modified, etag_false, fi_eq, true},
		// none -> false
		{r_none, etag_true, fi_old, false},
		{r_none, etag_false, fi_old, false},
		{r_none, etag_true, fi_new, false},
		{r_none, etag_false, fi_new, false},
		{r_none, etag_true, fi_eq, false},
		{r_none, etag_false, fi_eq, false},
		// invalid date -> false
		{r_modified_invalid, etag_true, fi_old, false},
		{r_modified_invalid, etag_false, fi_old, false},
		{r_modified_invalid, etag_true, fi_new, false},
		{r_modified_invalid, etag_false, fi_new, false},
		{r_modified_invalid, etag_true, fi_eq, false},
		{r_modified_invalid, etag_false, fi_eq, false},
	}
	for idx, t0 := range tdata {
		if conditional(t0.req, t0.etag, t0.fl) != t0.expected {
			t.Error(idx, t0.req.Header.Get("If-None-Match"), t0.etag,
				t0.req.Header.Get("If-Modified-Since"), t0.fl.Modified,
				t0.expected)
		}
	}
}

func TestAcceptEncoding(t *testing.T) {
	t.Parallel()
	h := ZipHandler{}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/", bytes.NewBuffer([]byte{}))
	req.Header.Set("Accept-Encoding", "gzip, x-gzip, compress, x-compress, deflate, br, identity, zstd, *;q=0.1, unknown")
	got := h.accept_encoding(req)
	if got&EncodingGzip == 0 {
		t.Error("gzip not detected", got)
	}
	if got&EncodingCompress == 0 {
		t.Error("compress not detected", got)
	}
	if got&EncodingDeflate == 0 {
		t.Error("deflate not detected", got)
	}
	if got&EncodingBrotli == 0 {
		t.Error("brotli not detected", got)
	}
	if got&EncodingIdentity == 0 {
		t.Error("identity not detected", got)
	}
	if got&EncodingZstd == 0 {
		t.Error("zstd not detected", got)
	}
	if got&EncodingAny == 0 {
		t.Error("any not detected", got)
	}
}

func TestFilename(t *testing.T) {
	t.Parallel()
	tdata := []struct {
		name        string
		stripprefix string
		addprefix   string
		indexname   string
		path        string
		expected    string
	}{
		{
			name:        "normalize and strip addprefix",
			stripprefix: "/root",
			addprefix:   "/static",
			indexname:   "index.html",
			path:        "/static/docs//a.txt",
			expected:    "root/docs/a.txt",
		},
		{
			name:        "root with index",
			stripprefix: "",
			addprefix:   "",
			indexname:   "index.html",
			path:        "/",
			expected:    "index.html",
		},
		{
			name:        "empty path with index",
			stripprefix: "",
			addprefix:   "",
			indexname:   "home.html",
			path:        "",
			expected:    "home.html",
		},
	}
	for _, tt := range tdata {
		t.Run(tt.name, func(t *testing.T) {
			h := ZipHandler{
				stripprefix: tt.stripprefix,
				addprefix:   tt.addprefix,
				indexname:   tt.indexname,
			}
			req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/", bytes.NewBuffer([]byte{}))
			req.URL.Path = tt.path
			if got := h.filename(req); got != tt.expected {
				t.Error("filename mismatch", got, tt.expected)
			}
		})
	}
}

func TestDirectoryRedirect(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		indexname:   "index.html",
		dirredirect: true,
		methodmap: map[string]map[uint16]int{
			"dir/index.html": {zip.Store: 0},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/dir", bytes.NewBuffer([]byte{}))
	got := httptest.NewRecorder()
	hdl.ServeHTTP(got, req)
	if got.Code != http.StatusMovedPermanently {
		t.Error("status", got.Code)
	}
	if loc := got.Result().Header.Get("Location"); loc != "/dir/" {
		t.Error("location", loc)
	}
}

func TestHeaderInjection(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		indexname: "index.html",
		methodmap: make(map[string]map[uint16]int),
		headers: map[string]string{
			"X-Test": "ok",
		},
	}
	if err := hdl.initialize_memory([][]byte{testzip}); err != nil {
		t.Error("initialize", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/512b.txt", bytes.NewBuffer([]byte{}))
	got := httptest.NewRecorder()
	hdl.ServeHTTP(got, req)
	if got.Code != http.StatusOK {
		t.Error("status", got.Code)
	}
	if hdr := got.Result().Header.Get("X-Test"); hdr != "ok" {
		t.Error("X-Test", hdr)
	}
}

func TestGzipSuffixRequest(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		indexname: "index.html",
		methodmap: make(map[string]map[uint16]int),
	}
	if err := hdl.initialize_memory([][]byte{testzip}); err != nil {
		t.Error("initialize", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/4kb.txt.gz", bytes.NewBuffer([]byte{}))
	got := httptest.NewRecorder()
	hdl.ServeHTTP(got, req)
	if got.Code != http.StatusOK {
		t.Error("status", got.Code)
	}
	if ctype := got.Result().Header.Get("Content-Type"); ctype != "application/gzip" {
		t.Error("content-type", ctype)
	}
	if etag := got.Result().Header.Get("Etag"); !strings.HasSuffix(etag, "_gz") {
		t.Error("etag", etag)
	}
	if got.Body.Len() == 0 {
		t.Error("empty body")
	}
}

func TestNotModifiedByETag(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		indexname: "index.html",
		methodmap: make(map[string]map[uint16]int),
	}
	if err := hdl.initialize_memory([][]byte{testzip}); err != nil {
		t.Error("initialize", err)
		return
	}
	idx, ok := hdl.methodmap["512b.txt"][zip.Store]
	if !ok {
		t.Error("missing entry", "512b.txt")
		return
	}
	fi := hdl.getidx(idx)
	if fi == nil {
		t.Error("missing file by index", idx)
		return
	}
	etag := "W/" + strconv.FormatUint(uint64(fi.CRC32), 16)
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/512b.txt", bytes.NewBuffer([]byte{}))
	req.Header.Set("If-None-Match", etag)
	got := httptest.NewRecorder()
	hdl.ServeHTTP(got, req)
	if got.Code != http.StatusNotModified {
		t.Error("status", got.Code)
	}
	if body := strings.TrimSpace(got.Body.String()); body != "" {
		t.Error("unexpected body", body)
	}
}

func TestHandlePreInternalErrorByInvalidIndex(t *testing.T) {
	t.Parallel()
	h := ZipHandler{
		zipfiles: nil,
		headers:  map[string]string{},
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/x", bytes.NewBuffer([]byte{}))
	w := httptest.NewRecorder()
	status := http.StatusOK
	_, err := h.handle_pre(w, req, map[uint16]int{zip.Store: 0}, zip.Store, "", 0, &status)
	if err == nil {
		t.Error("expected error")
	}
}

func TestInitializeFile(t *testing.T) {
	t.Parallel()
	zipname := prepare_testzip(t)
	h := ZipHandler{
		methodmap: make(map[string]map[uint16]int),
	}
	if err := h.initialize_file([]string{zipname}); err != nil {
		t.Error("initialize_file", err)
		return
	}
	if len(h.methodmap) == 0 {
		t.Error("methodmap is empty")
	}
	if err := h.Close(); err != nil {
		t.Error("close", err)
	}
}

func TestReload(t *testing.T) {
	zipname := prepare_testzip(t)
	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false
	globalOption.Archive = flags.Filename(zipname)

	cmd := WebServer{
		InMemory: false,
		handler: ZipHandler{
			methodmap: make(map[string]map[uint16]int),
		},
	}
	if err := cmd.handler.initialize([]string{zipname}, false); err != nil {
		t.Error("initialize", err)
		return
	}
	defer cmd.handler.Close()
	if err := cmd.Reload(); err != nil {
		t.Error("reload", err)
	}
	if _, ok := cmd.handler.methodmap[filepath.Base("512b.txt")]; !ok {
		if _, ok2 := cmd.handler.methodmap["512b.txt"]; !ok2 {
			t.Error("expected 512b.txt after reload")
		}
	}
}

func TestDeflateRawResponse(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		indexname: "index.html",
		methodmap: make(map[string]map[uint16]int),
	}
	if err := hdl.initialize_memory([][]byte{testzip}); err != nil {
		t.Error("initialize", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/4kb.txt", bytes.NewBuffer([]byte{}))
	req.Header.Set("Accept-Encoding", "deflate")
	got := httptest.NewRecorder()
	hdl.ServeHTTP(got, req)
	if got.Code != http.StatusOK {
		t.Error("status", got.Code)
	}
	if enc := got.Result().Header.Get("Content-Encoding"); enc != "deflate" {
		t.Error("content-encoding", enc)
	}
}

func TestZstdFallbackToNormal(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		indexname: "index.html",
		methodmap: make(map[string]map[uint16]int),
	}
	if err := hdl.initialize_memory([][]byte{testzip}); err != nil {
		t.Error("initialize", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/4kb.txt", bytes.NewBuffer([]byte{}))
	req.Header.Set("Accept-Encoding", "zstd")
	got := httptest.NewRecorder()
	hdl.ServeHTTP(got, req)
	if got.Code != http.StatusOK {
		t.Error("status", got.Code)
	}
	if enc := got.Result().Header.Get("Content-Encoding"); enc != "" {
		t.Error("unexpected content-encoding", enc)
	}
	if got.Result().ContentLength != 4096 {
		t.Error("content-length", got.Result().ContentLength)
	}
}

func TestServeHTTPNotFoundByEmptyMethodMap(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		indexname: "index.html",
		methodmap: map[string]map[uint16]int{
			"broken.txt": {},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/broken.txt", bytes.NewBuffer([]byte{}))
	got := httptest.NewRecorder()
	hdl.ServeHTTP(got, req)
	if got.Code != http.StatusNotFound {
		t.Error("status", got.Code)
	}
	if !strings.Contains(got.Body.String(), "not found") {
		t.Error("body", got.Body.String())
	}
}

func TestInitializeInMemory(t *testing.T) {
	t.Parallel()
	zipname := prepare_testzip(t)
	h := ZipHandler{methodmap: make(map[string]map[uint16]int)}
	if err := h.initialize([]string{zipname}, true); err != nil {
		t.Error("initialize in-memory", err)
		return
	}
	if len(h.methodmap) == 0 {
		t.Error("empty methodmap")
	}
}

func TestReloadError(t *testing.T) {
	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false
	globalOption.Archive = flags.Filename("/not/found/archive.zip")

	cmd := WebServer{handler: ZipHandler{methodmap: make(map[string]map[uint16]int)}}
	if err := cmd.Reload(); err == nil {
		t.Error("expected reload error")
	}
}

func TestDoListen(t *testing.T) {
	t.Parallel()
	ln, err := do_listen("tcp:127.0.0.1:0")
	if err != nil {
		t.Error("tcp listen", err)
		return
	}
	defer ln.Close()
	if _, err = url.Parse("http://" + ln.Addr().String()); err != nil {
		t.Error("invalid addr", err)
	}
}

func TestDoListenDefaultAndInvalid(t *testing.T) {
	t.Parallel()

	ln, err := do_listen("127.0.0.1:0")
	if err != nil {
		t.Error("default tcp listen", err)
		return
	}
	_ = ln.Close()

	if _, err = do_listen("bad://addr"); err == nil {
		t.Error("expected invalid listen error")
	}
}

func TestMultipleArchiveInitializeAndServe(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	customZip := filepath.Join(td, "custom.zip")
	err := createSimpleZip(customZip, "custom.txt", []byte("from-custom"))
	if err != nil {
		t.Error("create custom zip", err)
		return
	}

	baseZip := prepare_testzip(t)
	h := ZipHandler{methodmap: make(map[string]map[uint16]int), indexname: "index.html"}
	if err = h.initialize_file([]string{baseZip, customZip}); err != nil {
		t.Error("initialize_file", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/custom.txt", bytes.NewBuffer([]byte{}))
	got := httptest.NewRecorder()
	h.ServeHTTP(got, req)
	if got.Code != http.StatusOK {
		t.Error("status", got.Code)
	}
	if !strings.Contains(got.Body.String(), "from-custom") {
		t.Error("body", got.Body.String())
	}
}

func TestWebServerExecuteInitializeError(t *testing.T) {
	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false
	globalOption.Archive = flags.Filename("/not/found/archive.zip")

	cmd := WebServer{Listen: "127.0.0.1:0"}
	if err := cmd.Execute(nil); err == nil {
		t.Error("expected initialize error")
	}
}

func TestWebServerExecuteInvalidHeader(t *testing.T) {
	zipname := prepare_testzip(t)
	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false
	globalOption.Archive = flags.Filename(zipname)

	cmd := WebServer{Listen: "127.0.0.1:0", Headers: []string{"invalid-header"}}
	err := cmd.Execute(nil)
	if err == nil {
		t.Error("expected invalid header error")
		return
	}
	if !strings.Contains(err.Error(), "invalid header") {
		t.Error("unexpected error", err)
	}
}

func TestShutdown(t *testing.T) {
	t.Parallel()
	cmd := WebServer{}
	err := cmd.Shutdown()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Error("shutdown", err)
	}
}

func createSimpleZip(path, name string, content []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create(name)
	if err != nil {
		_ = zw.Close()
		_ = f.Close()
		return err
	}
	if _, err = w.Write(content); err != nil {
		_ = zw.Close()
		_ = f.Close()
		return err
	}
	if err = zw.Close(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return nil
}
