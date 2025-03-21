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

	"github.com/schollz/progressbar/v3"
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
	Last      bool     `long:"choose-last" description:"choose first of same as last"`
	SortBy    string   `long:"sort-by" choice:"none" choice:"name" choice:"time" choice:"usize" choice:"csize"`
	Reverse   bool     `short:"r" long:"reverse" description:"reversed order"`
	InMemory  bool     `long:"in-memory" description:"do not use /tmp"`
	Progress  bool     `long:"progress" description:"show progress bar"`

	nametable   map[string][]*ChooseFile
	zip_to_read []*zip.ReadCloser
	zipios      []ZipIO
}

func (cmd *ZipCmd) prepare_output() (*os.File, *zip.Writer, error) {
	var mode os.FileMode
	if globalOption.Self {
		mode = 0o755
	} else {
		mode = 0o644
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
		defer func() {
			if err := cmd_fp.Close(); err != nil {
				slog.Error("close cmd_fp", "command", cmd_exc, "error", err)
			}
		}()
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
		MakeZopfli(zipfile)
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
	fh.Name = name
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

func (cmd *ZipCmd) validate(args []string) (err error) {
	if cmd.Parallel == 0 {
		cmd.Parallel = uint(runtime.NumCPU())
	}
	return nil
}

func (cmd *ZipCmd) create_nametable(args []string) (err error) {
	lock := sync.Mutex{}
	var file_num uint
	wg := sync.WaitGroup{}
	cmd.nametable = make(map[string][]*ChooseFile, 0)
	cmd.zip_to_read = make([]*zip.ReadCloser, 0)
	for _, dirname := range args {
		st, err := os.Stat(dirname)
		if err != nil {
			slog.Error("stat", "path", dirname, "error", err)
		}
		if st.IsDir() {
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
					relpath, err := filepath.Rel(dirname, path)
					if err != nil {
						slog.Error("Relpath", "root", dirname, "path", path, "error", err)
						return err
					}
					var archivepath string
					if cmd.StripRoot {
						archivepath = relpath
					} else {
						archivepath = path
					}
					slog.Debug("archivepath", "root", dirname, "path", path, "apath", archivepath)
					lock.Lock()
					nameadd(cmd.nametable, archivepath, NewChooseFileFromDir(dirname, relpath))
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
			cmd.zip_to_read = append(cmd.zip_to_read, zipfile)
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
					nameadd(cmd.nametable, fi.Name, NewChooseFileFromZip(dirname, zipfile, fi))
					file_num++
					lock.Unlock()
				}
			}()
		} else if st.Mode().IsRegular() {
			// single file
			filename := filepath.Base(dirname)
			root := filepath.Dir(dirname)
			lock.Lock()
			nameadd(cmd.nametable, filename, NewChooseFileFromDir(root, filename))
			file_num++
			lock.Unlock()
		}
	}
	slog.Info("waiting to generate filelist")
	wg.Wait()
	slog.Info("done", "names", len(cmd.nametable), "files", file_num)
	if cmd.Delete && len(args) != 1 {
		to_del := make([]string, 0)
		for k, v := range cmd.nametable {
			if len(v) == 1 {
				// is first?
				if v[0].Root == args[0] {
					to_del = append(to_del, k)
				}
			}
		}
		for _, name := range to_del {
			slog.Info("remove", "name", name)
			delete(cmd.nametable, name)
		}
	}
	return nil
}

func (cmd *ZipCmd) boot_workers(wgp *sync.WaitGroup) (jobs chan CompressWork, tempdir string, err error) {
	// 0-th zipio is for passthru, 1 to N-th zipio have workers
	if !cmd.UseNormal {
		slog.Info("using zopfli compressor", "workers", cmd.Parallel)
	} else {
		slog.Info("normal compressor", "parallel", cmd.Parallel)
	}
	jobs = make(chan CompressWork, 10)
	cmd.zipios = make([]ZipIO, 0)
	if !cmd.InMemory {
		tempdir, err = os.MkdirTemp("", "")
		if err != nil {
			slog.Error("mkdirtemp", "error", err)
			return
		}
		slog.Info("tmpdir", "name", tempdir)
	}
	for i := range cmd.Parallel + 1 {
		var fi ZipIO
		if !cmd.InMemory {
			fi = NewFileZip(filepath.Join(tempdir, fmt.Sprintf("%d.zip", i)))
		} else {
			fi = NewMemZip()
		}
		cmd.zipios = append(cmd.zipios, fi)
		var wr *zip.Writer
		wr, err = fi.Writer()
		if err != nil {
			slog.Error("worker writer", "number", i)
			return
		}
		if !cmd.UseNormal {
			MakeZopfli(wr)
		}
		if i != 0 {
			wgp.Add(1)
			go CompressWorker(fmt.Sprint(i), wr, jobs, wgp)
		}
	}
	return
}

