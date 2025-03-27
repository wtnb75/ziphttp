package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/foobaz/go-zopfli/zopfli"
	"golang.org/x/net/html"
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
		slog.Error("invalid url", "error", err, "url", link, "here", here)
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
	if new_url.Scheme == base.Scheme && new_url.Hostname() == base.Hostname() && new_url.Path != "" {
		relpath, err := filepath.Rel(filepath.Dir(base.Path)+"/", new_url.Path)
		if strings.HasSuffix(new_url.Path, "/") && !strings.HasSuffix(relpath, "/") {
			relpath += "/"
		}
		if err != nil {
			slog.Warn("filepath.Rel", "error", err, "link", link, "base", base, "new", new_url, "path", new_url.Path)
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

func dochild(node *html.Node, here string) error {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		slog.Debug("node", "node", c)
		if c.Type == html.ElementNode {
			for idx, v := range c.Attr {
				if v.Key == "href" || v.Key == "src" {
					newlink := fix_link(here, v.Val)
					slog.Debug("fix link", "key", v.Key, "value", v.Val, "new", newlink)
					c.Attr[idx] = html.Attribute{Key: v.Key, Val: newlink}
				}
			}
			if err := dochild(c, here); err != nil {
				slog.Error("traverse-child", "error", err)
				return err
			}
		}
	}
	return nil
}

func LinkRelative_html(here string, reader io.Reader, writer io.Writer) error {
	if !ismatch(strings.ToLower(filepath.Base(here)), []string{"*.html", "*.htm"}) {
		slog.Debug("not match html", "here", here, "base", strings.ToLower(filepath.Base(here)))
		_, err := io.Copy(writer, reader)
		return err
	}
	node, err := html.Parse(reader)
	if err != nil {
		slog.Error("parse", "error", err)
		return err
	}
	err = dochild(node, here)
	if err != nil {
		slog.Error("traverse", "error", err)
		return err
	}
	return html.Render(writer, node)
}

func LinkRelative_xml(here string, reader io.Reader, writer io.Writer) error {
	if !ismatch(strings.ToLower(filepath.Base(here)), []string{"*.xml"}) {
		slog.Debug("not match xml", "here", here, "base", strings.ToLower(filepath.Base(here)))
		_, err := io.Copy(writer, reader)
		return err
	}
	dec := xml.NewDecoder(reader)
	enc := xml.NewEncoder(writer)
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Error("token error", "error", err)
			return err
		}
		switch v := token.(type) {
		case xml.StartElement:
			slog.Debug("startelement", "data", v)
			for idx, a := range v.Attr {
				if a.Name.Local == "href" || a.Name.Local == "src" {
					newlink := fix_link(here, a.Value)
					slog.Debug("fix link", "orig", a.Value, "new", newlink)
					v.Attr[idx] = xml.Attr{Name: a.Name, Value: newlink}
				}
			}
		}
		if err = enc.EncodeToken(token); err != nil {
			slog.Error("encode token", "token", token, "error", err)
			return err
		}
	}
	return nil
}

func LinkRelative(here string, reader io.Reader, writer io.Writer) error {
	/*
		if ismatch(strings.ToLower(filepath.Base(here)), []string{"*.xml"}) {
			return LinkRelative_xml(here, reader, writer)
		}
	*/
	if ismatch(strings.ToLower(filepath.Base(here)), []string{"*.html", "*.htm"}) {
		return LinkRelative_html(here, reader, writer)
	}
	// others: passthru
	_, err := io.Copy(writer, reader)
	return err
}

func filtercopy(dst io.Writer, src io.Reader, baseurl string) (int64, error) {
	if baseurl != "" {
		rpipe, wpipe := io.Pipe()
		defer rpipe.Close()
		var wg sync.WaitGroup
		wg.Add(1)
		go func(w *sync.WaitGroup) {
			defer w.Done()
			defer wpipe.Close()
			err := LinkRelative(baseurl, src, wpipe)
			if err != nil {
				slog.Error("linkrelative", "error", err, "baseurl", baseurl)
			}
		}(&wg)
		written, err := io.Copy(dst, rpipe)
		if err != nil {
			slog.Error("Copy", "baseurl", baseurl)
		}
		slog.Debug("written", "baseurl", baseurl, "written", written)
		wg.Wait()
		return written, err
	}
	return io.Copy(dst, src)
}

