package main

import (
	"archive/zip"
	"os"
	"testing"

	"github.com/jessevdk/go-flags"
)

func TestZopfliZip(t *testing.T) {
	fname := prepare(t)
	defer os.Remove(fname)
	zr0, err := zip.OpenReader(fname)
	if err != nil {
		t.Error("zipopen0", err)
		return
	}
	defer zr0.Close()
	crcmap := map[string]uint32{}
	for _, f := range zr0.File {
		crcmap[f.Name] = f.CRC32
	}
	tmpfile, err := os.CreateTemp(os.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	defer os.Remove(tmpfile.Name())

	zz := ZopfliZip{
		Exclude:   []string{"128m*"}, // skip 128mb.txt
		MinSize:   513,               // do not compress 512b.txt
		UseNormal: false,
	}
	globalOption.Archive = flags.Filename(tmpfile.Name())
	err = zz.Execute([]string{fname})
	globalOption.Archive = ""
	if err != nil {
		t.Error("failed", err)
	}
	zr, err := zip.OpenReader(tmpfile.Name())
	if err != nil {
		t.Error("open", err)
	}
	for _, zf := range zr.File {
		if zf.Name == "128m.txt" {
			t.Error("exclude", zf)
		}
		if zf.Method != zip.Deflate && zf.Name != "512b.txt" {
			t.Error("deflate", zf)
		}
		if zf.Method != zip.Store && zf.Name == "512b.txt" {
			t.Error("store", zf)
		}
		if crcmap[zf.Name] != zf.CRC32 {
			t.Error("crc mismatch", zf.Name, zf.CRC32, crcmap[zf.Name])
		}
	}
}

func TestNormalZip(t *testing.T) {
	fname := prepare(t)
	defer os.Remove(fname)
	zr0, err := zip.OpenReader(fname)
	if err != nil {
		t.Error("zipopen0", err)
		return
	}
	defer zr0.Close()
	crcmap := map[string]uint32{}
	for _, f := range zr0.File {
		crcmap[f.Name] = f.CRC32
	}
	tmpfile, err := os.CreateTemp(os.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	defer os.Remove(tmpfile.Name())

	zz := ZopfliZip{
		Exclude:   []string{"128m*"}, // skip 128mb.txt
		MinSize:   513,               // do not compress 512b.txt
		UseNormal: true,
	}
	globalOption.Archive = flags.Filename(tmpfile.Name())
	err = zz.Execute([]string{fname})
	globalOption.Archive = ""
	if err != nil {
		t.Error("failed", err)
	}
	zr, err := zip.OpenReader(tmpfile.Name())
	if err != nil {
		t.Error("open", err)
	}
	for _, zf := range zr.File {
		if zf.Name == "128m.txt" {
			t.Error("exclude", zf)
		}
		if zf.Method != zip.Deflate && zf.Name != "512b.txt" {
			t.Error("deflate", zf)
		}
		if zf.Method != zip.Store && zf.Name == "512b.txt" {
			t.Error("store", zf)
		}
		if crcmap[zf.Name] != zf.CRC32 {
			t.Error("crc mismatch", zf.Name, zf.CRC32, crcmap[zf.Name])
		}
	}
}

func TestNormalZipSiteMap(t *testing.T) {
	fname := prepare(t)
	defer os.Remove(fname)
	zr0, err := zip.OpenReader(fname)
	if err != nil {
		t.Error("zipopen0", err)
		return
	}
	defer zr0.Close()
	crcmap := map[string]uint32{}
	for _, f := range zr0.File {
		crcmap[f.Name] = f.CRC32
	}
	tmpfile, err := os.CreateTemp(os.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	defer os.Remove(tmpfile.Name())

	zz := ZopfliZip{
		Exclude:   []string{"128m*"}, // skip 128mb.txt
		MinSize:   513,               // do not compress 512b.txt
		UseNormal: true,
		SiteMap:   "http://example.com/path/",
		BaseURL:   "http://example.com/path/",
	}
	globalOption.Archive = flags.Filename(tmpfile.Name())
	err = zz.Execute([]string{fname})
	globalOption.Archive = ""
	if err != nil {
		t.Error("failed", err)
	}
	zr, err := zip.OpenReader(tmpfile.Name())
	if err != nil {
		t.Error("open", err)
	}
	flag := false
	for _, zf := range zr.File {
		if zf.Name == "sitemap.xml" {
			flag = true
			continue
		}
		if zf.Name == "128m.txt" {
			t.Error("exclude", zf)
		}
		if zf.Method != zip.Deflate && zf.Name != "512b.txt" {
			t.Error("deflate", zf)
		}
		if zf.Method != zip.Store && zf.Name == "512b.txt" {
			t.Error("store", zf)
		}
		if crcmap[zf.Name] != zf.CRC32 {
			t.Error("crc mismatch", zf.Name, zf.CRC32, crcmap[zf.Name])
		}
	}
	if !flag {
		t.Error("no sitemap generated")
	}
}
