package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"gopkg.in/loremipsum.v1"
)

func zero(b *testing.B, size uint) []byte {
	res := make([]byte, size)
	return res
}

func random(b *testing.B, size uint) []byte {
	res := make([]byte, size)
	n, err := rand.Read(res)
	if err != nil {
		b.Error("rand", err)
	}
	if n != int(size) {
		b.Error("short rand", "n", n, "size", size)
	}
	return res
}

func random_text(b *testing.B, size uint) []byte {
	lorem := loremipsum.New()
	var res string
	for uint(len(res)) < size {
		res += lorem.Paragraph() + "\n"
	}
	return []byte(res)[0:size]
}

func prep(b *testing.B) (*zip.Writer, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return zip.NewWriter(buf), buf
}

func prep2(b *testing.B, input *bytes.Buffer) *zip.Reader {
	buf := bytes.NewReader(input.Bytes())
	zr, err := zip.NewReader(buf, int64(input.Len()))
	if err != nil {
		b.Error("NewReader", err)
	}
	return zr
}

func do1(b *testing.B, wr *zip.Writer, data []byte, idx int, method uint16) {
	fh := zip.FileHeader{
		Name:   fmt.Sprintf("name-%d.zero", idx),
		Method: method,
	}
	f, err := wr.CreateHeader(&fh)
	if err != nil {
		b.Error("create", err)
	}
	res, err := f.Write(data)
	if err != nil {
		b.Error("write", err)
	}
	if res != 1024*1024 {
		b.Error("short write", res)
	}
	if err = wr.Flush(); err != nil {
		b.Error("flush", err)
	}
}

func do2(b *testing.B, rd *zip.Reader, idx int) int {
	fi, err := rd.Open(fmt.Sprintf("name-%d.zero", idx))
	if err != nil {
		b.Error("open", err)
	}
	defer fi.Close()
	res, err := io.ReadAll(fi)
	if err != nil {
		b.Error("readall", err)
	}
	return len(res)
}

func post(b *testing.B, wr *zip.Writer) error {
	return wr.Close()
}

type benchdata struct {
	name string
	data []byte
}

func makedata(b *testing.B, size uint) []benchdata {
	res := []benchdata{
		{"Zero", zero(b, 1024*1024)},
		{"Random", random(b, 1024*1024)},
		{"Text", random_text(b, 1024*1024)},
	}
	return res
}

func Benchmark_StoreEncode(b *testing.B) {
	bench := makedata(b, 1024*1024)
	for _, bm := range bench {
		wr, _ := prep(b)
		b.Run(bm.name, func(b *testing.B) {
			var i int
			for b.Loop() {
				do1(b, wr, bm.data, i, zip.Store)
			}
		})
		post(b, wr)
	}
}

func Benchmark_StoreDecode(b *testing.B) {
	bench := makedata(b, 1024*1024)
	wr, rd := prep(b)
	for idx, bm := range bench {
		do1(b, wr, bm.data, idx, zip.Store)
	}
	post(b, wr)
	for idx, bm := range bench {
		zr := prep2(b, rd)
		b.Run(bm.name, func(b *testing.B) {
			for b.Loop() {
				do2(b, zr, idx)
			}
		})
		post(b, wr)
	}
}

func Benchmark_DeflateEncode(b *testing.B) {
	bench := makedata(b, 1024*1024)
	for _, bm := range bench {
		wr, _ := prep(b)
		b.Run(bm.name, func(b *testing.B) {
			var i int
			for b.Loop() {
				do1(b, wr, bm.data, i, zip.Deflate)
			}
		})
		post(b, wr)
	}
}

func Benchmark_DeflateDecode(b *testing.B) {
	bench := makedata(b, 1024*1024)
	wr, rd := prep(b)
	for idx, bm := range bench {
		do1(b, wr, bm.data, idx, zip.Deflate)
	}
	post(b, wr)
	for idx, bm := range bench {
		zr := prep2(b, rd)
		b.Run(bm.name, func(b *testing.B) {
			for b.Loop() {
				do2(b, zr, idx)
			}
		})
		post(b, wr)
	}
}

func Benchmark_ZopfliEncode(b *testing.B) {
	b.Skip() // too slow
	bench := makedata(b, 1024*1024)
	for _, bm := range bench {
		wr, _ := prep(b)
		MakeZopfliWriter(wr, -1)
		b.Run(bm.name, func(b *testing.B) {
			var i int
			for b.Loop() {
				do1(b, wr, bm.data, i, zip.Deflate)
			}
		})
		post(b, wr)
	}
}

func Benchmark_ZopfliDecode(b *testing.B) {
	bench := makedata(b, 1024*1024)
	wr, rd := prep(b)
	MakeZopfliWriter(wr, -1)
	for idx, bm := range bench {
		do1(b, wr, bm.data, idx, zip.Deflate)
	}
	post(b, wr)
	for idx, bm := range bench {
		zr := prep2(b, rd)
		b.Run(bm.name, func(b *testing.B) {
			for b.Loop() {
				do2(b, zr, idx)
			}
		})
		post(b, wr)
	}
}

func Benchmark_BrotliEncode(b *testing.B) {
	bench := makedata(b, 1024*1024)
	for _, bm := range bench {
		wr, _ := prep(b)
		MakeBrotliWriter(wr, -1)
		b.Run(bm.name, func(b *testing.B) {
			var i int
			for b.Loop() {
				do1(b, wr, bm.data, i, Brotli)
			}
		})
		post(b, wr)
	}
}

func Benchmark_BrotliDecode(b *testing.B) {
	bench := makedata(b, 1024*1024)
	wr, rd := prep(b)
	MakeBrotliWriter(wr, -1)
	for idx, bm := range bench {
		do1(b, wr, bm.data, idx, Brotli)
	}
	post(b, wr)
	for idx, bm := range bench {
		zr := prep2(b, rd)
		b.Run(bm.name, func(b *testing.B) {
			for b.Loop() {
				do2(b, zr, idx)
			}
		})
		post(b, wr)
	}
}

func Benchmark_ZstdEncode(b *testing.B) {
	bench := makedata(b, 1024*1024)
	for _, bm := range bench {
		wr, _ := prep(b)
		MakeZstdWriter(wr, -1)
		b.Run(bm.name, func(b *testing.B) {
			var i int
			for b.Loop() {
				do1(b, wr, bm.data, i, Zstd)
			}
		})
		post(b, wr)
	}
}

func Benchmark_ZstdDecode(b *testing.B) {
	bench := makedata(b, 1024*1024)
	wr, rd := prep(b)
	MakeZstdWriter(wr, -1)
	for idx, bm := range bench {
		do1(b, wr, bm.data, idx, Zstd)
	}
	post(b, wr)
	for idx, bm := range bench {
		zr := prep2(b, rd)
		b.Run(bm.name, func(b *testing.B) {
			for b.Loop() {
				do2(b, zr, idx)
			}
		})
		post(b, wr)
	}
}
