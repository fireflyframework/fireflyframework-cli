package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/spf13/cobra"
)

func TestUpdateStopsBeforeInstallWhenPullFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Maven probe uses a POSIX executable")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLYWORK_HOME", "")
	root := t.TempDir()
	repo := filepath.Join(root, "fireflyframework-jafly")
	if out, err := exec.Command("git", "init", "-b", "main", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", out, err)
	}
	parent := filepath.Join(repo, "build", "parent", "pom.xml")
	if err := os.MkdirAll(filepath.Dir(parent), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parent, []byte("<project><version>26.10.17-SNAPSHOT</version></project>"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.ReposPath = root
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	marker := filepath.Join(root, "maven-ran")
	t.Setenv("FLYWORK_TEST_MAVEN_MARKER", marker)
	if err := os.WriteFile(filepath.Join(bin, "mvn"), []byte("#!/bin/sh\n: > \"$FLYWORK_TEST_MAVEN_MARKER\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	oldPullOnly, oldRepo, oldSkipTests := updatePullOnly, updateRepo, updateSkipTests
	updatePullOnly, updateRepo, updateSkipTests = false, "", true
	t.Cleanup(func() { updatePullOnly, updateRepo, updateSkipTests = oldPullOnly, oldRepo, oldSkipTests })
	command := &cobra.Command{}
	command.Flags().Bool("skip-tests", false, "")
	if err := command.Flags().Set("skip-tests", "true"); err != nil {
		t.Fatal(err)
	}

	if err := runUpdate(command, nil); err == nil || !strings.Contains(err.Error(), "failed to pull") {
		t.Errorf("update did not report failed pull: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("Maven ran after failed pull: %v", err)
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.ParentVersion != cfg.ParentVersion {
		t.Errorf("failed update changed configured parent from %s to %s", cfg.ParentVersion, saved.ParentVersion)
	}
}
