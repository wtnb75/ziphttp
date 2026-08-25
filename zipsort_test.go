package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jessevdk/go-flags"
)

type zipSortTestEntry struct {
	name    string
	content string
	mod     time.Time
}

func writeSortTestZip(t *testing.T, path string, entries []zipSortTestEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal("create", path, err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		fh := &zip.FileHeader{Name: e.name, Modified: e.mod, Method: zip.Store}
		w, err := zw.CreateHeader(fh)
		if err != nil {
			t.Fatal("create header", e.name, err)
		}
		if _, err = w.Write([]byte(e.content)); err != nil {
			t.Fatal("write", e.name, err)
		}
	}
	if err = zw.Close(); err != nil {
		t.Fatal("close writer", err)
	}
	if err = f.Close(); err != nil {
		t.Fatal("close file", err)
	}
}

func readZipNamesInOrder(t *testing.T, path string) []string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal("open output", path, err)
	}
	defer zr.Close()
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names
}

func TestCompareFile(t *testing.T) {
	t.Parallel()
	t.Run("prefer smaller compressed when crc same", func(t *testing.T) {
		a := &zip.File{FileHeader: zip.FileHeader{CRC32: 1, CompressedSize64: 10}}
		b := &zip.File{FileHeader: zip.FileHeader{CRC32: 1, CompressedSize64: 20}}
		if compare_file(a, b) {
			t.Error("a should be preferred")
		}
	})
	t.Run("prefer newer when crc different", func(t *testing.T) {
		a := &zip.File{FileHeader: zip.FileHeader{CRC32: 1, Modified: time.Unix(20, 0)}}
		b := &zip.File{FileHeader: zip.FileHeader{CRC32: 2, Modified: time.Unix(10, 0)}}
		if compare_file(a, b) {
			t.Error("a is newer and should be preferred")
		}
	})
	t.Run("prefer larger when timestamp same", func(t *testing.T) {
		m := time.Unix(10, 0)
		a := &zip.File{FileHeader: zip.FileHeader{CRC32: 1, Modified: m, UncompressedSize64: 200}}
		b := &zip.File{FileHeader: zip.FileHeader{CRC32: 2, Modified: m, UncompressedSize64: 100}}
		if compare_file(a, b) {
			t.Error("a is larger and should be preferred")
		}
	})
	t.Run("same crc and same csize prefers older", func(t *testing.T) {
		a := &zip.File{FileHeader: zip.FileHeader{CRC32: 1, CompressedSize64: 10, Modified: time.Unix(20, 0)}}
		b := &zip.File{FileHeader: zip.FileHeader{CRC32: 1, CompressedSize64: 10, Modified: time.Unix(10, 0)}}
		if !compare_file(a, b) {
			t.Error("b is older and should be preferred")
		}
	})
	t.Run("same crc and same csize same time keeps first", func(t *testing.T) {
		m := time.Unix(10, 0)
		a := &zip.File{FileHeader: zip.FileHeader{CRC32: 1, CompressedSize64: 10, Modified: m}}
		b := &zip.File{FileHeader: zip.FileHeader{CRC32: 1, CompressedSize64: 10, Modified: m}}
		if compare_file(a, b) {
			t.Error("first should be kept")
		}
	})
}

func TestPrepareOutput(t *testing.T) {
	t.Parallel()
	t.Run("normal", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out.zip")
		ofp, zw, err := prepare_output(out, false)
		if err != nil {
			t.Error("prepare_output", err)
			return
		}
		if err = zw.Close(); err != nil {
			t.Error("close zip writer", err)
		}
		if err = ofp.Close(); err != nil {
			t.Error("close output", err)
		}
		st, err := os.Stat(out)
		if err != nil {
			t.Error("stat", err)
			return
		}
		if st.Mode()&0o111 != 0 {
			t.Error("normal output should not be executable", st.Mode())
		}
	})
	t.Run("self", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out-self.zip")
		ofp, zw, err := prepare_output(out, true)
		if err != nil {
			t.Error("prepare_output self", err)
			return
		}
		if err = zw.Close(); err != nil {
			t.Error("close zip writer", err)
		}
		if err = ofp.Close(); err != nil {
			t.Error("close output", err)
		}
		st, err := os.Stat(out)
		if err != nil {
			t.Error("stat", err)
			return
		}
		if st.Size() == 0 {
			t.Error("expected copied executable contents")
		}
	})
}

