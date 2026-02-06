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

package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
)

// Status represents the state of a clone or install operation.
type Status string

const (
	StatusPending Status = "pending"
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"

	ManifestFile = "setup-manifest.json"
	ManifestVer  = 1
)

// RepoState tracks the clone and install state for a single repository.
type RepoState struct {
	CloneStatus   Status    `json:"clone_status"`
	InstallStatus Status    `json:"install_status"`
	CloneError    string    `json:"clone_error,omitempty"`
	InstallError  string    `json:"install_error,omitempty"`
	CommitSHA     string    `json:"commit_sha,omitempty"`
	LastAttempt   time.Time `json:"last_attempt"`
}

// Manifest is the top-level setup manifest persisted to disk.
type Manifest struct {
	Version     int                   `json:"version"`
	StartedAt   time.Time             `json:"started_at"`
	CompletedAt *time.Time            `json:"completed_at,omitempty"`
	JavaHome    string                `json:"java_home,omitempty"`
	SkipTests   bool                  `json:"skip_tests"`
	Repos       map[string]*RepoState `json:"repos"`

	path string // file path (not serialised)
}

// DefaultManifestPath returns ~/.flywork/setup-manifest.json.
func DefaultManifestPath() string {
	return filepath.Join(config.FlyworkHome(), ManifestFile)
}

// NewManifest creates a fresh manifest pre-populated with pending state for every repo.
func NewManifest(repos []string) *Manifest {
	m := &Manifest{
		Version:   ManifestVer,
		StartedAt: time.Now(),
		Repos:     make(map[string]*RepoState, len(repos)),
		path:      DefaultManifestPath(),
	}
	for _, r := range repos {
		m.Repos[r] = &RepoState{
			CloneStatus:   StatusPending,
			InstallStatus: StatusPending,
		}
	}
	return m
}

// LoadManifest reads a manifest from disk. Returns nil, nil if file does not exist.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	m.path = path
	return &m, nil
}

