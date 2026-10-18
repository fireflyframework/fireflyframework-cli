package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
)

func TestReleaseTagsOnlyThePublishedMainCommit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("GitHub CLI probe uses a POSIX executable")
	}
	for _, ahead := range []bool{false, true} {
		name := "published-main"
		if ahead {
			name = "unpushed-local-main"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("FLYWORK_HOME", "")
			repos := t.TempDir()
			repo := filepath.Join(repos, "fireflyframework-jafly")
			remote := filepath.Join(t.TempDir(), "origin.git")
			run := func(dir string, args ...string) string {
				t.Helper()
				command := exec.Command("git", args...)
				command.Dir = dir
				out, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %s %v", args, out, err)
				}
				return strings.TrimSpace(string(out))
			}
			run(repos, "init", "--bare", "--initial-branch=main", remote)
			run(repos, "clone", remote, repo)
			run(repo, "config", "user.name", "CLI Test")
			run(repo, "config", "user.email", "cli-test@example.invalid")
			parent := filepath.Join(repo, "build", "parent", "pom.xml")
			if err := os.MkdirAll(filepath.Dir(parent), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(parent, []byte("<project><version>26.10.17</version></project>"), 0644); err != nil {
				t.Fatal(err)
			}
			run(repo, "add", ".")
			run(repo, "commit", "-m", "Remote release train")
			run(repo, "push", "-u", "origin", "main")
			published := run(repo, "rev-parse", "origin/main")
			if ahead {
				if err := os.WriteFile(filepath.Join(repo, "local-change"), []byte("unpublished"), 0644); err != nil {
					t.Fatal(err)
				}
				run(repo, "add", ".")
				run(repo, "commit", "-m", "Unpushed local change")
			}
			local := run(repo, "rev-parse", "HEAD")
			cfg := config.DefaultConfig()
			cfg.ReposPath = repos
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			old := releaseDryRun
			releaseDryRun = false
			t.Cleanup(func() { releaseDryRun = old })

			err := mergeAndTagRepo(ui.NewPrinter(), "fireflyframework-jafly", "v26.10.17")
			if ahead {
				if err == nil || !strings.Contains(err.Error(), "origin/main") {
					t.Errorf("unpushed local main accepted: %v", err)
				}
				command := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/tags/v26.10.17")
				command.Dir = remote
				if err := command.Run(); err == nil {
					t.Error("unpushed local commit was published as a release tag")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if got := run(remote, "rev-parse", "refs/tags/v26.10.17^{commit}"); got != published {
					t.Fatalf("release tag = %s, published main = %s", got, published)
				}
			}
			if got := run(repo, "rev-parse", "HEAD"); got != local {
				t.Fatalf("local work was reset from %s to %s", local, got)
			}
		})
	}
}
