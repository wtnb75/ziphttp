package main

import (
	"archive/zip"
	"testing"
	"time"
)

func TestSiteMapInitialize(t *testing.T) {
	t.Parallel()
	sm := &SiteMapRoot{}
	if err := sm.initialize(); err != nil {
		t.Error("initialize", err)
		return
	}
	if sm.NS != "http://www.sitemaps.org/schemas/sitemap/0.9" {
		t.Error("namespace", sm.NS)
	}
}

func TestSiteMapAddZip(t *testing.T) {
	t.Parallel()
	sm := &SiteMapRoot{}
	dir := &zip.File{FileHeader: zip.FileHeader{Name: "docs/"}}
	if err := sm.AddZip("https://example.com/", dir); err != nil {
		t.Error("add dir", err)
	}
	if len(sm.SiteList) != 0 {
		t.Error("dir should be skipped")
	}

	fi := &zip.File{FileHeader: zip.FileHeader{Name: "docs/index.html", Modified: time.Unix(10, 0)}}
	if err := sm.AddZip("https://example.com/", fi); err != nil {
		t.Error("add zip", err)
		return
	}
	if len(sm.SiteList) != 1 {
		t.Error("size", len(sm.SiteList))
		return
	}
	if sm.SiteList[0].URL != "https://example.com/docs/" {
		t.Error("url", sm.SiteList[0].URL)
	}
}

func TestSiteMapAddFileAndLastMod(t *testing.T) {
	t.Parallel()
	sm := &SiteMapRoot{}
	if err := sm.AddFile("", "index.html", "a/index.html", time.Unix(1, 0)); err != nil {
		t.Error("add empty base", err)
	}
	if len(sm.SiteList) != 0 {
		t.Error("base empty should skip")
	}

	t1 := time.Unix(1, 0)
	t2 := time.Unix(5, 0)
	if err := sm.AddFile("https://example.com", "index.html", "a/index.html", t1); err != nil {
		t.Error("add file1", err)
	}
	if err := sm.AddFile("https://example.com", "index.html", "b/page.html", t2); err != nil {
		t.Error("add file2", err)
	}
	if len(sm.SiteList) != 2 {
		t.Error("size", len(sm.SiteList))
	}
	if sm.SiteList[0].URL != "https://example.com/a/" {
		t.Error("trimmed url", sm.SiteList[0].URL)
	}
	if !sm.LastMod().Equal(t2) {
		t.Error("lastmod", sm.LastMod(), t2)
	}
}
