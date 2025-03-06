package main

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

type ZipCmd struct {
	StripRoot bool     `short:"s" long:"strip-root" description:"strip root path"`
	Exclude   []string `short:"x" long:"exclude" description:"exclude files"`
	Stored    []string `short:"n" long:"stored" description:"non compress patterns"`
	MinSize   uint     `short:"m" long:"min-size" description:"compress minimum size" default:"512"`
	UseNormal bool     `long:"no-zopfli" description:"do not use zopfli compress"`
	UseAsIs   bool     `long:"asis" description:"copy as-is from zipfile"`
	BaseURL   string   `long:"baseurl" description:"rewrite html link to relative"`
	SiteMap   string   `long:"sitemap" description:"generate sitemap.xml"`
	Parallel  uint     `short:"p" long:"parallel" description:"parallel compression"`
	Delete    bool     `long:"delete" description:"skip removed files"`
	NoCRC     bool     `long:"no-crc" description:"do not use CRC32 to detect change"`
	SortBy    string   `long:"sort-by" choice:"none" choice:"name" choice:"time" choice:"usize" choice:"csize"`
	Reverse   bool     `short:"r" long:"reverse" description:"reversed order"`
}

func (cmd *ZipCmd) prepare_output() (*os.File, *zip.Writer, error) {
	var mode os.FileMode
	if globalOption.Self {
		mode = 0755
	} else {
		mode = 0644
	}
	ofp, err := os.OpenFile(string(globalOption.Archive), os.O_RDWR|os.O_CREATE, mode)
	if err != nil {
		slog.Error("openOutput", "path", globalOption.Archive, "error", err)
		return nil, nil, err
	}
	err = ofp.Truncate(0)
	if err != nil {
		slog.Error("truncate", "path", globalOption.Archive, "error", err)
		ofp.Close()
		return nil, nil, err
	}
	var written int64
	if globalOption.Self {
		cmd_exc, err := os.Executable()
		if err != nil {
			slog.Error("cmd", "error", err)
			ofp.Close()
			return nil, nil, err
		}
		cmd_fp, err := os.Open(cmd_exc)
		if err != nil {
			slog.Error("cmd open", "name", cmd_exc, "error", err)
			ofp.Close()
			return nil, nil, err
		}
		defer cmd_fp.Close()
		written, err = io.Copy(ofp, cmd_fp)
		if err != nil {
			slog.Error("cmd copy", "name", cmd_exc, "dest", globalOption.Archive, "error", err, "written", written)
			ofp.Close()
			return nil, nil, err
		}
		slog.Debug("copy", "written", written)
		err = ofp.Sync()
		if err != nil {
			slog.Error("sync", "name", cmd_exc, "error", err)
		}
	}
	zipfile := zip.NewWriter(ofp)
	slog.Debug("setoffiset", "written", written)
	zipfile.SetOffset(written)
	if !cmd.UseNormal {
		slog.Info("using zopfli compressor")
		MakeZopfli(zipfile)
	} else {
		slog.Info("using normal compressor")
	}
	return ofp, zipfile, nil
}

func nameadd(nametable map[string][]*ChooseFile, name string, file *ChooseFile) {
	slog.Debug("name add", "name", name, "file", file.Name)
	if target, ok := nametable[name]; !ok {
		nametable[name] = []*ChooseFile{file}
	} else {
		nametable[name] = append(target, file)
	}
}

func (cmd *ZipCmd) copy_asis(output *zip.Writer, name string, input *ChooseFile) (err error) {
	ifp, err := input.OpenRaw()
	if err != nil {
		slog.Debug("OpenRaw", "name", name, "in-zip", input.Name, "error", err)
		return err
	}
	fh := input.Header()
	fh.Method = input.ZipFile.Method
	fh.Name = name
	wr, err := output.CreateRaw(&fh)
	if err != nil {
		slog.Error("CreateRaw", "name", name, "error", err)
		return err
	}
	written, err := io.Copy(wr, ifp)
	if err != nil {
		slog.Error("Copy(asis)", "name", name, "error", err, "written", written)
		return err
	}
	slog.Debug("Copy(asis)", "name", name, "written", written)
	return nil
}

