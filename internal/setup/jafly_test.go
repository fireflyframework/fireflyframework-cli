package setup

import (
	"os"
	"path/filepath"
	"testing"
)

// Catch legacy setup successes suppressing monorepo installation or polluting retries.
func TestJaflySetupDoesNotResumeLegacyManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FLYWORK_HOME", "")
	old := filepath.Join(home, ".flywork", "setup-manifest.json")
	if err := os.MkdirAll(filepath.Dir(old), 0755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"repos":{"fireflyframework-parent":{"clone_status":"success","install_status":"success"}}}`)
	if err := os.WriteFile(old, data, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadManifest(DefaultManifestPath())
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("monorepo setup resumed a legacy multi-repository manifest")
	}
	b, err := os.ReadFile(old)
	if err != nil || string(b) != string(data) {
		t.Fatal("legacy setup state changed")
	}
}