func (cmd *ZipCmd) generate_sitemap(root *SiteMapRoot, zipwr *zip.Writer) error {
	if cmd.SiteMap != "" {
		slog.Info("generating sitemap", "num", len(root.SiteList))
		fh := zip.FileHeader{
			Name:     "sitemap.xml",
			Method:   zip.Deflate,
			Modified: root.LastMod(),
		}
		if wr, err := zipwr.CreateHeader(&fh); err != nil {
			slog.Error("create sitemap", "error", err)
			xmlstr, err := xml.Marshal(root)
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
			if err := zipwr.Flush(); err != nil {
				slog.Error("sitemap flush", "error", err)
				return err
			}
		}
	}
	return nil
}

func (cmd *ZipCmd) compress_process(sitemap *SiteMapRoot, misc_writer *zip.Writer, jobs chan CompressWork) (err error) {
	selected := make(map[string]int, 0)
	var bar *progressbar.ProgressBar
	if cmd.Progress {
		bar = progressbar.Default(int64(len(cmd.nametable)), string(globalOption.Archive))
		defer bar.Close()
	}
	for k, v := range cmd.nametable {
		if bar != nil {
			if err := bar.Add(1); err != nil {
				slog.Error("progressbar error", "error", err)
				bar = nil
			}
		}
		slog.Debug("process", "name", k, "num", len(v))
		var target *ChooseFile
		if cmd.NoCRC {
			target = ChooseFromNoCRC(v)
		} else if cmd.Last {
			target = ChooseFromLast(v, cmd.BaseURL)
		} else {
			target = ChooseFrom(v, cmd.BaseURL)
		}
		slog.Debug("choose", "name", k, "root", target.Root, "name", target.Name)
		if _, ok := selected[target.Root]; !ok {
			selected[target.Root] = 0
		}
		selected[target.Root]++
		if err := sitemap.AddFile(cmd.SiteMap, "index.html", k, target.ModTime); err != nil {
			slog.Error("sitemap error", "name", k, "error", err)
		}
		if cmd.UseAsIs {
			if err := cmd.copy_asis(misc_writer, k, target); err == nil {
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
	slog.Info("selected", "result", selected)
	if err = cmd.generate_sitemap(sitemap, misc_writer); err != nil {
		slog.Error("sitemap generate", "error", err)
	}
	return nil
}

func (cmd *ZipCmd) Execute(args []string) (err error) {
	init_log()
	if err = cmd.validate(args); err != nil {
		return err
	}
	err = cmd.create_nametable(args)
	if err != nil {
		slog.Error("create nametable", "error", err)
		return err
	}
	defer func() {
		for _, f := range cmd.zip_to_read {
			if err := f.Close(); err != nil {
				slog.Error("close zip", "error", err)
			}
		}
	}()
	// boot workers
	var wgp sync.WaitGroup
	jobs, tempdir, err := cmd.boot_workers(&wgp)
	if tempdir != "" {
		defer func() {
			if err := os.RemoveAll(tempdir); err != nil {
				slog.Error("tempdir remove", "name", tempdir, "error", err)
			}
		}()
	}
	if err != nil {
		slog.Error("boot workers", "error", err)
		return err
	}
	misc_writer, err := cmd.zipios[0].Writer()
	// sitemap
	sitemap := SiteMapRoot{}
	if err := sitemap.initialize(); err != nil {
		slog.Error("sitemap initialize", "error", err)
		return err
	}
	if _, ok := cmd.nametable["sitemap.xml"]; ok && cmd.SiteMap != "" {
		slog.Info("disable sitemap: already exists")
		cmd.SiteMap = ""
	}
	// process nametable
	if err = cmd.compress_process(&sitemap, misc_writer, jobs); err != nil {
		slog.Error("compress failed", "error", err)
		return err
	}
	slog.Debug("close jobs")
	close(jobs)
	slog.Debug("before wait")
	wgp.Wait()
	slog.Debug("close zipio[0]", "zipio[0]", cmd.zipios[0])
	if err = misc_writer.Close(); err != nil {
		slog.Error("misc writer close", "error", err)
	}
	if err = cmd.zipios[0].Close(); err != nil {
		slog.Error("0-th zipio close", "error", err)
	}
	slog.Info("wait done")
	ofp, zipfile, err := cmd.prepare_output()
	if ofp != nil {
		defer func() {
			if err := ofp.Close(); err != nil {
				slog.Error("close ofp", "error", err)
			}
		}()
	}
	if zipfile != nil {
		defer func() {
			if err := zipfile.Close(); err != nil {
				slog.Error("close zipfile", "error", err)
			}
		}()
	}
	if err != nil {
		slog.Error("open output", "error", err)
		return err
	}
	fileinzips := make([]*zip.File, 0)
	for idx, z := range cmd.zipios {
		reader, err := z.Reader()
		if err != nil {
			slog.Error("open reader", "error", err, "idx", idx, "zipio", z)
			return err
		}
		fileinzips = append(fileinzips, reader.File...)
	}
	cmd.sort_files(fileinzips)
	slog.Info("all files", "num", len(fileinzips))
	if err = ZipPassThru(zipfile, fileinzips); err != nil {
		slog.Error("ZipPassthru", "error", err)
	}
	return nil
}