func (cmd *ZipCmd) copy_compress_job(jobs chan<- CompressWork, name string, input *ChooseFile) (err error) {
	fh := input.Header()
	fh.Method = zip.Deflate
	if input.UncompressedSize < uint64(cmd.MinSize) {
		fh.Method = zip.Store
	}
	if ismatch(name, cmd.Stored) {
		fh.Method = zip.Store
	}
	ifp, err := input.Open()
	if err != nil {
		slog.Error("source Open", "name", name, "error", err)
		return err
	}
	myurl := ""
	if cmd.BaseURL != "" {
		myurl, err = url.JoinPath(cmd.BaseURL, name)
		if err != nil {
			slog.Error("url join", "baseurl", cmd.BaseURL, "name", name)
			ifp.Close()
			return err
		}
	}
	cw := CompressWork{
		Header: &fh,
		Reader: ifp,
		MyURL:  myurl,
	}
	jobs <- cw
	return nil
}

func (cmd *ZipCmd) sort_files(files []*zip.File) {
	switch cmd.SortBy {
	case "name":
		if cmd.Reverse {
			sort.Slice(files, func(i, j int) bool {
				return files[i].Name > files[j].Name
			})
		} else {
			sort.Slice(files, func(i, j int) bool {
				return files[i].Name < files[j].Name
			})
		}
	case "time":
		if cmd.Reverse {
			sort.Slice(files, func(i, j int) bool {
				return files[i].Modified.After(files[j].Modified)
			})
		} else {
			sort.Slice(files, func(i, j int) bool {
				return files[i].Modified.Before(files[j].Modified)
			})
		}
	case "usize":
		if cmd.Reverse {
			sort.Slice(files, func(i, j int) bool {
				return files[i].UncompressedSize64 > files[j].UncompressedSize64
			})
		} else {
			sort.Slice(files, func(i, j int) bool {
				return files[i].UncompressedSize64 < files[j].UncompressedSize64
			})
		}
	case "csize":
		if cmd.Reverse {
			sort.Slice(files, func(i, j int) bool {
				return files[i].CompressedSize64 > files[j].CompressedSize64
			})
		} else {
			sort.Slice(files, func(i, j int) bool {
				return files[i].CompressedSize64 < files[j].CompressedSize64
			})
		}
	default: // "none"
		slog.Info("no sort")
	}
}

