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
	"time"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/dag"
	"github.com/fireflyframework/fireflyframework-cli/internal/git"
	"github.com/fireflyframework/fireflyframework-cli/internal/java"
	"github.com/fireflyframework/fireflyframework-cli/internal/maven"
	"github.com/fireflyframework/fireflyframework-cli/internal/setup"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
	"github.com/spf13/cobra"
)

var (
	skipTests    bool
	setupRetry   bool
	setupFresh   bool
	setupFetch   bool
	setupJDKPath string
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Bootstrap the Firefly Framework into your local environment",
	Long:  "Clones all fireflyframework repos and installs them to your local Maven repository (~/.m2)",
	RunE:  runSetup,
}

func init() {
	setupCmd.Flags().BoolVar(&skipTests, "skip-tests", false, "Skip running tests during Maven install")
	setupCmd.Flags().BoolVar(&setupRetry, "retry", false, "Retry only previously failed repositories")
	setupCmd.Flags().BoolVar(&setupFresh, "fresh", false, "Force a fresh setup, ignoring any previous manifest")
	setupCmd.Flags().BoolVar(&setupFetch, "fetch-updates", false, "Fetch latest changes for already-cloned repos")
	setupCmd.Flags().StringVar(&setupJDKPath, "jdk", "", "Explicit JAVA_HOME path (skip JDK picker)")
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	overallStart := time.Now()

	p.Header("Firefly Framework Setup")

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 0 — Preflight Checks
	// ═════════════════════════════════════════════════════════════════════════
	p.StageHeader(0, "Preflight Checks")

	checks := []ui.CheckResult{}

	if git.IsInstalled() {
		gitVer, _ := git.Version()
		checks = append(checks, ui.CheckResult{Name: "Git", Status: "pass", Detail: gitVer})
	} else {
		checks = append(checks, ui.CheckResult{Name: "Git", Status: "fail", Detail: "not found — install git first"})
	}

	if maven.IsInstalled() {
		mvnVer, _ := maven.Version()
		checks = append(checks, ui.CheckResult{Name: "Maven", Status: "pass", Detail: mvnVer})
	} else {
		checks = append(checks, ui.CheckResult{Name: "Maven", Status: "fail", Detail: "not found — install maven 3.9+ first"})
	}

	if java.IsInstalled() {
		javaVer, _ := java.CurrentVersion()
		checks = append(checks, ui.CheckResult{Name: "Java", Status: "pass", Detail: fmt.Sprintf("version %d", javaVer)})
	} else {
		checks = append(checks, ui.CheckResult{Name: "Java", Status: "fail", Detail: "not found — install Java 25 (21+ minimum) first"})
	}

	p.PrintChecks(checks)
	p.Newline()

	for _, c := range checks {
		if c.Status == "fail" {
			return fmt.Errorf("preflight check failed: %s — %s", c.Name, c.Detail)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := os.MkdirAll(cfg.ReposPath, 0755); err != nil {
		return fmt.Errorf("failed to create repos directory: %w", err)
	}

	g := dag.FrameworkGraph()
	totalRepos := g.NodeCount()
	dagLayers, dagErr := g.Layers()
	if dagErr != nil {
		return fmt.Errorf("dependency graph error: %w", dagErr)
	}

	order, _ := g.FlatOrder()

	p.Info(fmt.Sprintf("Resolved dependency graph: %d repositories, %d layers", totalRepos, len(dagLayers)))
	p.Info(fmt.Sprintf("Target: %s", cfg.ReposPath))

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 1 — Resume / Retry Detection
	// ═════════════════════════════════════════════════════════════════════════
	manifestPath := setup.DefaultManifestPath()
	var manifest *setup.Manifest
	retryMode := false

	if setupRetry {
		manifest, err = setup.LoadManifest(manifestPath)
		if err != nil {
			return fmt.Errorf("failed to load setup manifest: %w", err)
		}
		if manifest == nil {
			return fmt.Errorf("no previous setup manifest found — run 'flywork setup' first")
		}
		retryMode = true
		manifest.ResetFailed()
		p.Newline()
		p.Info("Retry mode: re-processing previously failed repositories")
	} else if !setupFresh {
		manifest, err = setup.LoadManifest(manifestPath)
		if err != nil {
			p.Warning("Could not read setup manifest: " + err.Error())
			manifest = nil
		}

		if manifest != nil && !manifest.IsComplete() {
			p.Newline()
			s := manifest.Summary()
			p.Info(fmt.Sprintf("Previous setup found: %d/%d cloned, %d/%d installed, %d failed",
				s.ClonesOK, s.Total, s.InstallsOK, s.Total, s.ClonesFailed+s.InstallsFailed))

			choice := ui.Select("How would you like to proceed?", []string{
				"Resume — continue from where it left off",
				"Retry failed — only re-process failed repositories",
				"Fresh start — wipe manifest and start over",
			}, 0)

			switch {
			case len(choice) > 5 && choice[:6] == "Resume":
				p.Info("Resuming previous setup...")
			case len(choice) > 5 && choice[:5] == "Retry":
				retryMode = true
				manifest.ResetFailed()
				p.Info("Retrying failed repositories...")
			default:
				manifest = nil
			}
		} else if manifest != nil && manifest.IsComplete() {
			p.Newline()
			p.Info("Previous setup completed successfully")
			if !ui.Confirm("Run setup again?", false) {
				return nil
			}
			manifest = nil
		}
	}

	if manifest == nil {
		manifest = setup.NewManifest(order)
		manifest.SetPath(manifestPath)
	}

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 2 — JDK Selection
	// ═════════════════════════════════════════════════════════════════════════
	p.StageHeader(1, "JDK Selection")

	var javaHome string
	if setupJDKPath != "" {
		javaHome = setupJDKPath
		p.Info(fmt.Sprintf("Using specified JDK: %s", javaHome))
	} else if retryMode && manifest.JavaHome != "" {
		javaHome = manifest.JavaHome
		p.Info(fmt.Sprintf("Using previous JDK: %s", javaHome))
	} else {
		selectedHome, jdkErr := setup.SelectJDK(cfg.JavaVersion)
		if jdkErr != nil {
			p.Warning(jdkErr.Error() + " — using system default")
		} else {
			javaHome = selectedHome
		}
	}

	if javaHome != "" {
		p.Success(fmt.Sprintf("JAVA_HOME: %s", javaHome))
	}
	manifest.JavaHome = javaHome

	if !cmd.Flags().Changed("skip-tests") && !retryMode {
		skipTests = !ui.Confirm("Run tests during Maven install?", true)
	} else if retryMode && !cmd.Flags().Changed("skip-tests") {
		skipTests = manifest.SkipTests
	}
	manifest.SkipTests = skipTests

	if skipTests {
		p.Info("Tests: skipped")
	} else {
		p.Info("Tests: enabled")
	}

	_ = manifest.Save()

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 3 — Clone / Fetch Updates
	// ═════════════════════════════════════════════════════════════════════════
	p.StageHeader(2, "Cloning Repositories")
	p.Newline()

	cloneBar := ui.NewProgressBar(totalRepos, "cloned")
	cloned, skipped, cloneFailed := 0, 0, 0
	prevCloneLayer := -1

	_, _, dagErr = setup.CloneAllDAG(
		cfg.GithubOrg, cfg.ReposPath, manifest,
		func(layer int, repo string, idx, total int, r setup.CloneResult) {
			if verbose && layer != prevCloneLayer {
				if prevCloneLayer >= 0 {
					cloneBar.Finish()
				}
				p.LayerHeader(layer, len(dagLayers), len(dagLayers[layer]))
				prevCloneLayer = layer
			}

			switch {
			case r.Skipped:
				skipped++
				if verbose {
					p.Info(fmt.Sprintf("%-45s skipped", r.Repo))
				}
			case r.Error != nil:
				cloneFailed++
				p.Error(fmt.Sprintf("%-45s %s", r.Repo, r.Error))
			default:
				cloned++
				if verbose {
					p.Success(r.Repo)
				}
			}

			cloneBar.Increment()
		},
	)
	if dagErr != nil {
		return fmt.Errorf("dependency graph error: %w", dagErr)
	}

	cloneBar.Finish()
	p.Newline()
	p.Info(fmt.Sprintf("Clone: %d cloned, %d skipped, %d failed", cloned, skipped, cloneFailed))

	// Fetch updates for already-cloned repos
	fetchUpdates := setupFetch
	if !fetchUpdates && !retryMode && skipped > 0 {
		fetchUpdates = ui.Confirm("Fetch updates for already-cloned repositories?", false)
	}

	if fetchUpdates {
		p.Newline()
		clonedRepos := manifest.SuccessfulClones()
		if len(clonedRepos) > 0 {
			p.Step(fmt.Sprintf("Fetching updates for %d repositories...", len(clonedRepos)))
			fetchBar := ui.NewProgressBar(len(clonedRepos), "fetched")
			fetchFailed := 0

			setup.FetchUpdates(cfg.ReposPath, clonedRepos,
				func(repo string, idx, total int, r setup.FetchResult) {
					if r.Error != nil {
						fetchFailed++
						if verbose {
							p.Warning(fmt.Sprintf("%-45s %s", repo, r.Error))
						}
					}
					fetchBar.Increment()
				},
			)

			fetchBar.Finish()
			p.Newline()
			if fetchFailed > 0 {
				p.Warning(fmt.Sprintf("Fetch: %d/%d failed (non-fatal)", fetchFailed, len(clonedRepos)))
			} else {
				p.Success(fmt.Sprintf("Fetch: %d repositories updated", len(clonedRepos)))
			}
		}
	}

	if cloneFailed > 0 && cloned+skipped == 0 {
		return fmt.Errorf("all repositories failed to clone")
	}

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 4 — Maven Install
	// ═════════════════════════════════════════════════════════════════════════
	p.StageHeader(3, "Installing Artifacts")
	p.Newline()

	var reposFilter map[string]bool
	if retryMode {
		pending := manifest.PendingInstalls()
		if len(pending) > 0 {
			reposFilter = make(map[string]bool, len(pending))
			for _, r := range pending {
				reposFilter[r] = true
			}
			p.Info(fmt.Sprintf("Retry: %d repositories to install", len(pending)))
		} else {
			p.Info("No repositories need installation")
		}
	}

	installBar := ui.NewProgressBar(totalRepos, "installed")
	var activeSpinner *ui.Spinner
	installed, installSkipped, installFailed := 0, 0, 0
	prevInstallLayer := -1

	_, _, dagErr = setup.InstallAllDAG(
		cfg.ReposPath, javaHome, skipTests, manifest, reposFilter,
		func(layer int, repo string, idx, total int) {
			if verbose && layer != prevInstallLayer {
				if prevInstallLayer >= 0 {
					installBar.Finish()
				}
				p.LayerHeader(layer, len(dagLayers), len(dagLayers[layer]))
				prevInstallLayer = layer
			}
			activeSpinner = ui.NewSpinner(fmt.Sprintf("Building %s...", repo))
			activeSpinner.Start()
		},
		func(layer int, repo string, idx, total int, r setup.InstallResult) {
			if activeSpinner != nil {
				activeSpinner.Stop(r.Error == nil)
				activeSpinner = nil
			}

			switch {
			case r.Skipped:
				installSkipped++
			case r.Error != nil:
				installFailed++
				p.Error(fmt.Sprintf("%-45s %s", r.Repo, r.Error))
			default:
				installed++
			}

			installBar.Increment()
		},
	)
	if dagErr != nil {
		return fmt.Errorf("dependency graph error: %w", dagErr)
	}

	installBar.Finish()
	p.Newline()
	p.Info(fmt.Sprintf("Install: %d installed, %d skipped, %d failed",
		installed, installSkipped, installFailed))

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 5 — Post-Install: Retry Loop
	// ═════════════════════════════════════════════════════════════════════════
	for installFailed > 0 {
		p.Newline()
		failedRepos := manifest.FailedInstalls()
		p.Warning(fmt.Sprintf("%d repositories failed to install:", len(failedRepos)))
		for _, r := range failedRepos {
			rs := manifest.Repo(r)
			p.Error(fmt.Sprintf("  %s — %s", r, rs.InstallError))
		}

		p.Newline()
		if !ui.Confirm("Retry failed repositories now?", true) {
			break
		}

		manifest.ResetFailed()
		_ = manifest.Save()

		retryFilter := make(map[string]bool, len(failedRepos))
		for _, r := range failedRepos {
			retryFilter[r] = true
		}

		retryBar := ui.NewProgressBar(totalRepos, "installed")
		installed, installSkipped, installFailed = 0, 0, 0

		_, _, dagErr = setup.InstallAllDAG(
			cfg.ReposPath, javaHome, skipTests, manifest, retryFilter,
			func(layer int, repo string, idx, total int) {
				activeSpinner = ui.NewSpinner(fmt.Sprintf("Retrying %s...", repo))
				activeSpinner.Start()
			},
			func(layer int, repo string, idx, total int, r setup.InstallResult) {
				if activeSpinner != nil {
					activeSpinner.Stop(r.Error == nil)
					activeSpinner = nil
				}

				switch {
				case r.Skipped:
					installSkipped++
				case r.Error != nil:
					installFailed++
					p.Error(fmt.Sprintf("%-45s %s", r.Repo, r.Error))
				default:
					installed++
				}

				retryBar.Increment()
			},
		)
		if dagErr != nil {
			return fmt.Errorf("dependency graph error: %w", dagErr)
		}

		retryBar.Finish()
		p.Newline()
		p.Info(fmt.Sprintf("Retry: %d installed, %d still failing", installed, installFailed))
	}

	// ═════════════════════════════════════════════════════════════════════════
	// Summary
	// ═════════════════════════════════════════════════════════════════════════
	elapsed := time.Since(overallStart).Truncate(time.Second)

	s := manifest.Summary()
	if s.ClonesFailed == 0 && s.InstallsFailed == 0 && s.ClonesPending == 0 && s.InstallsPending == 0 {
		manifest.MarkComplete()
	}
	_ = manifest.Save()

	if err := cfg.Save(); err != nil {
		p.Warning("Could not save config: " + err.Error())
	}

	status := "Setup Complete"
	if s.ClonesFailed > 0 || s.InstallsFailed > 0 {
		status = "Setup Incomplete"
	}

	p.SummaryBox(status, []string{
		fmt.Sprintf("Repositories  %d", s.Total),
		fmt.Sprintf("Cloned        %d  (skipped %d, failed %d)", s.ClonesOK, skipped, s.ClonesFailed),
		fmt.Sprintf("Installed     %d  (skipped %d, failed %d)", s.InstallsOK, installSkipped, s.InstallsFailed),
		fmt.Sprintf("DAG layers    %d", len(dagLayers)),
		fmt.Sprintf("Total time    %s", elapsed),
		fmt.Sprintf("Manifest      %s", manifestPath),
	})

	if s.ClonesFailed > 0 || s.InstallsFailed > 0 {
		p.Newline()
		p.Info("Run 'flywork setup --retry' to retry failed repositories")
	}

	return nil
}
