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
	"os"
	"path/filepath"

	"github.com/fireflyframework/fireflyframework-cli/internal/dag"
	"github.com/fireflyframework/fireflyframework-cli/internal/git"
)

// FrameworkRepos lists all fireflyframework repos in dependency order (flat fallback).
var FrameworkRepos = []string{
	"fireflyframework-parent",
	"fireflyframework-bom",
	"fireflyframework-utils",
	"fireflyframework-validators",
	"fireflyframework-plugins",
	"fireflyframework-observability",
	"fireflyframework-cache",
	"fireflyframework-r2dbc",
	"fireflyframework-eda",
	"fireflyframework-cqrs",
	"fireflyframework-eventsourcing",
	"fireflyframework-transactional-engine",
	"fireflyframework-client",
	"fireflyframework-web",
	"fireflyframework-core",
	"fireflyframework-domain",
	"fireflyframework-data",
	"fireflyframework-workflow",
	"fireflyframework-ecm",
	"fireflyframework-ecm-esignature-adobe-sign",
	"fireflyframework-ecm-esignature-docusign",
	"fireflyframework-ecm-esignature-logalty",
	"fireflyframework-ecm-storage-aws",
	"fireflyframework-ecm-storage-azure",
	"fireflyframework-idp",
	"fireflyframework-idp-aws-cognito",
	"fireflyframework-idp-internal-db",
	"fireflyframework-idp-keycloak",
	"fireflyframework-notifications",
	"fireflyframework-notifications-firebase",
	"fireflyframework-notifications-resend",
	"fireflyframework-notifications-sendgrid",
	"fireflyframework-notifications-twilio",
	"fireflyframework-rule-engine",
	"fireflyframework-webhooks",
	"fireflyframework-callbacks",
	"fireflyframework-config-server",
	"fireflyframework-application",
	"fireflyframework-backoffice",
}

// CloneResult holds the result of a clone operation for a single repo.
type CloneResult struct {
	Repo    string
	Skipped bool
	Error   error
}

// CloneCallback is invoked after each repo clone with progress info.
type CloneCallback func(layer int, repo string, index int, total int, result CloneResult)

// FetchResult holds the result of a git pull/fetch for a single repo.
type FetchResult struct {
	Repo  string
	Error error
}

// FetchCallback is invoked after each repo fetch.
type FetchCallback func(repo string, index int, total int, result FetchResult)

// CloneAll clones all framework repos into reposDir (flat order, no callback).
func CloneAll(org, reposDir, branch string) []CloneResult {
	results := make([]CloneResult, 0, len(FrameworkRepos))

	for _, repo := range FrameworkRepos {
		target := filepath.Join(reposDir, repo)
		if _, err := os.Stat(target); err == nil {
			results = append(results, CloneResult{Repo: repo, Skipped: true})
			continue
		}

		url := git.RepoURL(org, repo)
		err := git.CloneQuiet(url, target, branch)
		results = append(results, CloneResult{Repo: repo, Error: err})
	}

	return results
}

// CloneAllDAG clones repos in DAG layer order, tracking state in the manifest.
// If manifest is nil, it behaves like the original (no persistence).
func CloneAllDAG(org, reposDir, branch string, manifest *Manifest, cb CloneCallback) ([]CloneResult, [][]string, error) {
	g := dag.FrameworkGraph()
	layers, err := g.Layers()
	if err != nil {
		return nil, nil, err
	}

	total := g.NodeCount()
	results := make([]CloneResult, 0, total)
	idx := 0

	for layerIdx, layer := range layers {
		for _, repo := range layer {
			idx++
			target := filepath.Join(reposDir, repo)

			// Skip repos that are already cloned successfully in manifest
			if manifest != nil {
				rs := manifest.Repo(repo)
				if rs.CloneStatus == StatusSuccess || rs.CloneStatus == StatusSkipped {
					r := CloneResult{Repo: repo, Skipped: true}
					results = append(results, r)
					if cb != nil {
						cb(layerIdx, repo, idx, total, r)
					}
					continue
				}
			}

			var r CloneResult
			if _, serr := os.Stat(target); serr == nil {
				r = CloneResult{Repo: repo, Skipped: true}
				if manifest != nil {
					manifest.MarkCloneSkipped(repo)
					if sha, shaErr := git.HeadCommit(target); shaErr == nil {
						manifest.Repo(repo).CommitSHA = sha
					}
				}
			} else {
				url := git.RepoURL(org, repo)
				cloneErr := git.CloneQuiet(url, target, branch)
				r = CloneResult{Repo: repo, Error: cloneErr}
				if manifest != nil {
					manifest.MarkClone(repo, cloneErr)
					if cloneErr == nil {
						if sha, shaErr := git.HeadCommit(target); shaErr == nil {
							manifest.Repo(repo).CommitSHA = sha
						}
					}
				}
			}

			results = append(results, r)
			if manifest != nil {
				_ = manifest.Save()
			}
			if cb != nil {
				cb(layerIdx, repo, idx, total, r)
			}
		}
	}

	return results, layers, nil
}

// FetchUpdates runs git pull on each already-cloned repo in the given list.
func FetchUpdates(reposDir string, repos []string, cb FetchCallback) []FetchResult {
	results := make([]FetchResult, 0, len(repos))

	for i, repo := range repos {
		repoDir := filepath.Join(reposDir, repo)
		var r FetchResult
		r.Repo = repo

		if _, err := os.Stat(repoDir); os.IsNotExist(err) {
			// Not cloned — skip silently
			results = append(results, r)
			if cb != nil {
				cb(repo, i+1, len(repos), r)
			}
			continue
		}

		r.Error = git.Pull(repoDir)
		results = append(results, r)
		if cb != nil {
			cb(repo, i+1, len(repos), r)
		}
	}

	return results
}
