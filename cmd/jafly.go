package cmd

import (
	"path/filepath"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/version"
)

// Keep subsequent scaffolds aligned with the reactor that setup/update installed.
func syncJaflyParentVersion(cfg *config.Config) error {
	ver, err := version.ReadPomVersion(filepath.Join(cfg.ReposPath, "fireflyframework-jafly", "build", "parent", "pom.xml"))
	if err != nil {
		return err
	}
	cfg.ParentVersion = ver
	return cfg.Save()
}
