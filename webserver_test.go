package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"

	"testing"
)

func TestStored(t *testing.T) {
	hdl := ZipHandler{
		zipfile:     nil,
		stripprefix: "",
		addprefix:   "",
		indexname:   "index.html",
		deflmap:     make(map[string]int),
		storemap:    make(map[string]int),
	}
	if err := hdl.initialize_memory(testzip); err!=nil{
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
	hdl := ZipHandler{
		zipfile:     nil,
		stripprefix: "",
		addprefix:   "",
		indexname:   "index.html",
		deflmap:     make(map[string]int),
		storemap:    make(map[string]int),
	}
	if err := hdl.initialize_memory(testzip); err!=nil{
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
