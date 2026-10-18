package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLatestTagIgnoresPreservedSourceRepositoryTags(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-c", "user.name=CLI Test", "-c", "user.email=cli@example.invalid"}, args...)...)
		command.Dir = dir
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	run("init")
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "pom.xml")
	run("commit", "-m", "Framework release")
	run("tag", "v26.10.16")
	run("commit", "--allow-empty", "-m", "Import historical source")
	run("tag", "legacy/fireflyframework-cqrs/v26.07.06")
	got, err := LatestTag(dir)
	if err != nil || strings.TrimSpace(got) != "v26.10.16" {
		t.Fatalf("current framework tag = %q, %v; expected v26.10.16", got, err)
	}
}
