package main

import (
	_ "embed"
	"os"

	"log/slog"

	"github.com/jessevdk/go-flags"
)

var globalOption struct {
	Verbose bool `short:"v" long:"verbose" description:"show verbose logs"`
}

func main() {
	var err error
	var webserv webserver
	var ziplist ZipList
	var ziptogzip ZiptoGzip
	var zopflicmd ZopfliZip
	parser := flags.NewParser(&globalOption, flags.Default)
	_, err = parser.AddCommand("webserver", "boot webserver", "boot zipweb", &webserv)
	if err != nil {
		slog.Error("addcommand webserver", "error", err)
		panic(err)
	}
	_, err = parser.AddCommand("ziplist", "zip list deflate", "zip list deflate", &ziplist)
	if err != nil {
		slog.Error("addcommand ziplist", "error", err)
		panic(err)
	}
	_, err = parser.AddCommand("ziptogzip", "zip list deflate", "zip list deflate", &ziptogzip)
	if err != nil {
		slog.Error("addcommand ziptogzip", "error", err)
		panic(err)
	}
	_, err = parser.AddCommand("zopflizip", "zopfli zip", "zopfli zip archiver", &zopflicmd)
	if err != nil {
		slog.Error("addcommand zopflizip", "error", err)
		panic(err)
	}
	if _, err := parser.Parse(); err != nil {
		if fe, ok := err.(*flags.Error); ok && fe.Type == flags.ErrHelp {
			os.Exit(0)
		}
		slog.Error("error exit", "error", err)
		os.Exit(1)
	}
}
