package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStored(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		zipfile:     nil,
		stripprefix: "",
		addprefix:   "",
		indexname:   "index.html",
		deflmap:     make(map[string]int),
		storemap:    make(map[string]int),
	}
	if err := hdl.initialize_memory(testzip); err != nil {
		t.Error("initialize", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/512b.txt", bytes.NewBuffer([]byte{}))
	req.Header.Add("accept-encoding", "br, deflate,gzip ")
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
	if got.Result().Header.Get("content-encoding") == "gzip" {
		t.Error("not stored", got.Result().Header.Get("content-encoding"))
	}
}

func TestDeflate(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		zipfile:     nil,
		stripprefix: "",
		addprefix:   "",
		indexname:   "index.html",
		deflmap:     make(map[string]int),
		storemap:    make(map[string]int),
	}
	if err := hdl.initialize_memory(testzip); err != nil {
		t.Error("initialize", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/4kb.txt", bytes.NewBuffer([]byte{}))
	req.Header.Add("accept-encoding", "br, gzip")
	got := httptest.NewRecorder()
	hdl.ServeHTTP(got, req)
	if got.Code != http.StatusOK {
		t.Error("got error", got.Code)
	}
	if got.Result().ContentLength == 4096 {
		t.Error("length", got.Result().ContentLength)
	}
	if !strings.HasPrefix(got.Result().Header.Get("etag"), "W/") {
		t.Error("etag", got.Result().Header.Get("etag"))
	}
	if got.Result().Header.Get("content-encoding") != "gzip" {
		t.Error("gzip", got.Result().Header.Get("content-encoding"))
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
	if !strings.HasPrefix(got2.Result().Header.Get("etag"), "W/") {
		t.Error("etag(decompress)", got2.Result().Header.Get("etag"))
	}
	if got2.Result().Header.Get("content-encoding") == "gzip" {
		t.Error("gzip(decompress)", got2.Result().Header.Get("content-encoding"))
	}
}

func TestIndex(t *testing.T) {
	t.Parallel()
	hdl := ZipHandler{
		zipfile:     nil,
		stripprefix: "",
		addprefix:   "",
		indexname:   "512b.txt",
		deflmap:     make(map[string]int),
		storemap:    make(map[string]int),
	}
	if err := hdl.initialize_memory(testzip); err != nil {
		t.Error("initialize", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/", bytes.NewBuffer([]byte{}))
	req.Header.Add("accept-encoding", "br, gzip")
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
		zipfile:     nil,
		stripprefix: "",
		addprefix:   "",
		indexname:   "index.html",
		deflmap:     make(map[string]int),
		storemap:    make(map[string]int),
	}
	if err := hdl.initialize_memory(testzip); err != nil {
		t.Error("initialize", err)
		return
	}
	req := httptest.NewRequest(http.MethodGet, "http://dummy.url.com/", bytes.NewBuffer([]byte{}))
	req.Header.Add("accept-encoding", "br, gzip")
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
