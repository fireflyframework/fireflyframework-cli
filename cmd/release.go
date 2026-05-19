// Copyright 2024-2026 Firefly Software Foundation.
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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/dag"
	"github.com/fireflyframework/fireflyframework-cli/internal/git"
	"github.com/fireflyframework/fireflyframework-cli/internal/setup"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
	"github.com/fireflyframework/fireflyframework-cli/internal/version"
	"github.com/spf13/cobra"
)

// ── flywork release ─────────────────────────────────────────────────────────

var (
	releaseTargetYear     int
	releaseTargetMonth    int
	releaseTargetPatch    int
	releaseDryRun         bool
	releaseSkipPRs        bool
	releaseSkipTag        bool
	releaseFromLayer      int
	releaseAdminMerge     bool
	releasePerRepoTimeout int
	releaseRepoOverride   string
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Orchestrate end-to-end framework release (bump + PR + merge + tag + wait)",
	Long: `Orchestrates a full Firefly Framework release across every Java repo.

Phases per DAG layer (parent → ... → adapters → starter-application):
  1. Switch each repo to release/<target>
  2. Push branch to origin
  3. Open PR (release/<target> → main) with auto-merge enabled
  4. Wait for CI green (re-trigger on transient failures from missing parent layer artifacts)
  5. Admin-merge if review-required gate blocks auto-merge
  6. Tag main with v<target>, push tag → fires release workflow
  7. Wait for release workflow's "github-packages" job to succeed
  8. Re-trigger CIs on next layer's open PRs (empty commit) so they pick up newly-published deps

Requires:
  - gh CLI authenticated (gh auth status)
  - All repos cloned (run 'flywork setup' first)
  - All repos on a clean working tree (run 'flywork fwversion check')

Examples:
  flywork release                              Auto-compute next CalVer; orchestrate end-to-end
  flywork release --year 26 --month 5 --patch 7    Explicit target version
  flywork release --from-layer 3               Resume cascade starting at layer 3
  flywork release --dry-run                    Print plan only, no changes
  flywork release --skip-prs                   Push directly to main, skip PR step (advanced)`,
	RunE: runRelease,
}

func runRelease(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	overallStart := time.Now()

	// ── Phase 1: Pre-flight ──────────────────────────────────────────────
	p.StageHeader(1, "Pre-flight")
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if _, err := os.Stat(cfg.ReposPath); os.IsNotExist(err) {
		return fmt.Errorf("repos directory not found: %s (run 'flywork setup' first)", cfg.ReposPath)
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found in PATH; install from https://cli.github.com")
	}

	// Resolve target version
	parentPom := filepath.Join(cfg.ReposPath, "fireflyframework-parent", "pom.xml")
	currentVer := cfg.ParentVersion
	if detected, err := version.ReadPomVersion(parentPom); err == nil {
		currentVer = detected
	}
	var target version.CalVer
	if releaseTargetYear > 0 || releaseTargetMonth > 0 || releaseTargetPatch > 0 {
		target = version.CalVer{Year: releaseTargetYear, Month: releaseTargetMonth, Patch: releaseTargetPatch}
	} else {
		current, err := version.Parse(currentVer)
		if err != nil {
			current = version.Current()
		}
		target = version.Next(current)
	}
	targetStr := target.String()
	releaseBranch := "release/" + targetStr
	releaseTag := "v" + targetStr

	p.KeyValue("Current version", currentVer)
	p.KeyValue("Target version", targetStr)
	p.KeyValue("Release branch", releaseBranch)
	p.KeyValue("Release tag", releaseTag)
	if releaseDryRun {
		p.Info("DRY RUN — no remote changes will be made")
	}
	p.Newline()

	// ── Phase 2: Resolve DAG layers ──────────────────────────────────────
	p.StageHeader(2, "DAG resolution")
	graph := dag.FrameworkGraph()
	layers, err := graph.Layers()
	if err != nil {
		return fmt.Errorf("dag layers: %w", err)
	}
	p.KeyValue("Layers", fmt.Sprintf("%d", len(layers)))
	for i, layer := range layers {
		p.Info(fmt.Sprintf("  Layer %d (%d repos): %s", i, len(layer), strings.Join(layer, ", ")))
	}
	p.Newline()

	// ── Phase 3: Per-layer cascade ────────────────────────────────────────
	for layerIdx, layer := range layers {
		if layerIdx < releaseFromLayer {
			p.Info(fmt.Sprintf("Skipping layer %d (--from-layer=%d)", layerIdx, releaseFromLayer))
			continue
		}
		p.StageHeader(3+layerIdx, fmt.Sprintf("Layer %d (%d repos)", layerIdx, len(layer)))

		// Per-repo: branch + push + PR + auto-merge
		for _, repo := range layer {
			if releaseRepoOverride != "" && repo != releaseRepoOverride {
				continue
			}
			if !isPartOfBuild(repo) {
				continue
			}
			if err := processRepoForRelease(p, cfg.ReposPath, repo, releaseBranch, targetStr); err != nil {
				p.Warning(fmt.Sprintf("%s: %s", repo, err))
			}
		}

		// Wait for all PRs in this layer to merge + tag + GH packages publish
		if err := waitForLayerRelease(p, layer, releaseTag); err != nil {
			p.Error(fmt.Sprintf("layer %d wait failed: %s", layerIdx, err))
			return err
		}

		// Re-trigger CIs on next layer
		if layerIdx+1 < len(layers) {
			retriggerCIsOnLayer(p, cfg.ReposPath, layers[layerIdx+1], releaseBranch)
		}
	}

	// ── Phase 4: Verification ────────────────────────────────────────────
	p.StageHeader(99, "Verification")
	report, err := version.CheckAll(cfg.ReposPath)
	if err != nil {
		p.Warning("Could not verify: " + err.Error())
	} else if report.Consistent {
		for ver := range report.UniqueVersions {
			if ver == targetStr {
				p.Success(fmt.Sprintf("All %d repos at %s", report.TotalWithPom, targetStr))
			} else {
				p.Warning(fmt.Sprintf("Repos at unexpected version %s", ver))
			}
		}
	}

	elapsed := time.Since(overallStart).Truncate(time.Second)
	p.SummaryBox("Release Complete", []string{
		fmt.Sprintf("Version       %s → %s", currentVer, targetStr),
		fmt.Sprintf("Layers        %d", len(layers)),
		fmt.Sprintf("Total time    %s", elapsed),
	})
	return nil
}

