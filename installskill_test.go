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

func TestInstallSkillInvalidNames(t *testing.T) {
	tests := []struct {
		name       string
		inputName  string
		errContain string
	}{
		{name: "empty", inputName: "", errContain: "name must not be empty"},
		{name: "whitespace only", inputName: " \t\n", errContain: "name must not be empty"},
		{name: "parent traversal", inputName: "../x", errContain: "name contains invalid characters"},
		{name: "path separator", inputName: "a/b", errContain: "name contains invalid characters"},
		{name: "regex reject", inputName: "bad name", errContain: "name contains invalid characters"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := InstallSkillCmd{
				Name:     tc.inputName,
				DestBase: filepath.Join(t.TempDir(), "skills"),
			}
			err := cmd.Execute(nil)
			if err == nil {
				t.Fatalf("expected error for name=%q", tc.inputName)
			}
			if !strings.Contains(err.Error(), tc.errContain) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
