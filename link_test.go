package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func TestLink(t *testing.T) {
	testdata := `
<html><head>
<link rel="something1" src="../path/relative.html" />
<link rel="something2" src="/path/from/root.html" />
</head>
<body>
<ul>
<li><a href="http://example.com/absolute/link/1.html">1</a></li>
<li><a href="http://other-site.example.com/absolute/link/2.html">2</a></li>
</ul>
</body></html>
`
	fp, err := os.CreateTemp(t.TempDir(), "*.html")
	if err != nil {
		t.Error("mktemp", err)
		return
	}
	defer os.Remove(fp.Name())
	defer fp.Close()
	fmt.Fprintln(fp, testdata)
	orgStdout := os.Stdout
	defer func() {
		os.Stdout = orgStdout
	}()
	r, w, err := os.Pipe()
	if err != nil {
		t.Error("pipe")
		return
	}
	os.Stdout = w

	cmd := LinkCommand{Url: "http://example.com/base/path/index.html"}
	err = cmd.Execute([]string{fp.Name()})
	w.Close()

	if err != nil {
		t.Error("result", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Error("read", err)
	}
	r.Close()
	sdata := string(data)
	if strings.Contains(sdata, "http://example.com") {
		t.Error("example.com", sdata)
	}
	if !strings.Contains(sdata, "\"../../path/from/root") {
		t.Error("from root", sdata)
	}
	if !strings.Contains(sdata, "\"../../absolute/link") {
		t.Error("from absolute", sdata)
	}
	if !strings.Contains(sdata, "http://other-site.example.com") {
		t.Error("other-site.example.com", sdata)
	}
}

func TestLinkNoHtml(t *testing.T) {
	testdata := `hello world src=http://example.com/blabla href="http://example.com"`
	fp, err := os.CreateTemp(t.TempDir(), "*.txt")
	if err != nil {
		t.Error("mktemp", err)
		return
	}
	defer os.Remove(fp.Name())
	defer fp.Close()
	fmt.Fprintln(fp, testdata)
	orgStdout := os.Stdout
	defer func() {
		os.Stdout = orgStdout
	}()
	r, w, err := os.Pipe()
	if err != nil {
		t.Error("pipe")
		return
	}
	os.Stdout = w

	cmd := LinkCommand{Url: "http://example.com/base/path/plaintext.txt"}
	err = cmd.Execute([]string{fp.Name()})
	w.Close()

	if err != nil {
		t.Error("result", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Error("read", err)
	}
	r.Close()
	sdata := string(data)
	if !strings.Contains(sdata, "http://example.com") {
		t.Error("example.com", sdata)
	}
}

func TestLinkNotFound(t *testing.T) {
	t.Parallel()
	fp, err := os.CreateTemp(t.TempDir(), "*.txt")
	if err != nil {
		t.Error("mktemp", err)
		return
	}
	name := fp.Name()
	if err = os.Remove(name); err != nil {
		t.Error("remove", err)
		return
	}
	cmd := LinkCommand{Url: "http://example.com/base/path/plaintext.txt"}
	err = cmd.Execute([]string{name})

	if err == nil {
		t.Error("is nil")
	}
}
