package main

import (
	"archive/zip"
	"encoding/xml"
	"log/slog"
	"net/url"
	"strings"
	"time"
)

type SiteMapRoot struct {
	XMLName  xml.Name   `xml:"urlset"`
	NS       string     `xml:"xmlns,attr"`
	SiteList []*SiteURL `xml:"url"`
	LastMod  time.Time  `xml:"-"`
}

type SiteURL struct {
	URL       string    `xml:"loc"`
	UpdatedAt time.Time `xml:"lastmod"`
}

func (r *SiteMapRoot) initialize() error {
	r.NS = "http://www.sitemaps.org/schemas/sitemap/0.9"
	return nil
}

func (r *SiteMapRoot) AddZip(baseurl string, fi *zip.File) error {
	if fi.FileInfo().IsDir() {
		return nil
	}
	u, err := url.JoinPath(baseurl, fi.Name)
	if err != nil {
		slog.Error("joinpath", "base", baseurl, "name", fi.Name)
		return err
	}
	if strings.HasSuffix(u, "/index.html") {
		u = strings.TrimSuffix(u, "index.html")
	}
	r.SiteList = append(r.SiteList, &SiteURL{URL: u, UpdatedAt: fi.Modified})
	if fi.Modified.After(r.LastMod) {
		r.LastMod = fi.Modified
	}
	return nil
}

func (r *SiteMapRoot) AddFile(baseurl string, indexname string, filename string, updated time.Time) error {
	u, err := url.JoinPath(baseurl, filename)
	if err != nil {
		slog.Error("joinpath", "base", baseurl, "name", filename)
		return err
	}
	if strings.HasSuffix(u, "/"+indexname) {
		u = strings.TrimSuffix(u, indexname)
	}
	r.SiteList = append(r.SiteList, &SiteURL{URL: u, UpdatedAt: updated})
	if updated.After(r.LastMod) {
		r.LastMod = updated
	}
	return nil
}
