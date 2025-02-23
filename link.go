package main

import (
	"log/slog"
	"os"
)

type LinkCommand struct {
	Url string `long:"url"`
}

func (cmd *LinkCommand) Execute(args []string) (err error) {
	init_log()
	for _, v := range args {
		rd, err := os.Open(v)
		if err != nil {
			slog.Error("open", "name", v, "error", err)
			return err
		}
		defer rd.Close()
		err = LinkRelative(cmd.Url, rd, os.Stdout)
		if err != nil {
			slog.Error("convert", "name", v, "error", err)
			return err
		}
	}
	return nil
}
