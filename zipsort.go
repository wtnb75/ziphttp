package main

import (
	"archive/zip"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
)

type ZipSort struct {
	StripPrefix []string `long:"strip-prefix" description:"strip prefixes"`
	Exclude     []string `short:"x" long:"exclude" description:"exclude files"`
	SortBy      string   `long:"sort-by" choice:"none" choice:"name" choice:"time" choice:"usize" choice:"csize"`
	Reverse     bool     `short:"r" long:"reverse" description:"reversed order"`
}

func compare_file(a, b *zip.File) bool {
	if a.CRC32 == b.CRC32 {
		if a.CompressedSize64 < b.CompressedSize64 {
			// a: smaller
			return false
		} else if a.CompressedSize64 > b.CompressedSize64 {
			// b: smaller
			return true
		}
		if a.Modified.After(b.Modified) {
			// b: older
			return true
		}
		return false
	}
	if a.Modified.After(b.Modified) {
		// a: newer
		return false
	}
	if a.Modified.Equal(b.Modified) && a.UncompressedSize64 > b.UncompressedSize64 {
		// a: smaller
		return false
	}
	// b: later
	return true
}

func (cmd *ZipSort) Execute(args []string) (err error) {
	init_log()
	var mode os.FileMode
	if globalOption.Self {
		mode = 0755
	} else {
		mode = 0644
	}
	ofp, err := os.OpenFile(string(globalOption.Archive), os.O_RDWR|os.O_CREATE, mode)
	if err != nil {
		slog.Error("openOutput", "path", globalOption.Archive, "error", err)
		return err
	}
	defer ofp.Close()
	err = ofp.Truncate(0)
	if err != nil {
		slog.Error("truncate", "path", globalOption.Archive, "error", err)
		return err
	}
	var written int64
	if globalOption.Self {
		cmd_exc, err := os.Executable()
		if err != nil {
			slog.Error("cmd", "error", err)
			return err
		}
		cmd_fp, err := os.Open(cmd_exc)
		if err != nil {
			slog.Error("cmd open", "name", cmd_exc, "error", err)
			return err
		}
		defer cmd_fp.Close()
		written, err = io.Copy(ofp, cmd_fp)
		if err != nil {
			slog.Error("cmd copy", "name", cmd_exc, "dest", globalOption.Archive, "error", err, "written", written)
			return err
		}
		slog.Debug("copy", "written", written)
		err = ofp.Sync()
		if err != nil {
			slog.Error("sync", "name", cmd_exc, "error", err)
		}
	}
	zipfile := zip.NewWriter(ofp)
	defer zipfile.Close()
	slog.Debug("setoffiset", "written", written)
	zipfile.SetOffset(written)
	files := make(map[string]*zip.File, 0)
	for _, fname := range args {
		zf, err := zip.OpenReader(fname)
		if err != nil {
			slog.Error("OpenReader", "name", fname, "error", err)
			return err
		}
		defer zf.Close()
		for _, f := range zf.File {
			if ismatch(f.Name, cmd.Exclude) {
				continue
			}
			if f.FileInfo().IsDir() {
				continue
			}
			name := f.Name
			for _, pfx := range cmd.StripPrefix {
				name = strings.TrimPrefix(name, pfx)
			}
			if prev, ok := files[name]; !ok {
				slog.Debug("new", "zip", fname, "name", f.Name, "archname", name)
				files[name] = f
			} else {
				if compare_file(prev, f) {
					// f
					slog.Info("update", "zip", fname, "name", f.Name, "arcname", name)
					files[name] = f
				} else {
					// prev
					slog.Info("ignore", "zip", fname, "name", f.Name, "arcname", name)
				}
			}
		}
	}
	slog.Info("read files", "num", len(files))
	// map to array
	names := make([]string, 0)
	for k := range files {
		names = append(names, k)
	}
	// sort by name
	switch cmd.SortBy {
	case "name":
		if cmd.Reverse {
			sort.Sort(sort.Reverse(sort.StringSlice(names)))
		} else {
			sort.Strings(names)
		}
	case "time":
		if cmd.Reverse {
			sort.Slice(names, func(i, j int) bool {
				a := files[names[i]]
				b := files[names[j]]
				return a.Modified.Before(b.Modified)
			})
		} else {
			sort.Slice(names, func(i, j int) bool {
				a := files[names[i]]
				b := files[names[j]]
				return a.Modified.After(b.Modified)
			})
		}
	case "usize":
		if cmd.Reverse {
			sort.Slice(names, func(i, j int) bool {
				a := files[names[i]]
				b := files[names[j]]
				return a.UncompressedSize64 > b.UncompressedSize64
			})
		} else {
			sort.Slice(names, func(i, j int) bool {
				a := files[names[i]]
				b := files[names[j]]
				return a.UncompressedSize64 < b.UncompressedSize64
			})
		}
	case "csize":
		if cmd.Reverse {
			sort.Slice(names, func(i, j int) bool {
				a := files[names[i]]
				b := files[names[j]]
				return a.CompressedSize64 > b.CompressedSize64
			})
		} else {
			sort.Slice(names, func(i, j int) bool {
				a := files[names[i]]
				b := files[names[j]]
				return a.CompressedSize64 < b.CompressedSize64
			})
		}
	default: // "none"
		slog.Info("no sort")
	}
	// output to zipfile
	for _, name := range names {
		rd, err := files[name].OpenRaw()
		if err != nil {
			slog.Error("OpenRaw", "name", files[name].Name, "arcname", name, "error", err)
			return err
		}
		filehead := *files[name]
		filehead.Name = name
		wr, err := zipfile.CreateRaw(&filehead.FileHeader)
		if err != nil {
			slog.Error("CreateRaw", "arcname", name, "error", err)
			return err
		}
		written, err := io.Copy(wr, rd)
		if err != nil && err != io.EOF {
			slog.Error("copy", "arcname", name, "error", err, "written", written)
			return err
		}
		slog.Debug("copied", "arcname", name, "written", written)
		if err = zipfile.Flush(); err != nil {
			slog.Error("flush", "arcname", name, "error", err)
			return err
		}
	}
	return nil
}
