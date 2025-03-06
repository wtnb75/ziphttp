package main

import (
	"testing"
	"time"
)

func TestEmpty(t *testing.T) {
	input := []*ChooseFile{}
	res := ChooseFrom(input)
	if res != nil {
		t.Error("empty")
	}
}

func TestSingle(t *testing.T) {
	input := []*ChooseFile{&ChooseFile{Root: "root"}}
	res := ChooseFrom(input)
	if res != input[0] {
		t.Error("single")
	}
}

func TestSameCRC_csize(t *testing.T) {
	input := []*ChooseFile{
		&ChooseFile{Root: "root100", Name: "name", CRC32: 123, CompressedSize: 10},
		&ChooseFile{Root: "root101", Name: "name", CRC32: 123, CompressedSize: 20},
	}
	res := ChooseFrom(input)
	if res != input[0] {
		t.Error("compress size")
	}
}

func TestSameCRC_choose_compressed(t *testing.T) {
	input := []*ChooseFile{
		&ChooseFile{Root: "root100", Name: "name", CRC32: 123, CompressedSize: 0, UncompressedSize: 20},
		&ChooseFile{Root: "root101", Name: "name", CRC32: 123, CompressedSize: 20, UncompressedSize: 20},
		&ChooseFile{Root: "root102", Name: "name", CRC32: 123, CompressedSize: 30, UncompressedSize: 20},
	}
	res := ChooseFrom(input)
	if res != input[1] {
		t.Error("compress size")
	}
}

func TestSameCRC_choose_old(t *testing.T) {
	input := []*ChooseFile{
		&ChooseFile{
			Root: "root100", Name: "name", CRC32: 123, CompressedSize: 0, UncompressedSize: 20},
		&ChooseFile{
			Root: "root101", Name: "name", CRC32: 123, CompressedSize: 20, UncompressedSize: 20,
			ModTime: time.Unix(10, 0)},
		&ChooseFile{
			Root: "root102", Name: "name", CRC32: 123, CompressedSize: 20, UncompressedSize: 20,
			ModTime: time.Unix(20, 0)},
	}
	res := ChooseFrom(input)
	if res != input[1] {
		t.Error("compress old")
	}
}

func TestSameCRC_choose_big(t *testing.T) {
	input := []*ChooseFile{
		&ChooseFile{
			Root: "root100", Name: "name", CRC32: 123, CompressedSize: 0, UncompressedSize: 20},
		&ChooseFile{
			Root: "root101", Name: "name", CRC32: 123, CompressedSize: 20, UncompressedSize: 20,
			ModTime: time.Unix(20, 0)},
		&ChooseFile{
			Root: "root102", Name: "name", CRC32: 123, CompressedSize: 20, UncompressedSize: 30,
			ModTime: time.Unix(20, 0)},
	}
	res := ChooseFrom(input)
	if res != input[2] {
		t.Error("compress uncompressed size")
	}
}

func TestDiffCRC_choose_new(t *testing.T) {
	input := []*ChooseFile{
		&ChooseFile{
			Root: "root100", Name: "name", CRC32: 100, CompressedSize: 0, UncompressedSize: 20,
			ModTime: time.Unix(30, 0)},
		&ChooseFile{
			Root: "root101", Name: "name", CRC32: 101, CompressedSize: 20, UncompressedSize: 20,
			ModTime: time.Unix(20, 0)},
		&ChooseFile{
			Root: "root102", Name: "name", CRC32: 102, CompressedSize: 20, UncompressedSize: 30,
			ModTime: time.Unix(20, 0)},
	}
	res := ChooseFrom(input)
	if res != input[0] {
		t.Error("compress uncompressed size")
	}
}
