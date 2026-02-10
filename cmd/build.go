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
	"strings"
	"time"

	"github.com/fireflyframework/fireflyframework-cli/internal/build"
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
	buildAll       bool
	buildRepo      string
	buildDryRun    bool
	buildSkipTests bool
	buildJDKPath   string
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Smart DAG-aware build with change detection",
	Long:  "Detects which repos have changed since the last build, computes affected downstream repos, and builds them in dependency order",
	RunE:  runBuild,
}

func init() {
	buildCmd.Flags().BoolVar(&buildAll, "all", false, "Rebuild everything (ignore change detection)")
	buildCmd.Flags().StringVar(&buildRepo, "repo", "", "Build a specific repo and its dependents")
	buildCmd.Flags().BoolVar(&buildDryRun, "dry-run", false, "Show what would be built without building")
	buildCmd.Flags().BoolVar(&buildSkipTests, "skip-tests", false, "Skip running tests during Maven install")
	buildCmd.Flags().StringVar(&buildJDKPath, "jdk", "", "Explicit JAVA_HOME path")
	rootCmd.AddCommand(buildCmd)
}

func runBuild(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	overallStart := time.Now()

	p.Header("Smart Build")

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 0 — Preflight Checks
	// ═════════════════════════════════════════════════════════════════════════
	p.StageHeader(0, "Preflight Checks")

	checks := []ui.CheckResult{}

	if git.IsInstalled() {
		gitVer, _ := git.Version()
		checks = append(checks, ui.CheckResult{Name: "Git", Status: "pass", Detail: gitVer})
	} else {
		checks = append(checks, ui.CheckResult{Name: "Git", Status: "fail", Detail: "not found"})
	}

	if maven.IsInstalled() {
		mvnVer, _ := maven.Version()
		checks = append(checks, ui.CheckResult{Name: "Maven", Status: "pass", Detail: mvnVer})
	} else {
		checks = append(checks, ui.CheckResult{Name: "Maven", Status: "fail", Detail: "not found"})
	}

	if java.IsInstalled() {
		javaVer, _ := java.CurrentVersion()
		checks = append(checks, ui.CheckResult{Name: "Java", Status: "pass", Detail: fmt.Sprintf("version %d", javaVer)})
	} else {
		checks = append(checks, ui.CheckResult{Name: "Java", Status: "fail", Detail: "not found"})
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

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 1 — Change Detection
	// ═════════════════════════════════════════════════════════════════════════
	p.StageHeader(1, "Change Detection")

	g := dag.FrameworkGraph()

	manifest, err := build.LoadManifest(build.DefaultManifestPath())
	if err != nil {
		p.Warning("Could not load build manifest: " + err.Error())
		manifest = nil
	}
	if manifest == nil {
		manifest = build.NewManifest()
	}

	changed := build.DetectChanges(g, cfg.ReposPath, manifest)
	affected := build.TransitiveClosure(g, changed)

	if buildAll {
		p.Info("Mode: full rebuild (--all)")
		affected = make(map[string]bool)
		for _, n := range g.Nodes() {
			affected[n] = true
		}
	} else if buildRepo != "" {
		if !g.HasNode(buildRepo) {
			return fmt.Errorf("unknown repository: %s", buildRepo)
		}
		p.Info(fmt.Sprintf("Mode: targeted build for %s + dependents", buildRepo))
	} else {
		p.Info(fmt.Sprintf("%d repos changed, %d affected by dependencies", len(changed), len(affected)-len(changed)))
	}

	if len(affected) == 0 && !buildAll {
		p.Newline()
		p.Success("Everything is up to date — nothing to build")
		return nil
	}

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 2 — Build Plan
	// ═════════════════════════════════════════════════════════════════════════
	p.StageHeader(2, "Build Plan")

	// Build subgraph for display
	var displaySet map[string]bool
	if buildRepo != "" {
		displaySet = make(map[string]bool)
		displaySet[buildRepo] = true
		for _, dep := range g.TransitiveDependentsOf(buildRepo) {
			displaySet[dep] = true
		}
		if !buildAll {
			for repo := range displaySet {
				if !affected[repo] {
					delete(displaySet, repo)
				}
			}
		}
	} else {
		displaySet = affected
	}

	sub := g.Subgraph(displaySet)
	layers, err := sub.Layers()
	if err != nil {
		return fmt.Errorf("failed to compute build layers: %w", err)
	}

	totalToBuild := 0
	for i, layer := range layers {
		p.LayerHeader(i, len(layers), len(layer))
		for _, repo := range layer {
			short := strings.TrimPrefix(repo, "fireflyframework-")
			marker := " "
			if changed[repo] {
				marker = ui.StyleWarning.Render("*")
			}
			fmt.Printf("    %s %s\n", marker, short)
		}
		totalToBuild += len(layer)
	}

	p.Newline()
	p.Info(fmt.Sprintf("Plan: %d repos to build across %d layers", totalToBuild, len(layers)))

	if buildDryRun {
		p.Newline()
		p.Info("Dry run — no builds executed")
		return nil
	}

	if !ui.Confirm("Proceed with build?", true) {
		return nil
	}

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 3 — DAG Build
	// ═════════════════════════════════════════════════════════════════════════
	p.StageHeader(3, "Building")
	p.Newline()

	javaHome := buildJDKPath
	if javaHome == "" && buildJDKPath == "" {
		selectedHome, jdkErr := setup.SelectJDK(cfg.JavaVersion)
		if jdkErr != nil {
			p.Warning(jdkErr.Error() + " — using system default")
		} else {
			javaHome = selectedHome
		}
	}

	opts := build.BuildOptions{
		ReposDir:  cfg.ReposPath,
		JavaHome:  javaHome,
		SkipTests: buildSkipTests,
		ForceAll:  buildAll,
		DryRun:    false,
	}
	if buildRepo != "" {
		opts.TargetRepos = []string{buildRepo}
	}

	bar := ui.NewProgressBar(totalToBuild, "built")
	var activeSpinner *ui.Spinner
	built, skipped, failed := 0, 0, 0
	prevLayer := -1

	results, _, err := build.RunDAGBuild(
		opts,
		func(layer int, repo string, idx, total int) {
			if verbose && layer != prevLayer {
				if prevLayer >= 0 {
					bar.Finish()
				}
				p.LayerHeader(layer, len(layers), len(layers[layer]))
				prevLayer = layer
			}
			activeSpinner = ui.NewSpinner(fmt.Sprintf("Building %s...", strings.TrimPrefix(repo, "fireflyframework-")))
			activeSpinner.Start()
		},
		func(layer int, repo string, idx, total int, r build.BuildResult) {
			if activeSpinner != nil {
				activeSpinner.Stop(r.Error == nil)
				activeSpinner = nil
			}

			switch {
			case r.Skipped:
				skipped++
			case r.Error != nil:
				failed++
				p.Error(fmt.Sprintf("%-45s %s", repo, r.Error))
				if r.LogFile != "" {
					p.Info(fmt.Sprintf("  Log: %s", r.LogFile))
				}
			default:
				built++
			}

			bar.Increment()
		},
	)
	if err != nil {
		return fmt.Errorf("build error: %w", err)
	}

	bar.Finish()

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 4 — Summary
	// ═════════════════════════════════════════════════════════════════════════
	elapsed := time.Since(overallStart).Truncate(time.Second)

	status := "Build Complete"
	if failed > 0 {
		status = "Build Incomplete"
	}

	summaryLines := []string{
		fmt.Sprintf("Built         %d", built),
		fmt.Sprintf("Skipped       %d", skipped),
		fmt.Sprintf("Failed        %d", failed),
		fmt.Sprintf("Layers        %d", len(layers)),
		fmt.Sprintf("Total time    %s", elapsed),
	}

	if failed > 0 {
		summaryLines = append(summaryLines, fmt.Sprintf("Build logs    %s", build.LogsDir()))
	}

	p.SummaryBox(status, summaryLines)

	if failed > 0 {
		p.Newline()
		p.Info("Failed repositories:")
		for _, r := range results {
			if r.Error != nil {
				p.Error(fmt.Sprintf("  %s — %s", r.Repo, r.Error))
			}
		}
	}

	return nil
}
