package main

import (
	"archive/zip"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
)

// from RFC1952, (no FEXTRA/FNAME/FCOMMENT/FHCRC)
const GzipHeaderSize = 10

// from RFC1952, CRC32(4) + ISIZE(4)
const GzipFooterSize = 8

// from APPNOTE.TXT (Local file header: 4+2+2+2+2+2+4+4+4+2+2+len(filename)+len(extra))
const ZipLocalFileHeaderSize = 30

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
	hdr := make([]byte, GzipHeaderSize)
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
	tail := make([]byte, GzipFooterSize)
	binary.LittleEndian.PutUint32(tail, zf.CRC32)
	binary.LittleEndian.PutUint32(tail[4:8], uint32(zf.UncompressedSize64%0x100000000))
	wtail, err := ofp.Write(tail)
	written += int64(wtail)
	if err != nil {
		return written, err
	}
	return written, nil
}

func ismatch(name string, patterns []string) bool {
	for _, pat := range patterns {
		if matched, _ := filepath.Match(pat, name); matched {
			slog.Debug("match", "name", name, "pattern", pat)
			return true
		}
	}
	return false
}

func ispat(head []byte, pat []string) bool {
	content_type := http.DetectContentType(head)
	sname := strings.SplitN(content_type, ";", 2)
	if len(sname) != 0 {
		return ismatch(strings.TrimSpace(sname[0]), pat)
	}
	return false
}

func ArchiveOffset(archivefile string) (int64, error) {
	rd0, err := zip.OpenReader(archivefile)
	if err != nil {
		slog.Error("open reader", "file", archivefile, "error", err)
		return 0, err
	}
	if len(rd0.File) == 0 {
		slog.Error("no content", "file", archivefile, "files", len(rd0.File), "comment", rd0.Comment)
		return 0, fmt.Errorf("no content in file")
	}
	first := rd0.File[0]
	offs, err := first.DataOffset()
	if err != nil {
		slog.Error("dataoffset", "file", archivefile, "error", err)
		return 0, err
	}
	hdrlen := int64(len(first.Name) + len(first.Comment) + len(first.Extra) + ZipLocalFileHeaderSize)
	slog.Debug("first offset", "offset", offs, "header", hdrlen)
	if offs > hdrlen {
		offs -= hdrlen
	}
	err = rd0.Close()
	if err != nil {
		slog.Error("close", "file", archivefile, "error", err)
	}
	return offs, err
}