// isPartOfBuild filters out non-Java sibling repos that show up in the DAG.
func isPartOfBuild(repo string) bool {
	skip := []string{"fireflyframework-genai", "fireflyframework-agentic"}
	for _, s := range skip {
		if repo == s {
			return false
		}
	}
	return true
}

func processRepoForRelease(p *ui.Printer, reposDir, repo, releaseBranch, targetStr string) error {
	repoDir := filepath.Join(reposDir, repo)
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		return fmt.Errorf("not cloned")
	}

	// Switch to release branch (create if missing, hard-reset to current main otherwise)
	if err := git.RunCmd(repoDir, "checkout", "-B", releaseBranch); err != nil {
		return fmt.Errorf("checkout: %w", err)
	}

	// Push the branch (no-op if up to date)
	if releaseDryRun {
		p.Info(repo + " [dry-run] would push " + releaseBranch)
	} else {
		if err := git.RunCmd(repoDir, "push", "-u", "origin", releaseBranch); err != nil {
			return fmt.Errorf("push: %w", err)
		}
	}

	if releaseSkipPRs {
		return nil
	}

	if releaseDryRun {
		p.Info(repo + " [dry-run] would open PR / enable auto-merge")
		return nil
	}

	// Skip if main is already at release branch (nothing to release)
	if out, _ := runGH(repoDir, "pr", "list", "--state", "merged", "--head", releaseBranch, "--json", "number", "-q", ".[0].number"); strings.TrimSpace(out) != "" {
		// Branch already merged in the past — treat as nothing-to-do
		return nil
	}

	// Create PR if not existing
	prNum := findOpenPRNumber(repoDir, releaseBranch)
	if prNum == 0 {
		title := "release: " + targetStr
		body := "Bump to " + targetStr + ".\n\nCreated by `flywork release`."
		out, err := runGH(repoDir, "pr", "create", "--base", "main", "--head", releaseBranch, "--title", title, "--body", body)
		if err != nil {
			// "No commits between main and release/x" means already in sync — not an error
			if strings.Contains(out, "No commits between") {
				return nil
			}
			return fmt.Errorf("pr create: %w: %s", err, strings.TrimSpace(out))
		}
		p.Success(repo + " PR opened")
	}

	// Enable auto-merge
	_, _ = runGH(repoDir, "pr", "merge", "--auto", "--squash", "--delete-branch")
	return nil
}

