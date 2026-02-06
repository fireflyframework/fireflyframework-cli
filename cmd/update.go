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

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/dag"
	"github.com/fireflyframework/fireflyframework-cli/internal/git"
	"github.com/fireflyframework/fireflyframework-cli/internal/java"
	"github.com/fireflyframework/fireflyframework-cli/internal/maven"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
	"github.com/spf13/cobra"
)

var (
	updatePullOnly    bool
	updateRepo        string
	updateSkipTests   bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update framework repositories and reinstall to local Maven cache",
	Long:  "Pulls the latest changes for all cloned fireflyframework repos and reinstalls them to ~/.m2",
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&updatePullOnly, "pull-only", false, "Only git pull, skip maven install")
	updateCmd.Flags().StringVar(&updateRepo, "repo", "", "Update a single repository by name")
	updateCmd.Flags().BoolVar(&updateSkipTests, "skip-tests", false, "Skip running tests during Maven install")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	overallStart := time.Now()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !git.IsInstalled() {
		return fmt.Errorf("git is not installed")
	}
	if !updatePullOnly && !maven.IsInstalled() {
		return fmt.Errorf("maven is not installed")
	}

	// Resolve JAVA_HOME for configured version
	var javaHome string
	if !updatePullOnly {
		javaHome, err = java.DetectJavaHome(cfg.JavaVersion)
		if err != nil {
			p.Warning(fmt.Sprintf("Could not detect JAVA_HOME for Java %s — using system default", cfg.JavaVersion))
		}
	}

	// Resolve DAG order
	g := dag.FrameworkGraph()
	order, dagErr := g.FlatOrder()
	if dagErr != nil {
		return fmt.Errorf("dependency graph error: %w", dagErr)
	}
	layers, _ := g.Layers()

	// Determine repos to update
	repos := order
	if updateRepo != "" {
		found := false
		for _, r := range order {
			if r == updateRepo {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown repository %q", updateRepo)
		}
		repos = []string{updateRepo}
	}

	action := "Updating"
	if updatePullOnly {
		action = "Pulling"
	}
	// Determine whether to skip tests
	if !updatePullOnly && !cmd.Flags().Changed("skip-tests") {
		updateSkipTests = !ui.Confirm("Run tests during Maven install?", true)
	}

	p.Header(fmt.Sprintf("Framework %s", action))
	p.Info(fmt.Sprintf("Resolved dependency graph: %d repos, %d layers", len(order), len(layers)))
	if javaHome != "" && !updatePullOnly {
		p.Info(fmt.Sprintf("JAVA_HOME: %s", javaHome))
	}

	// ── Phase 1: Git pull ───────────────────────────────────────────────────
	p.StageHeader(1, "Pulling Latest Changes")

	pullBar := ui.NewProgressBar(len(repos), "pulled")
	pulled, pullSkipped, pullFailed := 0, 0, 0

	for _, repo := range repos {
		repoDir := filepath.Join(cfg.ReposPath, repo)
		if _, serr := os.Stat(repoDir); os.IsNotExist(serr) {
			pullSkipped++
			if verbose {
				p.Warning(fmt.Sprintf("%-45s not cloned (run 'flywork setup')", repo))
			}
			pullBar.Increment()
			continue
		}

		if pullErr := git.Pull(repoDir); pullErr != nil {
			pullFailed++
			p.Error(fmt.Sprintf("%-45s %s", repo, pullErr))
		} else {
			pulled++
			if verbose {
				p.Success(fmt.Sprintf("%-45s pulled", repo))
			}
		}
		pullBar.Increment()
	}

	pullBar.Finish()
	p.Newline()
	p.Info(fmt.Sprintf("Pull: %d pulled, %d skipped, %d failed", pulled, pullSkipped, pullFailed))

	if pullFailed > 0 && updatePullOnly {
		return fmt.Errorf("%d repositories failed to pull", pullFailed)
	}

	if !updatePullOnly {
		// ── Phase 2: Maven install ─────────────────────────────────────────────
		p.StageHeader(2, "Installing Artifacts")

		installBar := ui.NewProgressBar(len(repos), "installed")
		var activeSpinner *ui.Spinner
		installed, installFailed := 0, 0

		for i, repo := range repos {
			repoDir := filepath.Join(cfg.ReposPath, repo)
			if _, serr := os.Stat(repoDir); os.IsNotExist(serr) {
				installBar.Increment()
				continue
			}

			// Start spinner
			activeSpinner = ui.NewSpinner(fmt.Sprintf("Building %s...", repo))
			activeSpinner.Start()

		var installErr error
			if javaHome != "" {
				installErr = maven.InstallQuietWithJava(repoDir, javaHome, updateSkipTests)
			} else {
				installErr = maven.InstallQuiet(repoDir, updateSkipTests)
			}

			activeSpinner.Stop(installErr == nil)

			if installErr != nil {
				installFailed++
				p.Error(fmt.Sprintf("%-45s %s", repo, installErr))
			} else {
				installed++
			}

			installBar.Increment()
			_ = i
		}

		installBar.Finish()
		p.Newline()
		p.Info(fmt.Sprintf("Install: %d installed, %d failed", installed, installFailed))

		if installFailed > 0 {
			return fmt.Errorf("%d repositories failed to install", installFailed)
		}

		// ── Summary ─────────────────────────────────────────────────────────
		elapsed := time.Since(overallStart).Truncate(time.Second)
		p.SummaryBox("Update Complete", []string{
			fmt.Sprintf("Pulled        %d", pulled),
			fmt.Sprintf("Installed     %d", installed),
			fmt.Sprintf("Failed        %d", pullFailed+installFailed),
			fmt.Sprintf("Total time    %s", elapsed),
		})
	} else {
		elapsed := time.Since(overallStart).Truncate(time.Second)
		p.SummaryBox("Pull Complete", []string{
			fmt.Sprintf("Pulled        %d", pulled),
			fmt.Sprintf("Skipped       %d", pullSkipped),
			fmt.Sprintf("Failed        %d", pullFailed),
			fmt.Sprintf("Total time    %s", elapsed),
		})
	}

	return nil
}
