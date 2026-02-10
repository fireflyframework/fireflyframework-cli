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
	"strings"
	"time"

	"github.com/fireflyframework/fireflyframework-cli/internal/build"
	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/dag"
	"github.com/fireflyframework/fireflyframework-cli/internal/git"
	"github.com/fireflyframework/fireflyframework-cli/internal/java"
	"github.com/fireflyframework/fireflyframework-cli/internal/maven"
	"github.com/fireflyframework/fireflyframework-cli/internal/publish"
	"github.com/fireflyframework/fireflyframework-cli/internal/setup"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
	"github.com/spf13/cobra"
)

var (
	publishAll       bool
	publishRepo      string
	publishDryRun    bool
	publishSkipTests bool
	publishJDKPath   string
)

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Publish artifacts to GitHub Packages",
	Long: `Deploys Maven artifacts to GitHub Packages and Python packages as GitHub
Release assets, using DAG-aware ordering with change detection.

Requires the GITHUB_TOKEN environment variable to be set with 'write:packages'
scope. The token is used for Maven deploy authentication and Python package
release uploads.

The publish process runs through the following phases:

  Phase 0 — Preflight Checks
    Verifies GITHUB_TOKEN is set, and Git, Maven, and Java are available.

  Phase 1 — Maven Settings
    Ensures ~/.m2/settings.xml contains the GitHub Packages server
    configuration for authentication.

  Phase 2 — Publish Plan
    Uses the same SHA-based change detection as 'flywork build' to determine
    which repos need publishing. Displays repos grouped by DAG layer.

  Phase 3 — Maven Deploy
    Runs 'mvn deploy' on each affected repository in dependency order with
    progress bars and per-repo spinners.

  Phase 4 — Python Publish (conditional)
    If fireflyframework-genai is in scope, publishes the Python package as
    GitHub Release assets.

  Phase 5 — Summary
    Reports published/skipped/failed counts and total time.

Use --all to publish everything regardless of change detection. Use --repo to
publish a specific repository only. Use --dry-run to preview without publishing.

Examples:
  flywork publish                     Publish changed repos
  flywork publish --all               Publish everything
  flywork publish --repo <name>       Publish a specific repo
  flywork publish --dry-run           Preview what would be published
  flywork publish --skip-tests=false  Run tests during deploy
  flywork publish --jdk /path/to/jdk  Use a specific JAVA_HOME`,
	RunE: runPublish,
}

func init() {
	publishCmd.Flags().BoolVar(&publishAll, "all", false, "Publish everything (ignore change detection)")
	publishCmd.Flags().StringVar(&publishRepo, "repo", "", "Publish a specific repo only")
	publishCmd.Flags().BoolVar(&publishDryRun, "dry-run", false, "Show what would be published without publishing")
	publishCmd.Flags().BoolVar(&publishSkipTests, "skip-tests", true, "Skip tests during deploy (default: true)")
	publishCmd.Flags().StringVar(&publishJDKPath, "jdk", "", "Explicit JAVA_HOME path")
	rootCmd.AddCommand(publishCmd)
}

