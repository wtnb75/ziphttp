package main

import (
	"log/slog"
	"os"

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
	var level = slog.LevelInfo
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

type SubCommand struct {
	Name  string
	Short string
	Long  string
	Data  interface{}
}

func realMain() int {
	var err error
	commands := []SubCommand{
		{Name: "webserver", Short: "boot webserver", Long: "boot zipweb", Data: &WebServer{}},
		{Name: "ziplist", Short: "list zip names", Long: "list zip names", Data: &ZipList{}},
		{Name: "zip2gzip", Short: "extract from zip", Long: "extract files from zip without decompress", Data: &ZiptoGzip{}},
		{Name: "testlink", Short: "test link rewrite", Long: "test rewrite link to relative", Data: &LinkCommand{}},
		{Name: "zipsort", Short: "sort zip", Long: "sort zip by name", Data: &ZipSort{}},
		{Name: "zip", Short: "create zip", Long: "create new archive from dir/file/zip", Data: &ZipCmd{}},
		{Name: "version", Short: "show version", Long: "show version and exit", Data: &VersionCmd{}},
	}
	parser := flags.NewParser(&globalOption, flags.Default)
	for _, cmd := range commands {
		_, err = parser.AddCommand(cmd.Name, cmd.Short, cmd.Long, cmd.Data)
		if err != nil {
			slog.Error(cmd.Name, "error", err)
			return -1
		}
	}
	if _, err := parser.Parse(); err != nil {
		if _, ok := err.(*flags.Error); ok {
			return 0
		}
		slog.Error("error exit", "error", err)
		parser.WriteHelp(os.Stdout)
		return 1
	}
	return 0
}

func main() {
	os.Exit(realMain())
}
