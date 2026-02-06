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
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/dag"
	"github.com/fireflyframework/fireflyframework-cli/internal/maven"
)

// InstallResult holds the result of a maven install for a single repo.
type InstallResult struct {
	Repo    string
	Skipped bool
	Error   error
	LogFile string // path to build log (populated on failure)
}

// InstallStartCallback is invoked before each repo install begins.
type InstallStartCallback func(layer int, repo string, index int, total int)

// InstallDoneCallback is invoked after each repo install completes.
type InstallDoneCallback func(layer int, repo string, index int, total int, result InstallResult)

// InstallAll runs mvn clean install on each repo in flat order.
func InstallAll(reposDir string, skipTests bool) []InstallResult {
	results := make([]InstallResult, 0, len(FrameworkRepos))

	for _, repo := range FrameworkRepos {
		dir := filepath.Join(reposDir, repo)
		err := maven.InstallQuiet(dir, skipTests)
		results = append(results, InstallResult{Repo: repo, Error: err})
	}

	return results
}

// InstallAllDAG installs repos in DAG layer order, tracking state in the manifest.
// If reposFilter is non-nil, only repos in that set are built (others are skipped).
// If manifest is nil, no state is persisted.
func InstallAllDAG(reposDir, javaHome string, skipTests bool, manifest *Manifest, reposFilter map[string]bool, onStart InstallStartCallback, onDone InstallDoneCallback) ([]InstallResult, [][]string, error) {
	g := dag.FrameworkGraph()
	layers, err := g.Layers()
	if err != nil {
		return nil, nil, err
	}

	total := g.NodeCount()
	results := make([]InstallResult, 0, total)
	idx := 0

	for layerIdx, layer := range layers {
		for _, repo := range layer {
			idx++
			dir := filepath.Join(reposDir, repo)

			// If we have a filter, skip repos not in the set
			if reposFilter != nil && !reposFilter[repo] {
				r := InstallResult{Repo: repo, Skipped: true}
				results = append(results, r)
				if onDone != nil {
					onDone(layerIdx, repo, idx, total, r)
				}
				continue
			}

			// If manifest shows this repo already succeeded, skip it
			if manifest != nil && reposFilter == nil {
				rs := manifest.Repo(repo)
				if rs.InstallStatus == StatusSuccess {
					r := InstallResult{Repo: repo, Skipped: true}
					results = append(results, r)
					if onDone != nil {
						onDone(layerIdx, repo, idx, total, r)
					}
					continue
				}
			}

			if onStart != nil {
				onStart(layerIdx, repo, idx, total)
			}

			// Skip repos that have no pom.xml (empty or uninitialized)
			var installErr error
			var buildOutput []byte
			pomPath := filepath.Join(dir, "pom.xml")
			if _, serr := os.Stat(pomPath); os.IsNotExist(serr) {
				// no pom.xml — skip silently
				if manifest != nil {
					manifest.MarkInstallSkipped(repo)
				}
			} else if javaHome != "" {
				buildOutput, installErr = maven.InstallQuietWithJavaOutput(dir, javaHome, skipTests)
			} else {
				buildOutput, installErr = maven.InstallQuietOutput(dir, skipTests)
			}

			if manifest != nil && installErr != nil {
				manifest.MarkInstall(repo, installErr)
			} else if manifest != nil {
				manifest.MarkInstall(repo, nil)
			}

			// Write build log on failure
			var logFile string
			if installErr != nil && len(buildOutput) > 0 {
				logFile = writeBuildLog(repo, buildOutput)
			}

			r := InstallResult{Repo: repo, Error: installErr, LogFile: logFile}
			results = append(results, r)
			if manifest != nil {
				_ = manifest.Save()
			}
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

// writeBuildLog writes Maven output to ~/.flywork/logs/<repo>.log and returns
// the log file path. Returns "" if writing fails.
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