// Save writes the manifest to disk.
func (m *Manifest) Save() error {
	if m.path == "" {
		m.path = DefaultManifestPath()
	}
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
func (m *Manifest) SetPath(p string) {
	m.path = p
}

// Repo returns the state for a repo, creating it if absent.
func (m *Manifest) Repo(name string) *RepoState {
	rs, ok := m.Repos[name]
	if !ok {
		rs = &RepoState{CloneStatus: StatusPending, InstallStatus: StatusPending}
		m.Repos[name] = rs
	}
	return rs
}

// MarkClone records the clone result for a repo.
func (m *Manifest) MarkClone(repo string, err error) {
	rs := m.Repo(repo)
	rs.LastAttempt = time.Now()
	if err != nil {
		rs.CloneStatus = StatusFailed
		rs.CloneError = err.Error()
	} else {
		rs.CloneStatus = StatusSuccess
		rs.CloneError = ""
	}
}

// MarkCloneSkipped marks a repo as skipped (already cloned).
func (m *Manifest) MarkCloneSkipped(repo string) {
	rs := m.Repo(repo)
	rs.CloneStatus = StatusSkipped
	rs.LastAttempt = time.Now()
}

// MarkInstall records the install result for a repo.
func (m *Manifest) MarkInstall(repo string, err error) {
	rs := m.Repo(repo)
	rs.LastAttempt = time.Now()
	if err != nil {
		rs.InstallStatus = StatusFailed
		rs.InstallError = err.Error()
	} else {
		rs.InstallStatus = StatusSuccess
		rs.InstallError = ""
	}
}

// MarkInstallSkipped marks a repo install as skipped.
func (m *Manifest) MarkInstallSkipped(repo string) {
	rs := m.Repo(repo)
	rs.InstallStatus = StatusSkipped
	rs.LastAttempt = time.Now()
}

// MarkComplete marks the overall setup as complete.
func (m *Manifest) MarkComplete() {
	now := time.Now()
	m.CompletedAt = &now
}

// PendingClones returns repo names with clone_status == pending or failed.
func (m *Manifest) PendingClones() []string {
	var out []string
	for name, rs := range m.Repos {
		if rs.CloneStatus == StatusPending || rs.CloneStatus == StatusFailed {
			out = append(out, name)
		}
	}
	return out
}

// FailedClones returns repo names with clone_status == failed.
func (m *Manifest) FailedClones() []string {
	var out []string
	for name, rs := range m.Repos {
		if rs.CloneStatus == StatusFailed {
			out = append(out, name)
		}
	}
	return out
}

// PendingInstalls returns repo names with install_status == pending or failed.
func (m *Manifest) PendingInstalls() []string {
	var out []string
	for name, rs := range m.Repos {
		if rs.InstallStatus == StatusPending || rs.InstallStatus == StatusFailed {
			out = append(out, name)
		}
	}
	return out
}

// FailedInstalls returns repo names with install_status == failed.
func (m *Manifest) FailedInstalls() []string {
	var out []string
	for name, rs := range m.Repos {
		if rs.InstallStatus == StatusFailed {
			out = append(out, name)
		}
	}
	return out
}

// SuccessfulClones returns repo names with clone_status == success or skipped.
func (m *Manifest) SuccessfulClones() []string {
	var out []string
	for name, rs := range m.Repos {
		if rs.CloneStatus == StatusSuccess || rs.CloneStatus == StatusSkipped {
			out = append(out, name)
		}
	}
	return out
}

// SuccessfulInstalls returns repo names with install_status == success.
func (m *Manifest) SuccessfulInstalls() []string {
	var out []string
	for name, rs := range m.Repos {
		if rs.InstallStatus == StatusSuccess {
			out = append(out, name)
		}
	}
	return out
}

// IsComplete returns true if all repos have been cloned and installed successfully.
func (m *Manifest) IsComplete() bool {
	for _, rs := range m.Repos {
		if rs.CloneStatus != StatusSuccess && rs.CloneStatus != StatusSkipped {
			return false
		}
		if rs.InstallStatus != StatusSuccess && rs.InstallStatus != StatusSkipped {
			return false
		}
	}
	return m.CompletedAt != nil
}

// Summary returns human-readable counts.
type ManifestSummary struct {
	Total          int
	ClonesOK       int
	ClonesFailed   int
	ClonesPending  int
	InstallsOK     int
	InstallsFailed int
	InstallsPending int
}

func (m *Manifest) Summary() ManifestSummary {
	s := ManifestSummary{Total: len(m.Repos)}
	for _, rs := range m.Repos {
		switch rs.CloneStatus {
		case StatusSuccess, StatusSkipped:
			s.ClonesOK++
		case StatusFailed:
			s.ClonesFailed++
		default:
			s.ClonesPending++
		}
		switch rs.InstallStatus {
		case StatusSuccess, StatusSkipped:
			s.InstallsOK++
		case StatusFailed:
			s.InstallsFailed++
		default:
			s.InstallsPending++
		}
	}
	return s
}

// ResetFailed resets all failed clone/install statuses back to pending.
func (m *Manifest) ResetFailed() {
	for _, rs := range m.Repos {
		if rs.CloneStatus == StatusFailed {
			rs.CloneStatus = StatusPending
			rs.CloneError = ""
		}
		if rs.InstallStatus == StatusFailed {
			rs.InstallStatus = StatusPending
			rs.InstallError = ""
		}
	}
	m.CompletedAt = nil
}

// ResetAll resets every repo back to pending.
func (m *Manifest) ResetAll() {
	for _, rs := range m.Repos {
		rs.CloneStatus = StatusPending
		rs.CloneError = ""
		rs.InstallStatus = StatusPending
		rs.InstallError = ""
		rs.CommitSHA = ""
	}
	m.CompletedAt = nil
	m.StartedAt = time.Now()
}
