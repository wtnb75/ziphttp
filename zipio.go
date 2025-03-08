package main

import (
	"archive/zip"
	"bytes"
	"os"
)

type ZipIO interface {
	Reader() (*zip.Reader, error)
	Writer() (*zip.Writer, error)
	Close() error
}

type FileZip struct {
	name string
	fp   *os.File
}

func NewFileZip(name string) *FileZip {
	return &FileZip{name: name}
}

func (f FileZip) Reader() (*zip.Reader, error) {
	if err := f.Close(); err != nil {
		return nil, err
	}
	fp, err := os.Open(f.name)
	if err != nil {
		return nil, err
	}
	f.fp = fp
	st, err := f.fp.Stat()
	if err != nil {
		return nil, err
	}
	return zip.NewReader(f.fp, st.Size())
}

func (f FileZip) Writer() (*zip.Writer, error) {
	if err := f.Close(); err != nil {
		return nil, err
	}
	fp, err := os.Create(f.name)
	if err != nil {
		return nil, err
	}
	f.fp = fp
	return zip.NewWriter(f.fp), nil
}

func (f FileZip) Close() error {
	if f.fp != nil {
		return f.fp.Close()
	}
	return nil
}

type MemZip struct {
	fp *bytes.Buffer
}

func NewMemZip() *MemZip {
	return &MemZip{
		fp: bytes.NewBuffer([]byte{}),
	}
}

func (f MemZip) Reader() (*zip.Reader, error) {
	return zip.NewReader(bytes.NewReader(f.fp.Bytes()), int64(f.fp.Len()))
}

func (f MemZip) Writer() (*zip.Writer, error) {
	return zip.NewWriter(f.fp), nil
}

func (f MemZip) Close() error {
	return nil
}
