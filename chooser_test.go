package main

import (
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEmpty(t *testing.T) {
	t.Parallel()
	input := []*ChooseFile{}
	res := ChooseFrom(input, "")
	if res != nil {
		t.Error("empty")
	}
}

func TestSingle(t *testing.T) {
	t.Parallel()
	input := []*ChooseFile{{Root: "root"}}
	res := ChooseFrom(input, "")
	if res != input[0] {
		t.Error("single")
	}
}

func TestSameCRC_csize(t *testing.T) {
	t.Parallel()
	input := []*ChooseFile{
		{Root: "root100", Name: "name", CRC32: 123, CompressedSize: 10},
		{Root: "root101", Name: "name", CRC32: 123, CompressedSize: 20},
	}
	res := ChooseFrom(input, "")
	if res != input[0] {
		t.Error("compress size")
	}
}

func TestSameCRC_choose_compressed(t *testing.T) {
	t.Parallel()
	input := []*ChooseFile{
		{Root: "root100", Name: "name", CRC32: 123, CompressedSize: 0, UncompressedSize: 20},
		{Root: "root101", Name: "name", CRC32: 123, CompressedSize: 20, UncompressedSize: 20},
		{Root: "root102", Name: "name", CRC32: 123, CompressedSize: 30, UncompressedSize: 20},
	}
	res := ChooseFrom(input, "")
	if res != input[1] {
		t.Error("compress size")
	}
}

func TestSameCRC_choose_old(t *testing.T) {
	t.Parallel()
	input := []*ChooseFile{
		{
			Root: "root100", Name: "name", CRC32: 123, CompressedSize: 0, UncompressedSize: 20,
		},
		{
			Root: "root101", Name: "name", CRC32: 123, CompressedSize: 20, UncompressedSize: 20,
			ModTime: time.Unix(10, 0),
		},
		{
			Root: "root102", Name: "name", CRC32: 123, CompressedSize: 20, UncompressedSize: 20,
			ModTime: time.Unix(20, 0),
		},
	}
	res := ChooseFrom(input, "")
	if res != input[1] {
		t.Error("compress old")
	}
}

func TestSameCRC_choose_big(t *testing.T) {
	t.Parallel()
	input := []*ChooseFile{
		{
			Root: "root100", Name: "name", CRC32: 123, CompressedSize: 0, UncompressedSize: 20,
		},
		{
			Root: "root101", Name: "name", CRC32: 123, CompressedSize: 20, UncompressedSize: 20,
			ModTime: time.Unix(20, 0),
		},
		{
			Root: "root102", Name: "name", CRC32: 123, CompressedSize: 20, UncompressedSize: 30,
			ModTime: time.Unix(20, 0),
		},
	}
	res := ChooseFrom(input, "")
	if res != input[2] {
		t.Error("compress uncompressed size")
	}
}

func TestDiffCRC_choose_new(t *testing.T) {
	t.Parallel()
	input := []*ChooseFile{
		{
			Root: "root100", Name: "name", CRC32: 100, CompressedSize: 0, UncompressedSize: 20,
			ModTime: time.Unix(30, 0),
		},
		{
			Root: "root101", Name: "name", CRC32: 101, CompressedSize: 20, UncompressedSize: 20,
			ModTime: time.Unix(20, 0),
		},
		{
			Root: "root102", Name: "name", CRC32: 102, CompressedSize: 20, UncompressedSize: 30,
			ModTime: time.Unix(20, 0),
		},
	}
	res := ChooseFrom(input, "")
	if res != input[0] {
		t.Error("compress uncompressed size")
	}
}

func TestFixCRC(t *testing.T) {
	t.Parallel()
	teststr := "hello world"
	cksum := crc32.ChecksumIEEE([]byte(teststr))
	td := t.TempDir()
	tf := filepath.Join(td, "test.data")
	fi, err := os.Create(tf)
	if err != nil {
		t.Error("file open")
		return
	}
	written, err := fi.WriteString(teststr)
	if err != nil {
		t.Error("writestring", written, err)
	}
	fi.Close()
	cf := ChooseFile{
		Root: td,
		Name: "test.data",
	}
	if err = cf.FixCRC(""); err != nil {
		t.Error("crc error", err)
	}
	if cksum != cf.CRC32 {
		t.Error("crc32 mismatch", cksum, cf.CRC32)
	}
}

func TestFixCRCRelative(t *testing.T) {
	t.Parallel()
	teststr1 := `<html><head></head><body><a href="http://example.com/path/to/file">abcde</a></body></html>`
	teststr2 := `<html><head></head><body><a href="../to/file">abcde</a></body></html>`
	cksum := crc32.ChecksumIEEE([]byte(teststr2))
	td := t.TempDir()
	tf := filepath.Join(td, "test.html")
	fi, err := os.Create(tf)
	if err != nil {
		t.Error("file open")
		return
	}
	written, err := fi.WriteString(teststr1)
	if err != nil {
		t.Error("writestring", written, err)
	}
	fi.Close()
	cf := ChooseFile{
		Root: td,
		Name: "test.html",
	}
	if err = cf.FixCRC("http://example.com/path/hello/index.html"); err != nil {
		t.Error("crc error", err)
	}
	if cksum != cf.CRC32 {
		t.Error("crc32 mismatch", cksum, cf.CRC32)
	}
}
