package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseTagResolutionUsesThePeeledCommit(t *testing.T) {
	commit := strings.Repeat("a", 40)
	annotation := strings.Repeat("b", 40)
	for _, annotated := range []bool{false, true} {
		calls := 0
		got, err := resolveReleaseTagCommit("example/jafly", "v26.10.17", func(endpoint string) ([]byte, error) {
			calls++
			switch endpoint {
			case "/repos/example/jafly/git/ref/tags/v26.10.17":
				if annotated {
					return []byte(`{"object":{"type":"tag","sha":"` + annotation + `"}}`), nil
				}
				return []byte(`{"object":{"type":"commit","sha":"` + commit + `"}}`), nil
			case "/repos/example/jafly/git/tags/" + annotation:
				return []byte(`{"object":{"type":"commit","sha":"` + commit + `"}}`), nil
			default:
				t.Fatalf("unexpected GitHub endpoint: %s", endpoint)
				return nil, nil
			}
		})
		if err != nil || got != commit {
			t.Fatalf("tag commit = %q, %v", got, err)
		}
		if (annotated && calls != 2) || (!annotated && calls != 1) {
			t.Fatalf("tag resolution used %d requests", calls)
		}
	}
}

func TestReleaseCommitEvidenceRejectsAnotherCommit(t *testing.T) {
	commit := strings.Repeat("a", 40)
	if err := validateReleaseCommit([]byte(commit+"\n"), commit); err != nil {
		t.Fatal(err)
	}
	for _, evidence := range []string{"", strings.Repeat("b", 40), commit + "\n" + strings.Repeat("b", 40)} {
		if err := validateReleaseCommit([]byte(evidence), commit); err == nil {
			t.Fatalf("unrelated release evidence accepted: %q", evidence)
		}
	}
}

func TestReleaseSourceCloneRejectsATagMovedToAnotherCommit(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = repo
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-b", "main")
	run("config", "user.name", "CLI Test")
	run("config", "user.email", "cli-test@example.invalid")
	file := filepath.Join(repo, "content")
	if err := os.WriteFile(file, []byte("released"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "Release fixture")
	commit := run("rev-parse", "HEAD")
	run("tag", "v26.10.17")
	if err := os.WriteFile(file, []byte("later development"), 0644); err != nil {
		t.Fatal(err)
	}
	run("commit", "-am", "Later fixture")
	later := run("rev-parse", "HEAD")
	clone := filepath.Join(t.TempDir(), "source")
	if err := cloneReleaseSource(repo, "v26.10.17", commit, clone); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(clone, "content"))
	if err != nil || string(content) != "released" {
		t.Fatalf("clone used development tree: %s %v", content, err)
	}
	if err := cloneReleaseSource(repo, "v26.10.17", later, filepath.Join(t.TempDir(), "source")); err == nil {
		t.Fatal("tag/expected commit mismatch accepted")
	}
	if got := run("rev-parse", "HEAD"); got != later {
		t.Fatalf("source checkout moved from %s to %s", later, got)
	}
}
