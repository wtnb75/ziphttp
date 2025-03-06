package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jessevdk/go-flags"
)

func TestZipCmd(t *testing.T) {
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

	zz := ZipCmd{
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

func TestZipCmdNormal(t *testing.T) {
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

	zz := ZipCmd{
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

func TestZipCmdSiteMap(t *testing.T) {
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

	zz := ZipCmd{
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

func TestZipcmdFromFile(t *testing.T) {
	tmpfile, err := os.CreateTemp(os.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	defer os.Remove(tmpfile.Name())
	inputfile, err := os.CreateTemp(os.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	if written, err := inputfile.Write([]byte("hello world\n")); err != nil {
		t.Error("write testdata", err, written)
	}
	if err = inputfile.Sync(); err != nil {
		t.Error("sync", err)
	}
	defer os.Remove(inputfile.Name())
	zz := ZipCmd{StripRoot: true}
	globalOption.Archive = flags.Filename(tmpfile.Name())
	err = zz.Execute([]string{inputfile.Name()})
	if err != nil {
		t.Error("execute", err)
		return
	}
	zr, err := zip.OpenReader(tmpfile.Name())
	if err != nil {
		t.Error("cannot open zip", err)
	}
	defer zr.Close()
	fl, err := zr.Open(filepath.Base(inputfile.Name()))
	if err != nil {
		t.Error("file not found", err)
	}
	defer fl.Close()
	data, err := io.ReadAll(fl)
	if err != nil {
		t.Error("read from zip", err)
	}
	if string(data) != "hello world\n" {
		t.Error("data mismatch", data)
	}
}

func TestZipcmdFromDir(t *testing.T) {
	tmpfile, err := os.CreateTemp(os.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	defer os.Remove(tmpfile.Name())
	inputdir, err := os.MkdirTemp(os.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	for i := range 10 {
		if cr, err := os.Create(filepath.Join(inputdir, fmt.Sprintf("%d.txt", i))); err != nil {
			t.Error("tmpfile", i, err)
		} else {
			for j := range 100 {
				fmt.Fprintln(cr, "test data", i, j)
			}
			cr.Close()
		}
	}
	defer os.RemoveAll(inputdir)
	zz := ZipCmd{StripRoot: true}
	globalOption.Archive = flags.Filename(tmpfile.Name())
	err = zz.Execute([]string{inputdir})
	if err != nil {
		t.Error("execute", err)
		return
	}
	zr, err := zip.OpenReader(tmpfile.Name())
	if err != nil {
		t.Error("cannot open zip", err)
	}
	defer zr.Close()
	fl, err := zr.Open("5.txt")
	if err != nil {
		t.Error("file not found", err)
	}
	defer fl.Close()
	data := make([]byte, 10)
	readlen, err := fl.Read(data)
	if err != nil {
		t.Error("read from zip", err)
	}
	if readlen != 10 {
		t.Error("short read", readlen)
	}
	if string(data) != "test data " {
		t.Error("data mismatch", data)
	}
}