func TestZipSortExecuteOrder(t *testing.T) {
	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false

	inzip := filepath.Join(t.TempDir(), "in.zip")
	writeSortTestZip(t, inzip, []zipSortTestEntry{
		{name: "a.txt", content: "AAAA", mod: time.Unix(300, 0)},
		{name: "b.txt", content: "BB", mod: time.Unix(200, 0)},
		{name: "c.txt", content: "CCCCCC", mod: time.Unix(100, 0)},
	})

	tests := []struct {
		name     string
		cmd      ZipSort
		expected []string
	}{
		{name: "name asc", cmd: ZipSort{SortBy: "name"}, expected: []string{"a.txt", "b.txt", "c.txt"}},
		{name: "name desc", cmd: ZipSort{SortBy: "name", Reverse: true}, expected: []string{"c.txt", "b.txt", "a.txt"}},
		{name: "time newest first (default)", cmd: ZipSort{SortBy: "time"}, expected: []string{"a.txt", "b.txt", "c.txt"}},
		{name: "time oldest first (reverse)", cmd: ZipSort{SortBy: "time", Reverse: true}, expected: []string{"c.txt", "b.txt", "a.txt"}},
		{name: "usize smallest first (default)", cmd: ZipSort{SortBy: "usize"}, expected: []string{"b.txt", "a.txt", "c.txt"}},
		{name: "usize largest first (reverse)", cmd: ZipSort{SortBy: "usize", Reverse: true}, expected: []string{"c.txt", "a.txt", "b.txt"}},
		{name: "csize smallest first (default)", cmd: ZipSort{SortBy: "csize"}, expected: []string{"b.txt", "a.txt", "c.txt"}},
		{name: "csize largest first (reverse)", cmd: ZipSort{SortBy: "csize", Reverse: true}, expected: []string{"c.txt", "a.txt", "b.txt"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outzip := filepath.Join(t.TempDir(), "out.zip")
			globalOption.Archive = flags.Filename(outzip)
			if err := tc.cmd.Execute([]string{inzip}); err != nil {
				t.Fatal("execute", err)
			}
			got := readZipNamesInOrder(t, outzip)
			if len(got) != len(tc.expected) {
				t.Fatalf("got %v, want %v", got, tc.expected)
			}
			for i := range got {
				if got[i] != tc.expected[i] {
					t.Errorf("order mismatch at %d: got %v, want %v", i, got, tc.expected)
					break
				}
			}
		})
	}
}

func TestZipSortExecuteStripPrefix(t *testing.T) {
	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false

	inzip := filepath.Join(t.TempDir(), "in.zip")
	writeSortTestZip(t, inzip, []zipSortTestEntry{
		{name: "prefix/a.txt", content: "AAAA", mod: time.Unix(100, 0)},
	})

	outzip := filepath.Join(t.TempDir(), "out.zip")
	globalOption.Archive = flags.Filename(outzip)
	cmd := ZipSort{SortBy: "none", StripPrefix: []string{"prefix/"}}
	if err := cmd.Execute([]string{inzip}); err != nil {
		t.Fatal("execute", err)
	}
	got := readZipNamesInOrder(t, outzip)
	if len(got) != 1 || got[0] != "a.txt" {
		t.Errorf("expected stripped name [a.txt], got %v", got)
	}
}

