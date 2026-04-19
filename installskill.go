package main

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed .github/skills/ziphttp/SKILL.md
var embeddedSkillMD string

type InstallSkillCmd struct {
	Name     string `long:"name" description:"skill name" default:"ziphttp"`
	DestBase string `long:"dest-base" description:"destination skills base directory" default:"~/.copilot/skills"`
	Force    bool   `long:"force" description:"overwrite existing files"`
}

var skillNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

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
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if !skillNamePattern.MatchString(name) {
		return fmt.Errorf("name contains invalid characters: %s", name)
	}
	if embeddedSkillMD == "" {
		return fmt.Errorf("embedded skill content is empty")
	}

	dest := filepath.Join(destBase, name)
	rel, err := filepath.Rel(destBase, dest)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("destination escapes base directory")
	}
	if err := os.MkdirAll(dest, 0o750); err != nil {
		slog.Error("mkdir", "path", dest, "error", err)
		return err
	}
	skillPath := filepath.Join(dest, "SKILL.md")
	if !cmd.Force {
		if _, err := os.Stat(skillPath); err == nil {
			return fmt.Errorf("target file already exists: %s", skillPath)
		} else if !os.IsNotExist(err) {
			slog.Error("stat skill", "path", skillPath, "error", err)
			return err
		}
	}

	if err := os.WriteFile(skillPath, []byte(embeddedSkillMD), 0o600); err != nil {
		slog.Error("write skill", "destination", skillPath, "error", err)
		return err
	}
	fmt.Println("installed", name, "to", dest)
	return nil
}
