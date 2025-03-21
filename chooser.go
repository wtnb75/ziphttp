package main

import (
	"archive/zip"
	"fmt"
	"hash/crc32"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type ChooseFile struct {
	Root             string
	ZipRoot          *zip.ReadCloser
	ZipFile          *zip.File
	Name             string
	CRC32            uint32
	ModTime          time.Time
	UncompressedSize uint64
	CompressedSize   uint64
}

type SameCRC []*ChooseFile

func (c SameCRC) Len() int {
	return len(c)
}

func (c SameCRC) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c SameCRC) Less(i, j int) bool {
	// if both compressed, smaller is better
	if c[i].CompressedSize != 0 && c[j].CompressedSize != 0 && c[i].CompressedSize != c[j].CompressedSize {
		return c[i].CompressedSize < c[j].CompressedSize
	}
	// compressed is better
	if c[i].CompressedSize != 0 && c[j].CompressedSize == 0 {
		return true
	} else if c[i].CompressedSize == 0 && c[j].CompressedSize != 0 {
		return false
	}
	// both uncompressed or same compressed size, older is better
	if c[i].ModTime != c[j].ModTime {
		return c[i].ModTime.Before(c[j].ModTime)
	}
	// same timestamp, bigger is better
	return c[i].UncompressedSize > c[j].UncompressedSize
}

type DiffCRC []*ChooseFile

func (c DiffCRC) Len() int {
	return len(c)
}

func (c DiffCRC) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c DiffCRC) Less(i, j int) bool {
	// newer is better
	if c[i].ModTime != c[j].ModTime {
		return c[i].ModTime.After(c[j].ModTime)
	}
	// same timestamp, bigger is better
	if c[i].UncompressedSize != c[j].UncompressedSize {
		return c[i].UncompressedSize > c[j].UncompressedSize
	}
	// same uncompressed size, compressed smaller is better
	if c[i].CompressedSize != 0 && c[j].CompressedSize != 0 {
		return c[i].CompressedSize < c[j].CompressedSize
	}
	// compressed is better
	if c[i].CompressedSize != 0 && c[j].CompressedSize == 0 {
		return true
	} else if c[i].CompressedSize == 0 && c[j].CompressedSize != 0 {
		return false
	}
	return true
}

func NewChooseFileFromDir(root, name string) *ChooseFile {
	realpath := filepath.Join(root, name)
	st, err := os.Stat(realpath)
	if err != nil {
		slog.Error("stat", "error", err)
		return nil
	}
	return &ChooseFile{
		Root:             root,
		Name:             name,
		ModTime:          st.ModTime(),
		UncompressedSize: uint64(st.Size()),
	}
}

func NewChooseFileFromZip(zipname string, root *zip.ReadCloser, zf *zip.File) *ChooseFile {
	return &ChooseFile{
		Root:             zipname,
		ZipRoot:          root,
		ZipFile:          zf,
		Name:             zf.Name,
		ModTime:          zf.Modified,
		UncompressedSize: zf.UncompressedSize64,
		CompressedSize:   zf.CompressedSize64,
		CRC32:            zf.CRC32,
	}
}

func ChooseFrom(input []*ChooseFile, baseurl string) *ChooseFile {
	if len(input) == 0 {
		return nil
	}
	if len(input) == 1 {
		return input[0]
	}
	// group by CRC32
	group := map[uint32][]*ChooseFile{}
	for _, v := range input {
		if v.CRC32 == 0 {
			if err := v.FixCRC(baseurl); err != nil {
				slog.Error("cannot calculate CRC", "root", v.Root, "name", v.Name, "error", err)
				continue
			}
		}
		if _, ok := group[v.CRC32]; !ok {
			group[v.CRC32] = make([]*ChooseFile, 0)
		}
		group[v.CRC32] = append(group[v.CRC32], v)
	}
	slog.Debug("groups", "name", input[0].Name, "num", len(group))
	differs := []*ChooseFile{}
	for _, v := range group {
		sort.Sort(SameCRC(v))
		differs = append(differs, v[0])
	}
	sort.Sort(DiffCRC(differs))
	return differs[0]
}

func ChooseFromNoCRC(input []*ChooseFile) *ChooseFile {
	if len(input) == 0 {
		return nil
	}
	if len(input) == 1 {
		return input[0]
	}
	tmp := make([]*ChooseFile, len(input))
	n := copy(tmp, input)
	if n != len(input) {
		slog.Error("copy failed?", "src", len(input), "dest", len(tmp))
	}
	sort.Sort(DiffCRC(tmp))
	return tmp[0]
}

func ChooseFromLast(input []*ChooseFile, baseurl string) *ChooseFile {
	if len(input) == 0 {
		return nil
	}
	if len(input) == 1 {
		return input[0]
	}
	chosen := input[len(input)-1]
	if err := chosen.FixCRC(baseurl); err != nil {
		slog.Error("cannot calculate crc", "file", chosen.Name, "error", err)
		return chosen
	}
	for _, v := range input {
		if err := v.FixCRC(baseurl); err != nil {
			slog.Error("cannot calculate crc", "file", v.Name, "error", err)
			continue
		}
		if v.CRC32 == chosen.CRC32 {
			return v
		}
	}
	return chosen
}

func (c *ChooseFile) OpenRaw() (io.Reader, error) {
	if c.ZipFile != nil {
		return c.ZipFile.OpenRaw()
	}
	return nil, fmt.Errorf("not a zip")
}

func (c *ChooseFile) Open() (io.ReadCloser, error) {
	if c.ZipFile != nil {
		return c.ZipFile.Open()
	}
	return os.Open(filepath.Join(c.Root, c.Name))
}

func (c *ChooseFile) FixCRC(baseurl string) error {
	if c.CRC32 != 0 || c.ZipRoot != nil {
		return nil
	}
	fi, err := os.Open(filepath.Join(c.Root, c.Name))
	if err != nil {
		return err
	}
	defer fi.Close()
	cksum := crc32.NewIEEE()
	if baseurl != "" {
		r, w := io.Pipe()
		defer r.Close()
		go func() {
			defer w.Close()
			written, err := io.Copy(w, fi)
			if err != nil {
				slog.Error("copy to buf", "error", err, "written", written)
			}
		}()
		if err = LinkRelative(baseurl, r, cksum); err != nil {
			slog.Error("link relative", "error", err)
			return err
		}
	} else {
		var written int64
		written, err = io.Copy(cksum, fi)
		if err != nil {
			slog.Error("copy to checksum", "error", err, "written", written)
			return err
		}
		slog.Debug("copy to checksum", "written", written)
	}
	hashval := cksum.Sum32()
	slog.Debug("crc32", "name", c.Name, "value", hashval)
	c.CRC32 = hashval
	return nil
}

func (c *ChooseFile) Header() zip.FileHeader {
	if c.ZipFile != nil {
		return c.ZipFile.FileHeader
	}
	return zip.FileHeader{
		Name:     c.Name,
		Modified: c.ModTime,
	}
}
