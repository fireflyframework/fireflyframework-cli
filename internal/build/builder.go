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

package build

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/dag"
	"github.com/fireflyframework/fireflyframework-cli/internal/git"
	"github.com/fireflyframework/fireflyframework-cli/internal/maven"
)

// BuildOptions configures a DAG-aware build run.
type BuildOptions struct {
	ReposDir    string
	JavaHome    string
	SkipTests   bool
	ForceAll    bool     // Ignore change detection, rebuild everything
	TargetRepos []string // Build specific repos + their dependents
	DryRun      bool     // Show plan without building
}

// BuildResult holds the outcome of building a single repository.
type BuildResult struct {
	Repo    string
	Skipped bool
	Error   error
	LogFile string
}

// BuildStartCallback is invoked before each repo build begins.
type BuildStartCallback func(layer int, repo string, index int, total int)

// BuildDoneCallback is invoked after each repo build completes.
type BuildDoneCallback func(layer int, repo string, index int, total int, result BuildResult)

// RunDAGBuild executes a smart, DAG-aware build with change detection.
//
// Algorithm:
//  1. Load the build manifest for change comparison
//  2. Run DetectChanges to find repos with new commits
//  3. Unless ForceAll, compute TransitiveClosure to get full build set
//  4. If TargetRepos is set, scope to those repos + their transitive dependents
//  5. Walk layers in order, building each repo via maven install
//  6. Update manifest after each repo
//  7. Save build logs on failure
func RunDAGBuild(opts BuildOptions, onStart BuildStartCallback, onDone BuildDoneCallback) ([]BuildResult, [][]string, error) {
	g := dag.FrameworkGraph()

	manifest, err := LoadManifest(DefaultManifestPath())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load build manifest: %w", err)
	}
	if manifest == nil {
		manifest = NewManifest()
	}

	// Determine which repos need building
	var buildSet map[string]bool

	if opts.ForceAll {
		buildSet = make(map[string]bool)
		for _, n := range g.Nodes() {
			buildSet[n] = true
		}
	} else {
		changed := DetectChanges(g, opts.ReposDir, manifest)
		buildSet = TransitiveClosure(g, changed)
	}

	// If targeting specific repos, scope to those + transitive dependents
	if len(opts.TargetRepos) > 0 {
		targeted := make(map[string]bool)
		for _, repo := range opts.TargetRepos {
			if !g.HasNode(repo) {
				return nil, nil, fmt.Errorf("unknown repository: %s", repo)
			}
			targeted[repo] = true
			for _, dep := range g.TransitiveDependentsOf(repo) {
				targeted[dep] = true
			}
		}

		// Intersect with change-detected set (unless --all)
		if !opts.ForceAll {
			for repo := range targeted {
				if !buildSet[repo] {
					delete(targeted, repo)
				}
			}
		}
		buildSet = targeted
	}

	// Build a subgraph of only the affected repos to get proper layer ordering
	sub := g.Subgraph(buildSet)
	layers, err := sub.Layers()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compute build layers: %w", err)
	}

	// Count total repos to build
	total := 0
	for _, layer := range layers {
		total += len(layer)
	}

	if opts.DryRun {
		// Return results without actually building
		results := make([]BuildResult, 0, total)
		for _, layer := range layers {
			for _, repo := range layer {
				results = append(results, BuildResult{Repo: repo})
			}
		}
		return results, layers, nil
	}

	results := make([]BuildResult, 0, total)
	idx := 0

	for layerIdx, layer := range layers {
		for _, repo := range layer {
			idx++
			dir := filepath.Join(opts.ReposDir, repo)

			if onStart != nil {
				onStart(layerIdx, repo, idx, total)
			}

			// Skip repos that have no pom.xml
			var buildErr error
			var buildOutput []byte
			pomPath := filepath.Join(dir, "pom.xml")
			if _, serr := os.Stat(pomPath); os.IsNotExist(serr) {
				r := BuildResult{Repo: repo, Skipped: true}
				results = append(results, r)
				if onDone != nil {
					onDone(layerIdx, repo, idx, total, r)
				}
				continue
			}

			sha, _ := git.HeadSHA(dir)

			if opts.JavaHome != "" {
				buildOutput, buildErr = maven.InstallQuietWithJavaOutput(dir, opts.JavaHome, opts.SkipTests)
			} else {
				buildOutput, buildErr = maven.InstallQuietOutput(dir, opts.SkipTests)
			}

			if buildErr != nil {
				manifest.MarkFailed(repo, sha, buildErr)
			} else {
				manifest.MarkSuccess(repo, sha)
			}

			// Write build log on failure
			var logFile string
			if buildErr != nil && len(buildOutput) > 0 {
				logFile = writeBuildLog(repo, buildOutput)
			}

			r := BuildResult{Repo: repo, Error: buildErr, LogFile: logFile}
			results = append(results, r)
			_ = manifest.Save()

			if onDone != nil {
				onDone(layerIdx, repo, idx, total, r)
			}
		}
	}

	return results, layers, nil
}

// LogsDir returns the path to the build logs directory (~/.flywork/logs).
func LogsDir() string {
	return filepath.Join(config.FlyworkHome(), "logs")
}

// writeBuildLog writes Maven output to ~/.flywork/logs/<repo>.log.
func writeBuildLog(repo string, output []byte) string {
	logsDir := LogsDir()
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return ""
	}
	logFile := filepath.Join(logsDir, repo+".log")

	header := fmt.Sprintf("=== Build log for %s ===\n=== %s ===\n\n", repo, time.Now().Format(time.RFC3339))
	content := append([]byte(header), output...)

	if err := os.WriteFile(logFile, content, 0644); err != nil {
		return ""
	}
	return logFile
}
