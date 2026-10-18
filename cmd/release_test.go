package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
)

// Catch release --dry-run creating a branch in a user's checkout.
func TestReleaseDryRunDoesNotChangeBranch(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "fireflyframework-jafly")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-b", "main", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", out, err)
	}
	before, err := os.ReadFile(filepath.Join(repo, ".git", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	old := releaseDryRun
	releaseDryRun = true
	defer func() { releaseDryRun = old }()
	if err := processRepoForRelease(ui.NewPrinter(), root, "fireflyframework-jafly", "release/26.10.17", "26.10.17"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(repo, ".git", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("branch changed during dry-run: %s", strings.TrimSpace(string(after)))
	}
}

// Catch associating a previous successful workflow with a different release tag.
func TestReleaseStatusMatchesTagAndCommit(t *testing.T) {
	payload := []byte(`[{"headSha":"old","headBranch":"v26.10.17","status":"completed","conclusion":"success"},{"headSha":"current","headBranch":"main","status":"completed","conclusion":"success"},{"headSha":"current","headBranch":"v26.10.17","status":"completed","conclusion":"failure"}]`)
	got, err := releaseConclusion(payload, "v26.10.17", "current")
	if err != nil || got != "failure" {
		t.Fatalf("release conclusion = %q, %v", got, err)
	}
	got, err = releaseConclusion(payload, "v26.10.18", "current")
	if err != nil || got != "" {
		t.Fatalf("absent release conclusion = %q, %v", got, err)
	}
}

// Catch tag operations mutating a checkout before the dry-run guard.
func TestTagDryRunDoesNotChangeCheckout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLYWORK_HOME", "")
	old := releaseDryRun
	releaseDryRun = true
	defer func() { releaseDryRun = old }()
	if err := mergeAndTagRepo(ui.NewPrinter(), "fireflyframework-jafly", "v26.10.17"); err != nil {
		t.Fatalf("dry run attempted Git operations: %v", err)
	}
}

// Catch creating release metadata without first committing the requested train.
func TestPrepareJaflyReleaseUpdatesAndCommitsVersion(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "fireflyframework-jafly")
	if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	script := `import pathlib, sys
p = pathlib.Path(__file__).resolve().parents[1] / 'pom.xml'
p.write_text('<project><version>' + sys.argv[1] + '</version></project>')
`
	for p, content := range map[string]string{"pom.xml": "<project><version>26.10.17-SNAPSHOT</version></project>", "scripts/set_version.py": script} {
		if err := os.WriteFile(filepath.Join(repo, p), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "CLI Test"}, {"config", "user.email", "cli-test@example.invalid"}, {"add", "."}, {"commit", "-m", "Initial fixture"}} {
		command := exec.Command("git", args...)
		command.Dir = repo
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	if err := prepareJaflyReleaseVersion(root, "26.10.17"); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "status", "--porcelain")
	command.Dir = repo
	out, err := command.Output()
	if err != nil || len(out) != 0 {
		t.Fatalf("version bump left uncommitted files: %s %v", out, err)
	}
	b, err := os.ReadFile(filepath.Join(repo, "pom.xml"))
	if err != nil || !strings.Contains(string(b), "<version>26.10.17</version>") {
		t.Fatalf("version was not prepared: %s %v", b, err)
	}
}

// Catch resetting an existing release branch to the current checkout's HEAD.
func TestCheckoutReleaseBranchPreservesExistingCommit(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		b, e := c.CombinedOutput()
		if e != nil {
			t.Fatalf("git %v: %s %v", args, b, e)
		}
		return strings.TrimSpace(string(b))
	}
	run("init", "-b", "main")
	run("config", "user.name", "CLI Test")
	run("config", "user.email", "cli-test@example.invalid")
	p := filepath.Join(repo, "file")
	if err := os.WriteFile(p, []byte("first"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "file")
	run("commit", "-m", "First")
	first := run("rev-parse", "HEAD")
	run("branch", "release/26.10.17")
	if err := os.WriteFile(p, []byte("second"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "file")
	run("commit", "-m", "Second")
	if err := checkoutReleaseBranch(repo, "release/26.10.17"); err != nil {
		t.Fatal(err)
	}
	if got := run("rev-parse", "HEAD"); got != first {
		t.Fatalf("existing release branch reset: %s", got)
	}
}

// Catch releasing an initial snapshot as an unrelated fallback train.
func TestInitialJaflySnapshotReleasesItsOwnTrain(t *testing.T) {
	got, err := resolveJaflyReleaseTarget("26.10.17-SNAPSHOT")
	if err != nil || got.String() != "26.10.17" {
		t.Fatalf("release target = %s, %v", got.String(), err)
	}
	if _, err := resolveJaflyReleaseTarget("invalid"); err == nil {
		t.Fatal("invalid current train silently guessed")
	}
}