type CompressWork struct {
	Header *zip.FileHeader
	Reader io.Reader
	MyURL  string
}

func CompressWorker(name string, wr *zip.Writer, ch <-chan CompressWork, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		job, ok := <-ch
		if !ok {
			slog.Debug("channel closed", "name", name)
			if err := wr.Close(); err != nil {
				slog.Error("Close", "name", name)
			}
			return
		}
		slog.Debug("work", "name", name, "job", job.Header.Name, "method", job.Header.Method)
		fp, err := wr.CreateHeader(job.Header)
		if err != nil {
			slog.Error("CreateHeader", "name", job.Header.Name, "error", err)
			return
		}
		written, err := filtercopy(fp, job.Reader, job.MyURL)
		if err != nil {
			slog.Error("Copy", "path", name, "url", job.MyURL, "error", err, "written", written)
			return
		}
		slog.Debug("Copy", "path", name, "url", job.MyURL, "name", job.Header.Name, "written", written)
		if err = wr.Flush(); err != nil {
			slog.Error("flush", "path", name, "name", job.Header.Name, "url", job.MyURL, "error", err, "written", written)
		}
		clos := job.Reader.(io.ReadCloser)
		if clos != nil {
			if err = clos.Close(); err != nil {
				slog.Error("close", "path", name, "url", job.MyURL, "error", err)
			}
		}
	}
}

func ZipPassThru(wr *zip.Writer, files []*zip.File) error {
	for _, f := range files {
		ofh := f.FileHeader
		ifp, err := f.OpenRaw()
		if err != nil {
			slog.Error("OpenRaw", "name", f.Name, "error", err)
			return err
		}
		ofp, err := wr.CreateRaw(&ofh)
		if err != nil {
			slog.Error("CreateRaw", "name", ofh.Name, "error", err)
			return err
		}
		written, err := io.Copy(ofp, ifp)
		if err != nil && err != io.EOF {
			slog.Error("copy", "error", err, "name", f.Name, "written", written)
			return err
		}
		slog.Debug("done copy", "written", written, "name", f.Name, "error", err)
		if err = wr.Flush(); err != nil {
			slog.Error("flush", "error", err, "name", f.Name)
			return err
		}
	}
	return nil
}

type DeflateWriteCloser struct {
	opts   zopfli.Options
	output io.Writer
	buf    bytes.Buffer
}

func (d *DeflateWriteCloser) Write(in []byte) (int, error) {
	return d.buf.Write(in)
}

func (d *DeflateWriteCloser) Close() error {
	return zopfli.DeflateCompress(&d.opts, d.buf.Bytes(), d.output)
}

func MakeZopfliWriter(zipfile *zip.Writer) {
	zipfile.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		opts := zopfli.DefaultOptions()
		dc := DeflateWriteCloser{opts: opts, output: out}
		return &dc, nil
	})
}

const (
	Brotli = 91
	Zstd   = 93
)

func MakeBrotliWriter(zipfile *zip.Writer) {
	zipfile.RegisterCompressor(Brotli, func(out io.Writer) (io.WriteCloser, error) {
		return brotli.NewWriter(out), nil
	})
}

func MakeBrotliReader(zipfile *zip.Reader) {
	zipfile.RegisterDecompressor(Brotli, func(input io.Reader) io.ReadCloser {
		return io.NopCloser(brotli.NewReader(input))
	})
}

func MakeBrotliReadCloser(zipfile *zip.ReadCloser) {
	zipfile.RegisterDecompressor(Brotli, func(input io.Reader) io.ReadCloser {
		return io.NopCloser(brotli.NewReader(input))
	})
}