func findOpenPRNumber(repoDir, branch string) int {
	out, err := runGH(repoDir, "pr", "list", "--state", "open", "--head", branch, "--json", "number", "-q", ".[0].number")
	if err != nil {
		return 0
	}
	out = strings.TrimSpace(out)
	if out == "" || out == "null" {
		return 0
	}
	var n int
	fmt.Sscanf(out, "%d", &n)
	return n
}

func runGH(repoDir string, args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// waitForLayerRelease blocks until every repo in the layer has its v<target>
// release workflow's github-packages job in SUCCESS state. Repos are tagged
// here (after their PR merges) and we then poll until the release workflow's
// gh-packages step completes.
func waitForLayerRelease(p *ui.Printer, layer []string, tag string) error {
	deadline := time.Now().Add(time.Duration(releasePerRepoTimeout) * time.Minute)
	for _, repo := range layer {
		if !isPartOfBuild(repo) {
			continue
		}
		if err := mergeAndTagRepo(p, repo, tag); err != nil {
			p.Warning(fmt.Sprintf("%s tag: %s", repo, err))
		}
	}
	// Now wait for github-packages
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for layer release")
		}
		allDone := true
		for _, repo := range layer {
			if !isPartOfBuild(repo) {
				continue
			}
			conc, _ := ghPackagesStatus(repo)
			if conc != "success" {
				allDone = false
				break
			}
		}
		if allDone {
			p.Success("Layer release complete (all on GitHub Packages)")
			return nil
		}
		time.Sleep(30 * time.Second)
	}
}

// mergeAndTagRepo merges the open PR for the repo (if any), pulls main, and
// pushes a new annotated tag. Uses --admin to bypass branch protection when
// the gh token has admin rights on the repo.
func mergeAndTagRepo(p *ui.Printer, repo, tag string) error {
	repoDir := filepath.Join(cfgRepoPath(), repo)
	// Admin merge the PR (no-op if already merged)
	_, _ = runGH(repoDir, "pr", "merge", "--squash", "--delete-branch", "--admin")
	// Pull main
	if err := git.RunCmd(repoDir, "checkout", "main"); err != nil {
		return err
	}
	if err := git.RunCmd(repoDir, "pull", "--ff-only"); err != nil {
		return err
	}
	// Tag (no-op if already exists)
	if err := git.RunCmd(repoDir, "tag", "-a", tag, "-m", "Release "+tag); err != nil {
		// May fail with "tag already exists" — ignore
	}
	if releaseDryRun {
		return nil
	}
	if err := git.RunCmd(repoDir, "push", "origin", tag); err != nil {
		return fmt.Errorf("push tag: %w", err)
	}
	p.Success(repo + " tagged " + tag)
	return nil
}

func cfgRepoPath() string {
	cfg, _ := config.Load()
	return cfg.ReposPath
}

