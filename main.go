package main

import (
	"os"

	"log/slog"

	"github.com/jessevdk/go-flags"
)

var globalOption struct {
	Verbose bool           `short:"v" long:"verbose" description:"show verbose logs"`
	Quiet   bool           `short:"q" long:"quiet" description:"suppress logs"`
	JsonLog bool           `long:"json-log" description:"use json format for logging"`
	Archive flags.Filename `short:"f" long:"archive" description:"archive file" env:"ZIPHTTP_ARCHIVE"`
	Self    bool           `long:"self" description:"use executable zip" env:"ZIPHTTP_SELF"`
}

func archiveFilename() string {
	if globalOption.Self {
		res, err := os.Executable()
		if err != nil {
			slog.Error("self name", "error", err)
			panic(err)
		}
		return res
	}
	return string(globalOption.Archive)
}

func init_log() {
	var level slog.Level = slog.LevelInfo
	if globalOption.Verbose {
		level = slog.LevelDebug
	} else if globalOption.Quiet {
		level = slog.LevelWarn
	}
	slog.SetLogLoggerLevel(level)
	if globalOption.JsonLog {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	}
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
