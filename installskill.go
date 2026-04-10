package main

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

//go:embed .github/skills/ziphttp/SKILL.md
var embeddedSkillMD string

type InstallSkillCmd struct {
	Name     string `long:"name" description:"skill name" default:"ziphttp"`
	DestBase string `long:"dest-base" description:"destination skills base directory" default:"~/.copilot/skills"`
	Force    bool   `long:"force" description:"overwrite existing files"`
}

func expandHome(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func (cmd *InstallSkillCmd) Execute(args []string) error {
	init_log()

	destBase, err := expandHome(cmd.DestBase)
	if err != nil {
		slog.Error("expand destination", "error", err)
		return err
	}
	if cmd.Name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if embeddedSkillMD == "" {
		return fmt.Errorf("embedded skill content is empty")
	}

	dest := filepath.Join(destBase, cmd.Name)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		slog.Error("mkdir", "path", dest, "error", err)
		return err
	}
	skillPath := filepath.Join(dest, "SKILL.md")
	if !cmd.Force {
		if _, err := os.Stat(skillPath); err == nil {
			return fmt.Errorf("target file already exists: %s", skillPath)
		}
	}

	if err := os.WriteFile(skillPath, []byte(embeddedSkillMD), 0o644); err != nil {
		slog.Error("write skill", "destination", skillPath, "error", err)
		return err
	}
	fmt.Println("installed", cmd.Name, "to", dest)
	return nil
}
