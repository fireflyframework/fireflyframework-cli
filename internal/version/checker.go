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

	"github.com/fireflyframework/fireflyframework-cli/internal/dag"
	"github.com/fireflyframework/fireflyframework-cli/internal/git"
)

// RepoStatus holds the version status for a single repo.
type RepoStatus struct {
	Repo       string
	PomVersion string
	GitTag     string
	Dirty      bool
	Exists     bool
	HasPom     bool
	Error      string
}

// VersionReport summarises version consistency across all repos.
type VersionReport struct {
	Repos          []RepoStatus
	UniqueVersions map[string]int // version string → count
	Consistent     bool
	TotalRepos     int
	TotalWithPom   int
}

// CheckAll scans all repos and returns a version consistency report.
func CheckAll(reposDir string) (*VersionReport, error) {
	g := dag.FrameworkGraph()
	order, err := g.FlatOrder()
	if err != nil {
		return nil, err
	}

	report := &VersionReport{
		Repos:          make([]RepoStatus, 0, len(order)),
		UniqueVersions: make(map[string]int),
		TotalRepos:     len(order),
	}

	for _, repo := range order {
		rs := checkRepo(reposDir, repo)
		report.Repos = append(report.Repos, rs)

		if rs.HasPom && rs.PomVersion != "" {
			report.UniqueVersions[rs.PomVersion]++
			report.TotalWithPom++
		}
	}

	report.Consistent = len(report.UniqueVersions) <= 1

	return report, nil
}

func checkRepo(reposDir, repo string) RepoStatus {
	rs := RepoStatus{Repo: repo}
	repoDir := filepath.Join(reposDir, repo)

	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		return rs
	}
	rs.Exists = true

	pomPath := filepath.Join(repoDir, "pom.xml")
	if _, err := os.Stat(pomPath); err == nil {
		rs.HasPom = true
		ver, err := ReadPomVersion(pomPath)
		if err != nil {
			rs.Error = err.Error()
		} else {
			rs.PomVersion = ver
		}
	}

	tag, err := git.LatestTag(repoDir)
	if err == nil {
		rs.GitTag = tag
	}

	dirty, err := git.IsDirty(repoDir)
	if err == nil {
		rs.Dirty = dirty
	}

	return rs
}
