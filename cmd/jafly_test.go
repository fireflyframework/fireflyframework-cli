package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
)

// Catch new projects using an old parent after installing a newer reactor.
func TestSetupVersionTracksInstalledJaflyParent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLYWORK_HOME", "")
	root := t.TempDir()
	p := filepath.Join(root, "fireflyframework-jafly", "build", "parent", "pom.xml")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("<project><version>26.10.17-SNAPSHOT</version></project>"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.ReposPath = root
	if err := syncJaflyParentVersion(cfg); err != nil {
		t.Fatal(err)
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.ParentVersion != "26.10.17-SNAPSHOT" {
		t.Fatalf("project parent remains %s", saved.ParentVersion)
	}
}