func runPublish(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	overallStart := time.Now()

	p.Header("Publish to GitHub Packages")

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 0 — Preflight Checks
	// ═════════════════════════════════════════════════════════════════════════
	p.StageHeader(0, "Preflight Checks")

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required for publishing to GitHub Packages")
	}

	checks := []ui.CheckResult{
		{Name: "GITHUB_TOKEN", Status: "pass", Detail: "set"},
	}

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
	// Phase 1 — Maven Settings
	// ═════════════════════════════════════════════════════════════════════════
	p.StageHeader(1, "Maven Settings")

	modified, err := publish.EnsureSettingsXML()
	if err != nil {
		return fmt.Errorf("failed to configure Maven settings: %w", err)
	}
	if modified {
		p.Success("Updated ~/.m2/settings.xml with GitHub Packages server")
	} else {
		p.Info("~/.m2/settings.xml already configured")
	}

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 2 — Publish Plan
	// ═════════════════════════════════════════════════════════════════════════
	p.StageHeader(2, "Publish Plan")

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

	if publishAll {
		p.Info("Mode: publish all (--all)")
		affected = make(map[string]bool)
		for _, n := range g.Nodes() {
			affected[n] = true
		}
	} else if publishRepo != "" {
		if !g.HasNode(publishRepo) {
			return fmt.Errorf("unknown repository: %s", publishRepo)
		}
		p.Info(fmt.Sprintf("Mode: publishing %s", publishRepo))
		affected = map[string]bool{publishRepo: true}
	} else {
		p.Info(fmt.Sprintf("%d repos changed, %d total to publish", len(changed), len(affected)))
	}

	if len(affected) == 0 {
		p.Newline()
		p.Success("Everything is up to date — nothing to publish")
		return nil
	}

	sub := g.Subgraph(affected)
	layers, err := sub.Layers()
	if err != nil {
		return fmt.Errorf("failed to compute publish layers: %w", err)
	}

	totalToPublish := 0
	for i, layer := range layers {
		p.LayerHeader(i, len(layers), len(layer))
		for _, repo := range layer {
			short := strings.TrimPrefix(repo, "fireflyframework-")
			fmt.Printf("    %s %s\n", ui.StyleMuted.Render("•"), short)
		}
		totalToPublish += len(layer)
	}

	p.Newline()
	p.Info(fmt.Sprintf("Plan: %d repos to publish across %d layers", totalToPublish, len(layers)))

	if publishDryRun {
		p.Newline()
		p.Info("Dry run — no artifacts published")
		return nil
	}

	if !ui.Confirm("Proceed with publish?", true) {
		return nil
	}

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 3 — Maven Deploy
	// ═════════════════════════════════════════════════════════════════════════
	p.StageHeader(3, "Publishing Maven Artifacts")
	p.Newline()

	javaHome := publishJDKPath
	if javaHome == "" {
		selectedHome, jdkErr := setup.SelectJDK(cfg.JavaVersion)
		if jdkErr != nil {
			p.Warning(jdkErr.Error() + " — using system default")
		} else {
			javaHome = selectedHome
		}
	}

	opts := publish.PublishOptions{
		ReposDir:  cfg.ReposPath,
		JavaHome:  javaHome,
		GithubOrg: cfg.GithubOrg,
		SkipTests: publishSkipTests,
		ForceAll:  publishAll,
		DryRun:    false,
	}
	if publishRepo != "" {
		opts.TargetRepos = []string{publishRepo}
	}

	bar := ui.NewProgressBar(totalToPublish, "published")
	var activeSpinner *ui.Spinner
	published, pubSkipped, pubFailed := 0, 0, 0
	prevLayer := -1

	results, _, err := publish.PublishAllDAG(
		opts,
		func(layer int, repo string, idx, total int) {
			if verbose && layer != prevLayer {
				if prevLayer >= 0 {
					bar.Finish()
				}
				p.LayerHeader(layer, len(layers), len(layers[layer]))
				prevLayer = layer
			}
			activeSpinner = ui.NewSpinner(fmt.Sprintf("Publishing %s...", strings.TrimPrefix(repo, "fireflyframework-")))
			activeSpinner.Start()
		},
		func(layer int, repo string, idx, total int, r publish.PublishResult) {
			if activeSpinner != nil {
				activeSpinner.Stop(r.Error == nil)
				activeSpinner = nil
			}

			switch {
			case r.Skipped:
				pubSkipped++
			case r.Error != nil:
				pubFailed++
				p.Error(fmt.Sprintf("%-45s %s", repo, r.Error))
				if r.LogFile != "" {
					p.Info(fmt.Sprintf("  Log: %s", r.LogFile))
				}
			default:
				published++
			}

			bar.Increment()
		},
	)
	if err != nil {
		return fmt.Errorf("publish error: %w", err)
	}

	bar.Finish()

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 4 — Python Publish (if genai in scope)
	// ═════════════════════════════════════════════════════════════════════════
	genaiDir := filepath.Join(cfg.ReposPath, "fireflyframework-genai")
	if _, serr := os.Stat(genaiDir); serr == nil {
		if publishAll || publishRepo == "fireflyframework-genai" {
			p.StageHeader(4, "Publishing Python Package")

			err := publish.PublishPython(genaiDir, cfg.GithubOrg)
			if err != nil {
				p.Error(fmt.Sprintf("Python publish failed: %s", err))
				pubFailed++
			} else {
				p.Success("Python package published as GitHub Release assets")
				published++
			}
		}
	}

	// ═════════════════════════════════════════════════════════════════════════
	// Phase 5 — Summary
	// ═════════════════════════════════════════════════════════════════════════
	elapsed := time.Since(overallStart).Truncate(time.Second)

	status := "Publish Complete"
	if pubFailed > 0 {
		status = "Publish Incomplete"
	}

	summaryLines := []string{
		fmt.Sprintf("Published     %d", published),
		fmt.Sprintf("Skipped       %d", pubSkipped),
		fmt.Sprintf("Failed        %d", pubFailed),
		fmt.Sprintf("Layers        %d", len(layers)),
		fmt.Sprintf("Total time    %s", elapsed),
	}
	p.SummaryBox(status, summaryLines)

	if pubFailed > 0 {
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
