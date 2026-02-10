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

// Package build provides DAG-aware build orchestration with change detection.
// It tracks build state across invocations via a persistent manifest, detects
// which repositories have changed since the last successful build, and computes
// the transitive closure of affected downstream repositories.
package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
)

const (
	ManifestFile = "build-manifest.json"
	ManifestVer  = 1
)

// BuildManifest persists build state across invocations.
type BuildManifest struct {
	Version   int                    `json:"version"`
	UpdatedAt time.Time              `json:"updated_at"`
	Repos     map[string]*BuildState `json:"repos"`

	path string
}

// BuildState tracks the last build result for a single repository.
type BuildState struct {
	LastBuildSHA    string    `json:"last_build_sha"`
	LastBuildTime   time.Time `json:"last_build_time"`
	ArtifactVersion string    `json:"artifact_version,omitempty"`
	Status          string    `json:"status"` // pending, success, failed
	Error           string    `json:"error,omitempty"`
}

// DefaultManifestPath returns ~/.flywork/build-manifest.json.
func DefaultManifestPath() string {
	return filepath.Join(config.FlyworkHome(), ManifestFile)
}

// NewManifest creates a fresh empty build manifest.
func NewManifest() *BuildManifest {
	return &BuildManifest{
		Version:   ManifestVer,
		UpdatedAt: time.Now(),
		Repos:     make(map[string]*BuildState),
		path:      DefaultManifestPath(),
	}
}

// LoadManifest reads a build manifest from disk. Returns nil, nil if the file
// does not exist.
func LoadManifest(path string) (*BuildManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m BuildManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	m.path = path
	if m.Repos == nil {
		m.Repos = make(map[string]*BuildState)
	}
	return &m, nil
}

// Save writes the manifest to disk.
func (m *BuildManifest) Save() error {
	if m.path == "" {
		m.path = DefaultManifestPath()
	}
	m.UpdatedAt = time.Now()
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0644)
}

// SetPath overrides the file path for this manifest.
func (m *BuildManifest) SetPath(p string) {
	m.path = p
}

// LastSHA returns the last successfully built SHA for a repo, or "" if unknown.
func (m *BuildManifest) LastSHA(repo string) string {
	bs, ok := m.Repos[repo]
	if !ok {
		return ""
	}
	return bs.LastBuildSHA
}

// MarkSuccess records a successful build for a repo.
func (m *BuildManifest) MarkSuccess(repo, sha string) {
	bs := m.ensureState(repo)
	bs.LastBuildSHA = sha
	bs.LastBuildTime = time.Now()
	bs.Status = "success"
	bs.Error = ""
}

// MarkFailed records a failed build for a repo.
func (m *BuildManifest) MarkFailed(repo, sha string, buildErr error) {
	bs := m.ensureState(repo)
	bs.LastBuildSHA = sha
	bs.LastBuildTime = time.Now()
	bs.Status = "failed"
	if buildErr != nil {
		bs.Error = buildErr.Error()
	}
}

func (m *BuildManifest) ensureState(repo string) *BuildState {
	bs, ok := m.Repos[repo]
	if !ok {
		bs = &BuildState{Status: "pending"}
		m.Repos[repo] = bs
	}
	return bs
}