func TestZipSortExecuteMergeAcrossZips(t *testing.T) {
	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false

	zipA := filepath.Join(t.TempDir(), "a.zip")
	zipB := filepath.Join(t.TempDir(), "b.zip")
	// same-crc.txt: identical content (same CRC) in both inputs, but A is
	// older - compare_file must keep the older duplicate on a CRC match.
	// diff-crc.txt: different content (different CRC), B is newer -
	// compare_file must keep the newer duplicate when CRCs differ.
	writeSortTestZip(t, zipA, []zipSortTestEntry{
		{name: "same-crc.txt", content: "identical", mod: time.Unix(100, 0)},
		{name: "diff-crc.txt", content: "alpha", mod: time.Unix(50, 0)},
	})
	writeSortTestZip(t, zipB, []zipSortTestEntry{
		{name: "same-crc.txt", content: "identical", mod: time.Unix(200, 0)},
		{name: "diff-crc.txt", content: "beta-x", mod: time.Unix(999, 0)},
	})

	outzip := filepath.Join(t.TempDir(), "out.zip")
	globalOption.Archive = flags.Filename(outzip)
	cmd := ZipSort{SortBy: "name"}
	if err := cmd.Execute([]string{zipA, zipB}); err != nil {
		t.Fatal("execute", err)
	}

	zr, err := zip.OpenReader(outzip)
	if err != nil {
		t.Fatal("open output", err)
	}
	defer zr.Close()
	if len(zr.File) != 2 {
		t.Fatalf("expected exactly one entry per name, got %d files", len(zr.File))
	}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal("open entry", f.Name, err)
		}
		buf := make([]byte, 32)
		n, _ := rc.Read(buf)
		rc.Close()
		got := string(buf[:n])
		switch f.Name {
		case "same-crc.txt":
			if !f.Modified.Equal(time.Unix(100, 0)) {
				t.Errorf("same-crc.txt: expected the older duplicate (A) to win, got modtime %v", f.Modified)
			}
		case "diff-crc.txt":
			if got != "beta-x" {
				t.Errorf("diff-crc.txt: expected the newer duplicate (B, %q) to win, got %q", "beta-x", got)
			}
		default:
			t.Errorf("unexpected entry %s", f.Name)
		}
	}
}

func TestZipSortExecuteOpenReaderError(t *testing.T) {
	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false

	outzip := filepath.Join(t.TempDir(), "out.zip")
	globalOption.Archive = flags.Filename(outzip)
	cmd := ZipSort{SortBy: "none"}
	if err := cmd.Execute([]string{"/no/such/input.zip"}); err == nil {
		t.Error("expected error opening a nonexistent input zip")
	}
}

func TestZipSortExecute(t *testing.T) {
	inzip := prepare_testzip(t)
	tests := []struct {
		name   string
		cmd    ZipSort
		inputs []string
	}{
		{name: "sort by name", cmd: ZipSort{SortBy: "name"}, inputs: []string{inzip}},
		{name: "sort by time reverse", cmd: ZipSort{SortBy: "time", Reverse: true}, inputs: []string{inzip}},
		{name: "sort by usize", cmd: ZipSort{SortBy: "usize"}, inputs: []string{inzip}},
		{name: "sort by csize reverse", cmd: ZipSort{SortBy: "csize", Reverse: true}, inputs: []string{inzip}},
		{name: "sort none with exclude", cmd: ZipSort{SortBy: "none", Exclude: []string{"512b*"}}, inputs: []string{inzip}},
	}

	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false

	for idx, tc := range tests {
		outzip := filepath.Join(t.TempDir(), "sorted-"+tc.name+".zip")
		globalOption.Archive = flags.Filename(outzip)
		if err := tc.cmd.Execute(tc.inputs); err != nil {
			t.Error("execute", idx, tc.name, err)
			continue
		}
		st, err := os.Stat(outzip)
		if err != nil {
			t.Error("stat output", idx, tc.name, err)
			continue
		}
		if st.Size() == 0 {
			t.Error("empty output", idx, tc.name)
		}
	}
}
