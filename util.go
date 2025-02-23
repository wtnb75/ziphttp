package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

func ismatch0(name string, patterns []string) bool {
	for _, pat := range patterns {
		if matched, _ := filepath.Match(pat, name); matched {
			slog.Debug("match", "name", name, "pattern", pat)
			return true
		}
	}
	return false
}

func ismatch(name string, patterns []string) bool {
	return ismatch0(filepath.Base(name), patterns)
}

func ispat(head []byte, pat []string) bool {
	content_type := http.DetectContentType(head)
	slog.Debug("ispat", "type", content_type)
	sname := strings.SplitN(content_type, ";", 2)
	if len(sname) != 0 {
		return ismatch0(strings.TrimSpace(sname[0]), pat)
	}
	return false
}

func ArchiveOffset(archivefile string) (int64, error) {
	fp, err := os.Open(archivefile)
	if err != nil {
		slog.Error("open archive", "name", archivefile, "error", err)
		return 0, err
	}
	defer fp.Close()
	cur, err := fp.Seek(-512, io.SeekEnd)
	if err != nil {
		slog.Error("seek", "name", archivefile, "error", err, "cur", cur)
		return 0, err
	}

	// read end of central directory
	tail := make([]byte, 512)
	sz, err := fp.Read(tail)
	if err != nil && err != io.EOF {
		slog.Error("read(tail)", "name", archivefile, "error", err, "size", sz)
		return 0, err
	}
	idx := bytes.LastIndex(tail[0:sz], []byte{0x50, 0x4b, 0x05, 0x06})
	if idx == -1 {
		slog.Error("end of central directory not found", "name", archivefile, "bytes", tail)
	}
	cdsize := binary.LittleEndian.Uint32(tail[idx+0xc : idx+0xc+4])
	cur, err = fp.Seek(-512+int64(idx)-int64(cdsize), io.SeekEnd)
	if err != nil {
		slog.Error("seek central directory head", "name", archivefile, "error", err, "cdsize", cdsize, "cur", cur)
		return 0, err
	}
	cdhead := make([]byte, 0x30)
	_, err = fp.Read(cdhead)
	if err != nil {
		slog.Error("read central directory head", "name", archivefile, "error", err, "cdsize", cdsize)
		return 0, err
	}
	if !bytes.HasPrefix(cdhead, []byte{0x50, 0x4b, 0x1, 0x2}) {
		slog.Error("invalid signature", "signature", cdhead[0:4])
		return 0, err
	}
	return int64(binary.LittleEndian.Uint32(cdhead[0x2a:0x2e])), nil
}

func ArchiveOffset_Old(archivefile string) (int64, error) {
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
	slog.Debug("first offset", "offset", offs, "header", hdrlen, "name", len(first.Name), "comment", len(first.Comment), "extra", len(first.Extra))
	if offs > hdrlen {
		offs -= hdrlen
	}
	err = rd0.Close()
	if err != nil {
		slog.Error("close", "file", archivefile, "error", err)
	}
	return offs, err
}

func fix_link(here string, link string) string {
	u, err := url.Parse(link)
	if err != nil {
		slog.Error("invalid url", "error", err, "url", link)
		return link
	}
	if u.User.String() != "" {
		// URL has username:password
		slog.Debug("url has username:password", "url", u.Redacted())
		return link
	}
	base, err := url.Parse(here)
	if err != nil {
		slog.Warn("invalid base url", "error", err, "url", here)
		return link
	}
	new_url := base.ResolveReference(u)
	if new_url.Scheme == base.Scheme && new_url.Hostname() == base.Hostname() {
		relpath, err := filepath.Rel(filepath.Dir(base.Path)+"/", new_url.Path)
		if strings.HasSuffix(new_url.Path, "/") && !strings.HasSuffix(relpath, "/") {
			relpath += "/"
		}
		if err != nil {
			slog.Warn("filepath.Rel", "error", err, "base", base, "new", new_url)
			return link
		}
		new_url.Host = ""
		new_url.Scheme = ""
		new_url.Path = relpath
		slog.Debug("link change", "base", base, "link", link, "new", new_url)
		return new_url.String()
	}
	slog.Debug("link unchange", "base", base, "link", link, "new", new_url)
	return link
}

func process_line(here string, line string) string {
	link_regex := regexp.MustCompile(`\s*(src|href)\s*=\s*"?([^" >]*)"?`)
	return link_regex.ReplaceAllStringFunc(line, func(part string) string {
		match := link_regex.FindStringSubmatch(part)
		new_link := fix_link(here, match[2])
		return fmt.Sprintf(" %s=\"%s\"", match[1], new_link)
	})
}

func LinkRelative(here string, reader io.Reader, writer io.Writer) error {
	if !ismatch(strings.ToLower(filepath.Base(here)), []string{"*.html", "*.htm", "*.xml"}) {
		_, err := io.Copy(writer, reader)
		return err
	}
	slog.Debug("link relative", "here", here)
	rd := bufio.NewReader(reader)
	for {
		line, err := rd.ReadString('\n')
		if err == io.EOF {
			_, err = writer.Write([]byte(process_line(here, line)))
			if err != nil {
				slog.Error("write(eof)", "line", line)
				return err
			}
			break
		}
		if err != nil {
			slog.Error("readstring", "line", line, "error", err)
			return err
		}
		_, err = writer.Write([]byte(process_line(here, line)))
		if err != nil {
			slog.Error("write", "line", line, "error", err)
			return err
		}
	}
	return nil
}
