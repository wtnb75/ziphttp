package main

import (
	"archive/zip"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jessevdk/go-flags"
)

func TestZipCmd(t *testing.T) {
	orig_global := globalOption
	defer func() {
		globalOption = orig_global
	}()
	fname := prepare_testzip(t)
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
	tmpfile, err := os.CreateTemp(t.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	defer os.Remove(tmpfile.Name())

	zz := ZipCmd{
		Exclude: []string{"128m*"}, // skip 128mb.txt
		MinSize: 513,               // do not compress 512b.txt
		Method:  "zopfli",
	}
	globalOption.Archive = flags.Filename(tmpfile.Name())
	err = zz.Execute([]string{fname})
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
	orig_global := globalOption
	defer func() {
		globalOption = orig_global
	}()
	fname := prepare_testzip(t)
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
	tmpfile, err := os.CreateTemp(t.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	defer os.Remove(tmpfile.Name())

	zz := ZipCmd{
		Exclude: []string{"128m*"}, // skip 128mb.txt
		MinSize: 513,               // do not compress 512b.txt
		Method:  "deflate",
	}
	globalOption.Archive = flags.Filename(tmpfile.Name())
	err = zz.Execute([]string{fname})
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
	t.Skip()
	orig_global := globalOption
	defer func() {
		globalOption = orig_global
	}()
	fname := prepare_testzip(t)
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
	tmpfile, err := os.CreateTemp(t.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	defer os.Remove(tmpfile.Name())

	zz := ZipCmd{
		Exclude: []string{"128m*"}, // skip 128mb.txt
		MinSize: 513,               // do not compress 512b.txt
		Method:  "deflate",
		SiteMap: "http://example.com/path/",
		BaseURL: "http://example.com/path/",
	}
	globalOption.Archive = flags.Filename(tmpfile.Name())
	err = zz.Execute([]string{fname})
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
	orig_global := globalOption
	defer func() {
		globalOption = orig_global
	}()
	tmpfile, err := os.CreateTemp(t.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	defer os.Remove(tmpfile.Name())
	inputfile, err := os.CreateTemp(t.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	if written, err := inputfile.WriteString("hello world\n"); err != nil {
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
	orig_global := globalOption
	defer func() {
		globalOption = orig_global
	}()
	tmpfile, err := os.CreateTemp(t.TempDir(), "")
	if err != nil {
		t.Error("mktemp-d", err)
		return
	}
	defer os.Remove(tmpfile.Name())
	inputdir := t.TempDir()
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

func zipcmd_helper_inittest(t *testing.T) (dirname string, zipname string, err error) {
	t.Helper()
	testdata := "hello world\n"
	hashval := crc32.ChecksumIEEE([]byte(testdata))
	t.Log("hashval is", hashval)
	base := t.TempDir()
	if err = os.Mkdir(filepath.Join(base, "orgdir"), 0o755); err != nil {
		t.Error("mkdir(orgdir)")
		return
	}
	dirname = filepath.Join(base, "orgdir")
	if err = os.Mkdir(filepath.Join(base, "orgdir", "indir"), 0o755); err != nil {
		t.Error("mkdir(orgdir/indir)")
		return
	}
	files_dir := []string{"name1.txt", "name2.txt", "indir/name1.txt", "indir/name2.txt"}
	files_zip := []string{"name0.txt", "name2.txt", "indir/name0.txt", "indir/name2.txt"}
	for _, fn := range files_dir {
		var fp *os.File
		if fp, err = os.Create(filepath.Join(base, "orgdir", fn)); err != nil {
			t.Error("create temp", fn)
			return
		}
		var written int
		if written, err = fp.WriteString(testdata); err != nil {
			t.Error("testdata write", "error", err, "written", written)
			return
		}
		fp.Close()
	}
	zipname = filepath.Join(base, "input.zip")
	var zf *os.File
	if zf, err = os.Create(zipname); err != nil {
		t.Error("create(fd-zip)")
	}
	zw := zip.NewWriter(zf)
	for _, fn := range files_zip {
		var w io.Writer
		if w, err = zw.Create(fn); err != nil {
			t.Error("create(zip)")
		}
		var written int
		if written, err = w.Write([]byte(testdata)); err != nil {
			t.Error("testdata write", "written", written, "error", err)
		}
		if err = zw.Flush(); err != nil {
			t.Error("flush(zip)", fn)
			return
		}
	}
	if err = zw.Close(); err != nil {
		t.Error("close(zip)")
	}
	if err = zf.Close(); err != nil {
		t.Error("close(fd-zip)")
	}
	return
}

func zipcmd_helper_check(t *testing.T, zipfile string, expected []string) {
	t.Helper()
	t.Log("outfile", zipfile)
	zr, err := zip.OpenReader(zipfile)
	if err != nil {
		t.Error("result open", zipfile, err)
		return
	}
	defer zr.Close()
	MakeBrotliReader(zr)
	MakeZstdReader(zr)
	if len(zr.File) != len(expected) {
		t.Error("file count mismatch", "in-zip", len(zr.File), "expected", len(expected))
	}
	names := ""
	for _, v := range zr.File {
		names += "<" + v.Name + ">, "
	}
	t.Log("names-in-zip are:", names)
	for _, v := range expected {
		if !strings.Contains(names, "<"+v+">") {
			t.Error("not found", zipfile, v)
		}
	}
	for _, v := range zr.File {
		t.Log("name", v.Name, "checksum", v.CRC32)
		rdc, err := v.Open()
		if err != nil {
			t.Error("cannot open", "name", v.Name, "error", err, "checksum", v.CRC32)
		}
		data, err := io.ReadAll(rdc)
		if err != nil {
			t.Error("cannot read", "name", v.Name, "error", err, "checksum", v.CRC32)
		}
		if len(data) != int(v.UncompressedSize64) {
			t.Error("size mismatch", "name", v.Name, "read", len(data), "metadata", v.UncompressedSize64)
		}
		if err = rdc.Close(); err != nil {
			t.Error("close error", "name", v.Name, "error", err)
		}
	}
}

func TestZipCmdDel(t *testing.T) {
	orig_global := globalOption
	defer func() {
		globalOption = orig_global
	}()
	dirname, zipname, err := zipcmd_helper_inittest(t)
	if err != nil {
		return
	}
	commands := []ZipCmd{
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: true, Parallel: 1, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: true, Parallel: 1, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: false, Parallel: 1, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: false, Parallel: 1, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: true, Parallel: 5, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: true, Parallel: 5, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: false, Parallel: 5, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: false, Parallel: 5, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "brotli", UseAsIs: false, InMemory: false, Parallel: 5, Delete: true, SortBy: "none"},
	}
	expected := []string{"name0.txt", "name2.txt", "indir/name0.txt", "indir/name2.txt"}
	for idx, cmd := range commands {
		outfile := filepath.Join(t.TempDir(), fmt.Sprintf("output-%d.zip", idx))
		globalOption.Archive = flags.Filename(outfile)
		res := cmd.Execute([]string{dirname, zipname})
		if res != nil {
			t.Error("error", res, "idx", idx)
		}
		zipcmd_helper_check(t, outfile, expected)
	}
}

func TestZipCmdSkipStore(t *testing.T) {
	orig_global := globalOption
	defer func() {
		globalOption = orig_global
	}()
	dirname, zipname, err := zipcmd_helper_inittest(t)
	if err != nil {
		return
	}
	commands := []ZipCmd{
		{StripRoot: true, Method: "deflate", SkipStore: true, MinSize: 512},
	}
	expected := []string{}
	for idx, cmd := range commands {
		outfile := filepath.Join(t.TempDir(), fmt.Sprintf("output-%d.zip", idx))
		globalOption.Archive = flags.Filename(outfile)
		res := cmd.Execute([]string{dirname, zipname})
		if res != nil {
			t.Error("error", res, "idx", idx)
		}
		zipcmd_helper_check(t, outfile, expected)
	}
}

func TestZipCmdNoDel(t *testing.T) {
	orig_global := globalOption
	defer func() {
		globalOption = orig_global
	}()
	dirname, zipname, err := zipcmd_helper_inittest(t)
	if err != nil {
		return
	}
	commands := []ZipCmd{
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: true, Parallel: 1, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: true, Parallel: 1, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: false, Parallel: 1, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: false, Parallel: 1, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: true, Parallel: 5, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: true, Parallel: 5, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: false, Parallel: 5, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: false, Parallel: 5, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "brotli", UseAsIs: false, InMemory: false, Parallel: 5, Delete: false, SortBy: "none"},
	}
	expected := []string{
		"name0.txt", "name1.txt", "name2.txt", "indir/name0.txt",
		"indir/name1.txt", "indir/name2.txt"}
	for idx, cmd := range commands {
		outfile := filepath.Join(t.TempDir(), fmt.Sprintf("output-%d.zip", idx))
		globalOption.Archive = flags.Filename(outfile)
		res := cmd.Execute([]string{dirname, zipname})
		if res != nil {
			t.Error("error", res, "idx", idx)
		}
		t.Log("command", idx, cmd)
		zipcmd_helper_check(t, outfile, expected)
	}
}

func TestZipCmdDelSitemap(t *testing.T) {
	orig_global := globalOption
	defer func() {
		globalOption = orig_global
	}()
	dirname, zipname, err := zipcmd_helper_inittest(t)
	if err != nil {
		return
	}
	commands := []ZipCmd{
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: true, Parallel: 1, Delete: true, SortBy: "none", SiteMap: "http://localhost/"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: true, Parallel: 1, Delete: true, SortBy: "none", SiteMap: "http://localhost/"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: false, Parallel: 1, Delete: true, SortBy: "none", SiteMap: "http://localhost/"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: false, Parallel: 1, Delete: true, SortBy: "none", SiteMap: "http://localhost/"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: true, Parallel: 5, Delete: true, SortBy: "none", SiteMap: "http://localhost/"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: true, Parallel: 5, Delete: true, SortBy: "none", SiteMap: "http://localhost/"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: false, Parallel: 5, Delete: true, SortBy: "none", SiteMap: "http://localhost/"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: false, Parallel: 5, Delete: true, SortBy: "none", SiteMap: "http://localhost/"},
		{StripRoot: true, Method: "brotli", UseAsIs: false, InMemory: false, Parallel: 5, Delete: true, SortBy: "none", SiteMap: "http://localhost/"},
	}
	expected := []string{"name0.txt", "name2.txt", "indir/name0.txt", "indir/name2.txt", "sitemap.xml"}
	for idx, cmd := range commands {
		outfile := filepath.Join(t.TempDir(), fmt.Sprintf("output-%d.zip", idx))
		globalOption.Archive = flags.Filename(outfile)
		res := cmd.Execute([]string{dirname, zipname})
		if res != nil {
			t.Error("error", res, "idx", idx)
		}
		zipcmd_helper_check(t, outfile, expected)
	}
}

func TestZipCmdSelfDel(t *testing.T) {
	orig_global := globalOption
	defer func() {
		globalOption = orig_global
	}()
	globalOption.Self = true
	dirname, zipname, err := zipcmd_helper_inittest(t)
	if err != nil {
		return
	}
	commands := []ZipCmd{
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: true, Parallel: 1, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: true, Parallel: 1, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: false, Parallel: 1, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: false, Parallel: 1, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: true, Parallel: 5, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: true, Parallel: 5, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: false, Parallel: 5, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: false, Parallel: 5, Delete: true, SortBy: "none"},
		{StripRoot: true, Method: "brotli", UseAsIs: false, InMemory: false, Parallel: 5, Delete: true, SortBy: "none"},
	}
	expected := []string{"name0.txt", "name2.txt", "indir/name0.txt", "indir/name2.txt"}
	for idx, cmd := range commands {
		outfile := filepath.Join(t.TempDir(), fmt.Sprintf("output-%d.zip", idx))
		globalOption.Archive = flags.Filename(outfile)
		res := cmd.Execute([]string{dirname, zipname})
		if res != nil {
			t.Error("error", res, "idx", idx)
		}
		zipcmd_helper_check(t, outfile, expected)
	}
}

func TestZipCmdSelfNoDel(t *testing.T) {
	orig_global := globalOption
	defer func() {
		globalOption = orig_global
	}()
	globalOption.Verbose = true
	globalOption.Self = true
	dirname, zipname, err := zipcmd_helper_inittest(t)
	if err != nil {
		return
	}
	commands := []ZipCmd{
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: true, Parallel: 1, Delete: false, SortBy: "name"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: true, Parallel: 1, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: false, Parallel: 1, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: false, Parallel: 1, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: true, Parallel: 5, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: true, Parallel: 5, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: true, InMemory: false, Parallel: 5, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "deflate", UseAsIs: false, InMemory: false, Parallel: 5, Delete: false, SortBy: "none"},
		{StripRoot: true, Method: "brotli", UseAsIs: false, InMemory: false, Parallel: 5, Delete: false, SortBy: "none"},
	}
	expected := []string{
		"name0.txt", "name1.txt", "name2.txt", "indir/name0.txt",
		"indir/name1.txt", "indir/name2.txt"}
	for idx, cmd := range commands {
		outfile := filepath.Join(t.TempDir(), fmt.Sprintf("output-%d.zip", idx))
		globalOption.Archive = flags.Filename(outfile)
		res := cmd.Execute([]string{dirname, zipname})
		if res != nil {
			t.Error("error", res, "idx", idx)
		}
		zipcmd_helper_check(t, outfile, expected)
	}
}
