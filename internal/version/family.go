// Copyright 2024-2026 Firefly Software Solutions Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package version

import (
	"os"
	"path/filepath"
	"time"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"gopkg.in/yaml.v3"
)

const familyFileName = "version-families.yaml"

// VersionFamily records a single release version and the repos/commits it covers.
type VersionFamily struct {
	Version    string            `yaml:"version"`
	ReleasedAt time.Time         `yaml:"released_at"`
	Notes      string            `yaml:"notes,omitempty"`
	Modules    map[string]string `yaml:"modules"` // repo name → commit SHA
}

// VersionFamilyFile is the on-disk container for all recorded version families.
type VersionFamilyFile struct {
	Families []VersionFamily `yaml:"families"`
}

func familyFilePath() string {
	return filepath.Join(config.FlyworkHome(), familyFileName)
}

// LoadFamilies reads the version families file. Returns an empty file if it doesn't exist.
func LoadFamilies() (*VersionFamilyFile, error) {
	path := familyFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &VersionFamilyFile{}, nil
		}
		return nil, err
	}

	var f VersionFamilyFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Save writes the version families file to disk.
func (f *VersionFamilyFile) Save() error {
	dir := config.FlyworkHome()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}

	return os.WriteFile(familyFilePath(), data, 0644)
}

// Record adds or updates a version family entry with the given module SHAs.
func (f *VersionFamilyFile) Record(ver string, modules map[string]string) {
	// Update existing entry if version already recorded
	for i, fam := range f.Families {
		if fam.Version == ver {
			f.Families[i].ReleasedAt = time.Now()
			f.Families[i].Modules = modules
			return
		}
	}

	// Append new entry
	f.Families = append(f.Families, VersionFamily{
		Version:    ver,
		ReleasedAt: time.Now(),
		Modules:    modules,
	})
}

// Latest returns the most recently recorded family, or nil if empty.
func (f *VersionFamilyFile) Latest() *VersionFamily {
	if len(f.Families) == 0 {
		return nil
	}
	return &f.Families[len(f.Families)-1]
}