func (cmd *ZipCmd) Execute(args []string) (err error) {
	init_log()
	if cmd.Parallel == 0 {
		cmd.Parallel = uint(runtime.NumCPU())
	}
	slog.Info("parallel", "num", cmd.Parallel)
	if cmd.Parallel != 1 && cmd.SortBy != "" && cmd.SortBy != "none" {
		slog.Warn("non-parallel and sort is not supported", "sortby", cmd.SortBy, "parallel", cmd.Parallel)
	}
	if !cmd.NoCRC && cmd.BaseURL != "" {
		slog.Warn("make url relative (--baseurl) without --no-crc is not supported", "baseurl", cmd.BaseURL, "nocrc", cmd.NoCRC)
	}
	ofp, zipfile, err := cmd.prepare_output()
	if err != nil {
		slog.Error("open output", "error", err)
		return err
	}
	if ofp != nil {
		defer ofp.Close()
	}
	if zipfile != nil {
		defer zipfile.Close()
	}
	lock := sync.Mutex{}
	var file_num uint
	wg := sync.WaitGroup{}
	nametable := map[string][]*ChooseFile{}
	first_key_set := false
	var first_key_root string
	var first_key_zip *zip.ReadCloser
	for _, dirname := range args {
		st, err := os.Stat(dirname)
		if err != nil {
			slog.Error("stat", "path", dirname, "error", err)
		}
		if st.IsDir() {
			if !first_key_set {
				first_key_root = dirname
				first_key_set = true
			}
			// walk dir
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := filepath.WalkDir(dirname, func(path string, info fs.DirEntry, err1 error) error {
					if err1 != nil {
						slog.Error("walk", "path", path, "error", err1)
						return nil
					}
					if info.IsDir() {
						slog.Debug("isdir", "root", dirname, "path", path)
						return nil
					}
					if ismatch(path, cmd.Exclude) {
						slog.Debug("exclude-match", "path", path, "exclude", cmd.Exclude)
						return nil
					}
					var archivepath string
					if cmd.StripRoot {
						archivepath, err = filepath.Rel(dirname, path)
						if err != nil {
							slog.Error("Relpath", "root", dirname, "path", path, "error", err)
							return err
						}
					} else {
						archivepath = path
					}
					slog.Debug("archivepath", "root", dirname, "path", path, "apath", archivepath)
					lock.Lock()
					nameadd(nametable, archivepath, NewChooseFileFromDir(dirname, archivepath))
					file_num++
					lock.Unlock()
					return nil
				})
				if err != nil {
					slog.Error("walk", "error", err)
				}
			}()
		} else if filepath.Ext(dirname) == ".zip" {
			// zip file
			zipfile, err := zip.OpenReader(dirname)
			if err != nil {
				slog.Error("zip open", "name", dirname, "error", err)
			}
			defer zipfile.Close()
			if !first_key_set {
				first_key_zip = zipfile
				first_key_set = true
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, fi := range zipfile.File {
					if fi.FileInfo().IsDir() {
						slog.Debug("isdir", "root", dirname, "path", fi.Name)
						continue
					}
					if ismatch(fi.Name, cmd.Exclude) {
						slog.Debug("exclude-match", "path", fi.Name, "exclude", cmd.Exclude)
						continue
					}
					lock.Lock()
					nameadd(nametable, fi.Name, NewChooseFileFromZip(zipfile, fi))
					file_num++
					lock.Unlock()
				}
			}()
		} else if st.Mode().IsRegular() {
			// single file
			filename := filepath.Base(dirname)
			root := filepath.Dir(dirname)
			if !first_key_set {
				first_key_root = root
				first_key_set = true
			}
			lock.Lock()
			nameadd(nametable, filename, NewChooseFileFromDir(root, filename))
			file_num++
			lock.Unlock()
		}
	}
	slog.Info("waiting to generate filelist")
	wg.Wait()
	slog.Info("done", "names", len(nametable), "files", file_num)
	if cmd.Delete && len(args) != 1 {
		to_del := make([]string, 0)
		for k, v := range nametable {
			if len(v) == 1 {
				// is first?
				if first_key_root != "" && v[0].Root == first_key_root {
					to_del = append(to_del, k)
				}
				if first_key_zip != nil && v[0].ZipRoot == first_key_zip {
					to_del = append(to_del, k)
				}
			}
		}
		for _, name := range to_del {
			slog.Info("remove", "name", name)
			delete(nametable, name)
		}
	}
	// boot workers
	if cmd.Parallel == 0 {
		cmd.Parallel = uint(runtime.NumCPU())
	}
	slog.Info("parallel", "num", cmd.Parallel)
	var td string
	var wgp sync.WaitGroup
	jobs := make(chan CompressWork, 10)
	var asiszip *zip.Writer
	if cmd.Parallel <= 1 {
		asiszip = zipfile
		wgp.Add(1)
		go CompressWorker("root", zipfile, jobs, &wgp)
	} else {
		td, err = os.MkdirTemp("", "")
		if err != nil {
			slog.Error("mkdirtemp", "error", err)
			return err
		}
		slog.Info("tmpdir", "name", td)
		defer os.RemoveAll(td)
		asisfp, err := os.Create(filepath.Join(td, "asis.zip"))
		if err != nil {
			slog.Error("create tempfile", "name", "asis.zip")
			return err
		}
		defer asisfp.Close()
		asiszip = zip.NewWriter(asisfp)
		if !cmd.UseNormal {
			MakeZopfli(asiszip)
		}
		for i := range cmd.Parallel {
			tf := filepath.Join(td, fmt.Sprintf("%d.zip", i))
			fi, err := os.Create(tf)
			if err != nil {
				slog.Error("create tempfile", "name", tf)
				return err
			}
			defer fi.Close()
			wr := zip.NewWriter(fi)
			if !cmd.UseNormal {
				MakeZopfli(wr)
			}
			wgp.Add(1)
			go CompressWorker(filepath.Base(tf), wr, jobs, &wgp)
		}
	}
	// sitemap
	sitemap := SiteMapRoot{}
	if err := sitemap.initialize(); err != nil {
		slog.Error("sitemap initialize", "error", err)
		return err
	}
	if _, ok := nametable["sitemap.xml"]; ok && cmd.SiteMap != "" {
		slog.Info("disable sitemap: already exists")
		cmd.SiteMap = ""
	}
	// process nametable
	for k, v := range nametable {
		slog.Debug("process", "name", k, "num", len(v))
		var target *ChooseFile
		if cmd.NoCRC {
			target = ChooseFromNoCRC(v)
		} else {
			target = ChooseFrom(v)
		}
		if target.Root != "" {
			slog.Debug("win-file", "name", k, "root", target.Root, "name", target.Name)
		} else {
			slog.Debug("win-zip", "name", k, "name", target.Name)
		}
		if cmd.SiteMap != "" {
			if err := sitemap.AddFile(cmd.SiteMap, "index.html", k, target.ModTime); err != nil {
				slog.Error("sitemap error", "name", k, "error", err)
			}
		}
		if cmd.UseAsIs {
			if err := cmd.copy_asis(asiszip, k, target); err == nil {
				continue
			} else {
				slog.Debug("asis failed", "name", k, "error", err)
			}
		}
		// re-compress
		if err := cmd.copy_compress_job(jobs, k, target); err != nil {
			slog.Error("compress failed", "name", k, "error", err)
			return err
		}
	}
	if cmd.SiteMap != "" {
		fh := zip.FileHeader{
			Name:     "sitemap.xml",
			Method:   zip.Deflate,
			Modified: sitemap.LastMod,
		}
		if wr, err := asiszip.CreateHeader(&fh); err != nil {
			slog.Error("create sitemap", "error", err)
			xmlstr, err := xml.Marshal(sitemap)
			if err != nil {
				slog.Error("sitemap generate error", "error", err)
				return err
			}
			written, err := wr.Write(xmlstr)
			if err != nil {
				slog.Error("write sitemap", "error", err, "written", written)
				return err
			}
			slog.Debug("sitemap written", "written", written)
			if err := asiszip.Flush(); err != nil {
				slog.Error("sitemap flush", "error", err)
				return err
			}
		}
	}
	slog.Info("close jobs")
	close(jobs)
	slog.Info("before wait")
	wgp.Wait()
	if err = asiszip.Close(); err != nil {
		slog.Error("close-asiszip", "error", err)
	}
	slog.Info("wait done")
	if cmd.Parallel > 1 {
		fileinzips := make([]*zip.File, 0)
		fname := filepath.Join(td, "asis.zip")
		zr, err := zip.OpenReader(fname)
		if err != nil {
			slog.Error("OpenReader", "name", fname, "error", err)
		}
		defer zr.Close()
		fileinzips = append(fileinzips, zr.File...)
		for i := range cmd.Parallel {
			fname := filepath.Join(td, fmt.Sprintf("%d.zip", i))
			zr, err := zip.OpenReader(fname)
			if err != nil {
				slog.Error("OpenReader", "name", fname, "error", err)
			}
			defer zr.Close()
			fileinzips = append(fileinzips, zr.File...)
			slog.Info("merge", "name", filepath.Base(fname), "files", len(zr.File))
		}
		cmd.sort_files(fileinzips)
		slog.Info("all files", "num", len(fileinzips))
		if err = ZipPassThru(zipfile, fileinzips); err != nil {
			slog.Error("ZipPassthru", "error", err)
		}
	}
	return nil
}