// ghPackagesStatus returns the conclusion of the github-packages job in the
// latest release workflow run, or "" if not yet reported.
func ghPackagesStatus(repo string) (string, error) {
	out, err := runGH(filepath.Join(cfgRepoPath(), repo), "run", "list",
		"--workflow=release.yml", "--limit", "1", "--json", "databaseId", "-q", ".[0].databaseId")
	if err != nil {
		return "", err
	}
	runID := strings.TrimSpace(out)
	if runID == "" {
		return "", nil
	}
	jobsOut, err := runGH(filepath.Join(cfgRepoPath(), repo), "run", "view", runID, "--json", "jobs")
	if err != nil {
		return "", err
	}
	var data struct {
		Jobs []struct {
			Name       string `json:"name"`
			Conclusion string `json:"conclusion"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(jobsOut), &data); err != nil {
		return "", err
	}
	for _, j := range data.Jobs {
		if strings.Contains(j.Name, "github-packages") {
			return j.Conclusion, nil
		}
	}
	return "", nil
}

// retriggerCIsOnLayer pushes an empty commit to each open release branch in the
// next layer so their CIs pick up newly-published parent artifacts.
func retriggerCIsOnLayer(p *ui.Printer, reposDir string, layer []string, releaseBranch string) {
	for _, repo := range layer {
		if !isPartOfBuild(repo) {
			continue
		}
		repoDir := filepath.Join(reposDir, repo)
		if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
			continue
		}
		_ = git.RunCmd(repoDir, "checkout", releaseBranch)
		if err := git.RunCmd(repoDir, "commit", "--allow-empty", "-m", "ci: re-trigger after parent layer publish"); err == nil {
			_ = git.RunCmd(repoDir, "push")
		}
	}
}

// ── flywork release status ──────────────────────────────────────────────────

var releaseStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show in-flight release status (per-repo: PR state, CI, release workflow)",
	RunE:  runReleaseStatus,
}

func runReleaseStatus(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	p.Header("Release status @ " + cfg.ParentVersion)
	p.Newline()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_ = ctx

	tag := "v" + cfg.ParentVersion
	for _, repo := range setup.FrameworkRepos {
		if !isPartOfBuild(repo) {
			continue
		}
		// Source of truth: does the tag exist? (releases lag the workflow)
		tagOut, tagErr := runGH(filepath.Join(cfg.ReposPath, repo), "api",
			"/repos/fireflyframework/"+repo+"/git/refs/tags/"+tag, "-q", ".object.sha")
		hasTag := tagErr == nil && strings.TrimSpace(tagOut) != "" && !strings.Contains(tagOut, "Not Found")

		// Release page (lags but signals workflow has fully finished)
		relOut, _ := runGH(filepath.Join(cfg.ReposPath, repo), "release", "list", "--limit", "1", "--json", "tagName", "-q", ".[0].tagName")
		latestRelease := strings.TrimSpace(relOut)

		marker := "  "
		state := latestRelease
		switch {
		case hasTag && latestRelease == tag:
			marker = ui.StyleSuccess.Render("✓ ")
		case hasTag:
			marker = ui.StyleWarning.Render("⏳")
			state = tag + " (tagged, publish in-progress)"
		}
		fmt.Printf("%s%-45s %s\n", marker, repo, state)
	}
	return nil
}

// ── flywork release verify ──────────────────────────────────────────────────
// 3-column status: git tag / GH Packages / Maven Central — for a target version.

var verifyVersion string

var releaseVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify a released version across git tags, GitHub Packages, and Maven Central",
	RunE:  runReleaseVerify,
}

func runReleaseVerify(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ver := verifyVersion
	if ver == "" {
		ver = cfg.ParentVersion
	}
	tag := "v" + ver

	p.Header("Release verification @ " + ver)
	p.Newline()
	fmt.Printf("  %-50s %-12s %-12s %-12s\n", "Repo", "Tag", "GH Pkgs", "Mvn Central")
	fmt.Println("  " + strings.Repeat("─", 92))

	var tagOK, ghOK, mcOK int
	for _, repo := range setup.FrameworkRepos {
		if !isPartOfBuild(repo) {
			continue
		}
		tagMark := mark(checkTagExists(repo, tag))
		ghMark := mark(checkGHPackagesHas(repo, ver))
		mcMark := mark(checkMavenCentralHas(repo, ver))
		if tagMark == "✓" {
			tagOK++
		}
		if ghMark == "✓" {
			ghOK++
		}
		if mcMark == "✓" {
			mcOK++
		}
		fmt.Printf("  %-50s %-12s %-12s %-12s\n", repo, tagMark, ghMark, mcMark)
	}
	fmt.Println("  " + strings.Repeat("─", 92))
	fmt.Printf("  %-50s %d/%-9d %d/%-9d %d/%-9d\n", "Total", tagOK, len(setup.FrameworkRepos), ghOK, len(setup.FrameworkRepos), mcOK, len(setup.FrameworkRepos))
	return nil
}

func mark(ok bool) string {
	if ok {
		return ui.StyleSuccess.Render("✓")
	}
	return "·"
}

func checkTagExists(repo, tag string) bool {
	out, err := runGH("", "api", "/repos/fireflyframework/"+repo+"/git/refs/tags/"+tag, "-q", ".object.sha")
	if err != nil {
		return false
	}
	s := strings.TrimSpace(out)
	return s != "" && !strings.Contains(out, "Not Found")
}

func checkGHPackagesHas(repo, version string) bool {
	out, err := runGH("", "api",
		"/orgs/fireflyframework/packages/maven/org.fireflyframework."+repo+"/versions",
		"-q", ".[] | select(.name==\""+version+"\") | .name")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == version
}

func checkMavenCentralHas(repo, version string) bool {
	// Public Maven Central search API — does not require auth
	cmd := exec.Command("curl", "-sf", "-o", "/dev/null", "-w", "%{http_code}",
		"https://repo.maven.apache.org/maven2/org/fireflyframework/"+repo+"/"+version+"/"+repo+"-"+version+".pom")
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out)) == "200"
}

// ── flywork release publish-mvn-central ─────────────────────────────────────
// Triggers the per-repo maven-central.yml workflow via workflow_dispatch.
// Useful when Sonatype polling timed out and the deploy bundle was uploaded
// but not validated — retrying is enough since the bundle is already there.

var mvnCentralVersion string
var mvnCentralRepoOverride string
var mvnCentralWaitTime int

var releaseMvnCentralCmd = &cobra.Command{
	Use:   "publish-mvn-central",
	Short: "Trigger Maven Central publish workflow (workflow_dispatch) for repos missing on Central",
	RunE:  runReleasePublishMvnCentral,
}

func runReleasePublishMvnCentral(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ver := mvnCentralVersion
	if ver == "" {
		ver = cfg.ParentVersion
	}

	p.Header("Maven Central publish @ " + ver)
	p.Newline()

	repos := setup.FrameworkRepos
	if mvnCentralRepoOverride != "" {
		repos = []string{mvnCentralRepoOverride}
	}

	dispatched, skipped, failed := 0, 0, 0
	for _, repo := range repos {
		if !isPartOfBuild(repo) {
			continue
		}
		if checkMavenCentralHas(repo, ver) {
			p.Info(fmt.Sprintf("%-50s already on Maven Central", repo))
			skipped++
			continue
		}
		args := []string{"workflow", "run", "maven-central.yml",
			"--repo", "fireflyframework/" + repo,
			"--ref", "v" + ver,
			"-f", fmt.Sprintf("wait-max-time=%d", mvnCentralWaitTime),
		}
		out, err := runGH("", args...)
		if err != nil {
			p.Warning(fmt.Sprintf("%-50s dispatch failed: %s", repo, strings.TrimSpace(out)))
			failed++
			continue
		}
		p.Success(fmt.Sprintf("%-50s dispatched", repo))
		dispatched++
	}
	p.SummaryBox("publish-mvn-central", []string{
		fmt.Sprintf("Dispatched   %d", dispatched),
		fmt.Sprintf("Already on   %d", skipped),
		fmt.Sprintf("Failed       %d", failed),
	})
	return nil
}

// ── init ─────────────────────────────────────────────────────────────────────

func init() {
	releaseCmd.Flags().IntVar(&releaseTargetYear, "year", 0, "CalVer year (YY)")
	releaseCmd.Flags().IntVar(&releaseTargetMonth, "month", 0, "CalVer month (MM)")
	releaseCmd.Flags().IntVar(&releaseTargetPatch, "patch", 0, "CalVer patch (PP)")
	releaseCmd.Flags().BoolVar(&releaseDryRun, "dry-run", false, "Print plan without making changes")
	releaseCmd.Flags().BoolVar(&releaseSkipPRs, "skip-prs", false, "Push directly to main (skip PR step)")
	releaseCmd.Flags().BoolVar(&releaseSkipTag, "skip-tag", false, "Do not tag main (skip release trigger)")
	releaseCmd.Flags().IntVar(&releaseFromLayer, "from-layer", 0, "Resume cascade starting at this layer (0-indexed)")
	releaseCmd.Flags().BoolVar(&releaseAdminMerge, "admin", true, "Use --admin to bypass branch protection / required reviews")
	releaseCmd.Flags().IntVar(&releasePerRepoTimeout, "timeout-minutes", 30, "Per-layer timeout in minutes for the github-packages wait")
	releaseCmd.Flags().StringVar(&releaseRepoOverride, "repo", "", "Limit to a single repo (still respects DAG order)")

	releaseVerifyCmd.Flags().StringVar(&verifyVersion, "version", "", "Version to verify (defaults to config.ParentVersion)")

	releaseMvnCentralCmd.Flags().StringVar(&mvnCentralVersion, "version", "", "Version to publish (defaults to config.ParentVersion)")
	releaseMvnCentralCmd.Flags().StringVar(&mvnCentralRepoOverride, "repo", "", "Limit to a single repo")
	releaseMvnCentralCmd.Flags().IntVar(&mvnCentralWaitTime, "wait-seconds", 3600, "Sonatype Central polling wait time (seconds)")

	releaseCmd.AddCommand(releaseStatusCmd)
	releaseCmd.AddCommand(releaseVerifyCmd)
	releaseCmd.AddCommand(releaseMvnCentralCmd)
	rootCmd.AddCommand(releaseCmd)
}
