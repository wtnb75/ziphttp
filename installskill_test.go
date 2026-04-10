package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSkillHelp(t *testing.T) {
	stdout, _ := runcmd_test(t, []string{"ziphttp", "install-skill", "--help"}, 0)
	if !strings.Contains(stdout, "--name") {
		t.Error("help does not contain --name")
	}
	if !strings.Contains(stdout, "--dest-base") {
		t.Error("help does not contain --dest-base")
	}
}

func TestInstallSkillExecute(t *testing.T) {
	tmp := t.TempDir()
	destBase := filepath.Join(tmp, "skills")
	cmd := InstallSkillCmd{
		Name:     "ziphttp",
		DestBase: destBase,
	}
	if err := cmd.Execute(nil); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(destBase, "ziphttp", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != embeddedSkillMD {
		t.Error("copied content mismatch")
	}
}

func TestInstallSkillNoForce(t *testing.T) {
	tmp := t.TempDir()
	destBase := filepath.Join(tmp, "skills")
	dest := filepath.Join(destBase, "ziphttp")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := InstallSkillCmd{
		Name:     "ziphttp",
		DestBase: destBase,
		Force:    false,
	}
	if err := cmd.Execute(nil); err == nil {
		t.Fatal("expected error when target exists")
	}
}

func TestExpandHome(t *testing.T) {
	res, err := expandHome("~/abc")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res, "~/") {
		t.Error("home is not expanded")
	}
}
