package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsolatedFlyworkHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLYWORK_HOME", dir)
	if got := FlyworkHome(); got != dir {
		t.Fatalf("FlyworkHome() = %q, want %q", got, dir)
	}
	cfg := DefaultConfig()
	if cfg.ReposPath != filepath.Join(dir, ReposDir) {
		t.Fatalf("default repositories escape configured home: %s", cfg.ReposPath)
	}
	cfg.ParentVersion = "26.10.17-SNAPSHOT"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ConfigFile)); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil || loaded.ParentVersion != cfg.ParentVersion {
		t.Fatalf("isolated configuration did not round-trip: %v, %v", loaded, err)
	}
}

func TestDefaultFlyworkHome(t *testing.T) {
	t.Setenv("FLYWORK_HOME", "")
	if got, want := FlyworkHome(), filepath.Join(HomeDir(), FireflyDir); got != want {
		t.Fatalf("FlyworkHome() = %q, want %q", got, want)
	}
}
