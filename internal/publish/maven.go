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

package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fireflyframework/fireflyframework-cli/internal/build"
	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/dag"
	"github.com/fireflyframework/fireflyframework-cli/internal/git"
	"github.com/fireflyframework/fireflyframework-cli/internal/maven"
)

// PublishOptions configures a DAG-aware publish run.
type PublishOptions struct {
	ReposDir    string
	JavaHome    string
	GithubOrg   string
	SkipTests   bool
	ForceAll    bool     // Publish all repos regardless of changes
	TargetRepos []string // Publish specific repos only
	DryRun      bool     // Show plan without publishing
}

// PublishResult holds the outcome of publishing a single repository.
type PublishResult struct {
	Repo    string
	Skipped bool
	Error   error
	LogFile string
}

// PublishStartCallback is invoked before each repo publish begins.
type PublishStartCallback func(layer int, repo string, index int, total int)

// PublishDoneCallback is invoked after each repo publish completes.
type PublishDoneCallback func(layer int, repo string, index int, total int, result PublishResult)

// DeployRepo returns the Maven altDeploymentRepository value for a given repo.
func DeployRepo(githubOrg, repoName string) string {
	return fmt.Sprintf("github::https://maven.pkg.github.com/%s/%s", githubOrg, repoName)
}

// PublishAllDAG publishes all Maven repos in DAG order with change detection.
func PublishAllDAG(opts PublishOptions, onStart PublishStartCallback, onDone PublishDoneCallback) ([]PublishResult, [][]string, error) {
	g := dag.FrameworkGraph()

	manifest, err := build.LoadManifest(build.DefaultManifestPath())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load build manifest: %w", err)
	}
	if manifest == nil {
		manifest = build.NewManifest()
	}

	// Determine which repos need publishing
	var publishSet map[string]bool

	if opts.ForceAll {
		publishSet = make(map[string]bool)
		for _, n := range g.Nodes() {
			publishSet[n] = true
		}
	} else {
		changed := build.DetectChanges(g, opts.ReposDir, manifest)
		publishSet = build.TransitiveClosure(g, changed)
	}

	// Scope to targeted repos if specified
	if len(opts.TargetRepos) > 0 {
		targeted := make(map[string]bool)
		for _, repo := range opts.TargetRepos {
			if !g.HasNode(repo) {
				return nil, nil, fmt.Errorf("unknown repository: %s", repo)
			}
			targeted[repo] = true
		}

		if !opts.ForceAll {
			for repo := range targeted {
				if !publishSet[repo] {
					delete(targeted, repo)
				}
			}
		}
		publishSet = targeted
	}

	sub := g.Subgraph(publishSet)
	layers, err := sub.Layers()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compute publish layers: %w", err)
	}

	total := 0
	for _, layer := range layers {
		total += len(layer)
	}

	if opts.DryRun {
		results := make([]PublishResult, 0, total)
		for _, layer := range layers {
			for _, repo := range layer {
				results = append(results, PublishResult{Repo: repo})
			}
		}
		return results, layers, nil
	}

	results := make([]PublishResult, 0, total)
	idx := 0

	for layerIdx, layer := range layers {
		for _, repo := range layer {
			idx++
			dir := filepath.Join(opts.ReposDir, repo)

			if onStart != nil {
				onStart(layerIdx, repo, idx, total)
			}

			// Skip repos without pom.xml
			pomPath := filepath.Join(dir, "pom.xml")
			if _, serr := os.Stat(pomPath); os.IsNotExist(serr) {
				r := PublishResult{Repo: repo, Skipped: true}
				results = append(results, r)
				if onDone != nil {
					onDone(layerIdx, repo, idx, total, r)
				}
				continue
			}

			deployTarget := DeployRepo(opts.GithubOrg, repo)
			sha, _ := git.HeadSHA(dir)

			output, deployErr := maven.DeployQuietOutput(dir, opts.JavaHome, opts.SkipTests, deployTarget)

			var logFile string
			if deployErr != nil && len(output) > 0 {
				logFile = writePublishLog(repo, output)
			}

			if deployErr == nil {
				manifest.MarkSuccess(repo, sha)
			} else {
				manifest.MarkFailed(repo, sha, deployErr)
			}
			_ = manifest.Save()

			r := PublishResult{Repo: repo, Error: deployErr, LogFile: logFile}
			results = append(results, r)
			if onDone != nil {
				onDone(layerIdx, repo, idx, total, r)
			}
		}
	}

	return results, layers, nil
}

// writePublishLog writes deploy output to ~/.flywork/logs/<repo>-publish.log.
func writePublishLog(repo string, output []byte) string {
	logsDir := filepath.Join(config.FlyworkHome(), "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return ""
	}
	logFile := filepath.Join(logsDir, repo+"-publish.log")

	header := fmt.Sprintf("=== Publish log for %s ===\n=== %s ===\n\n", repo, time.Now().Format(time.RFC3339))
	content := append([]byte(header), output...)

	if err := os.WriteFile(logFile, content, 0644); err != nil {
		return ""
	}
	return logFile
}
