package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jessevdk/go-flags"
)

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

func TestZipSortExecute(t *testing.T) {
	t.Parallel()
	inzip := prepare_testzip(t)
	outzip := filepath.Join(t.TempDir(), "sorted.zip")

	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Archive = flags.Filename(outzip)
	globalOption.Self = false

	cmd := ZipSort{SortBy: "name"}
	if err := cmd.Execute([]string{inzip}); err != nil {
		t.Error("execute", err)
		return
	}
	st, err := os.Stat(outzip)
	if err != nil {
		t.Error("stat output", err)
		return
	}
	if st.Size() == 0 {
		t.Error("empty output")
	}
}
