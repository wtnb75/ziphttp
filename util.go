package main

import (
	"archive/zip"
	"compress/gzip"
	"encoding/binary"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
)

func CopyGzip(ofp io.Writer, zf *zip.File) (int64, error) {
	if zf.Method != zip.Deflate {
		rd, err := zf.Open()
		if err != nil {
			return 0, err
		}
		defer rd.Close()
		wr := gzip.NewWriter(ofp)
		defer wr.Close()
		return io.Copy(wr, rd)
	}
	ifp, err := zf.OpenRaw()
	if err != nil {
		return 0, err
	}
	var written int64 = 0
	// gzip header
	hdr := make([]byte, 10)
	hdr[0] = 0x1f // id1
	hdr[1] = 0x8b // id2
	hdr[2] = 0x08 // deflate
	hdr[3] = 0x01 // flag(not text)
	binary.LittleEndian.PutUint32(
		hdr[4:8], uint32(zf.Modified.Unix()%0x100000000)) // timestamp
	switch zf.Flags & 0x3 {
	case 0x1:
		hdr[8] = 0x02 // max compression
	case 0x3:
		hdr[8] = 0x04 // min compression
	default:
		hdr[8] = 0x03 // middle compression
	}
	hdr[9] = 0x03 // unix
	whead, err := ofp.Write(hdr)
	written += int64(whead)
	if err != nil {
		return written, err
	}
	// body
	wcopy, err := io.Copy(ofp, ifp)
	written += wcopy
	if err != nil {
		return written, err
	}
	// gzip tailer
	tail := make([]byte, 8)
	binary.LittleEndian.PutUint32(tail, zf.CRC32)
	binary.LittleEndian.PutUint32(tail[4:8], uint32(zf.UncompressedSize64%0x100000000))
	wtail, err := ofp.Write(tail)
	written += int64(wtail)
	if err != nil {
		return written, err
	}
	return written, nil
}

func ispat(name string, head []byte, pat []string) bool {
	// filename pattern
	for _, p := range pat {
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
	}
	content_type := http.DetectContentType(head)
	sname := strings.SplitN(content_type, ";", 2)
	slog.Debug("content type", "name", name, "content-type", content_type, "sname", sname[0])
	if len(sname) != 0 {
		for _, p := range pat {
			if matched, _ := filepath.Match(p, strings.TrimSpace(sname[0])); matched {
				return true
			}
		}
	}
	return false
}

func ismatch(name string, exclude []string) bool {
	for _, pat := range exclude {
		res, err := filepath.Match(pat, name)
		if err != nil {
			slog.Error("match error", "pattern", pat, "name", name, "error", err)
		}
		if res {
			slog.Debug("match", "pattern", pat, "name", name)
			return res
		}
	}
	return false
}
