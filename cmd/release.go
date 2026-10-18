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
	Long: `Prepare the Jafly release train with its version tool, open a release PR,
wait for its merge, and tag the final commit. The exact tag and commit's release
workflow must verify GitHub Packages and Maven Central before completion.

Requires Python 3, authenticated gh, a clean Jafly checkout, and release credentials.
Use --dry-run to print the plan without changing local or remote state.
Branch protection is respected unless --admin is explicitly supplied.

Examples:
  flywork release --year 26 --month 10 --patch 17
  flywork release --dry-run
  flywork release --skip-tag
  flywork release verify --version 26.10.17`,
	RunE: runRelease,
}

func resolveJaflyReleaseTarget(current string) (version.CalVer, error) {
	parsed, err := version.Parse(strings.TrimSuffix(current, "-SNAPSHOT"))
	if err != nil {
		return version.CalVer{}, err
	}
	if strings.HasSuffix(current, "-SNAPSHOT") {
		return parsed, nil
	}
	return version.Next(parsed), nil
}

func runRelease(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	overallStart := time.Now()
	if releaseRepoOverride != "" && releaseRepoOverride != "fireflyframework-jafly" {
		return fmt.Errorf("Java releases use fireflyframework-jafly")
	}
	if releaseFromLayer != 0 {
		return fmt.Errorf("Jafly has one repository layer; --from-layer must be 0")
	}

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
	parentPom := filepath.Join(cfg.ReposPath, "fireflyframework-jafly", "build", "parent", "pom.xml")
	currentVer := cfg.ParentVersion
	if detected, err := version.ReadPomVersion(parentPom); err == nil {
		currentVer = detected
	}
	var target version.CalVer
	if releaseTargetYear > 0 || releaseTargetMonth > 0 || releaseTargetPatch > 0 {
		target = version.CalVer{Year: releaseTargetYear, Month: releaseTargetMonth, Patch: releaseTargetPatch}
	} else {
		target, err = resolveJaflyReleaseTarget(currentVer)
		if err != nil {
			return fmt.Errorf("resolve release target: %w", err)
		}
	}
	targetStr := target.String()
	if _, err := version.Parse(targetStr); err != nil {
		return fmt.Errorf("invalid release target: %w", err)
	}
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

	if releaseDryRun {
		p.Info("Dry run complete: no branches, tags, workflows, or release records changed")
		return nil
	}

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
				return fmt.Errorf("%s: %w", repo, err)
			}
		}

		if releaseSkipTag {
			p.Info("Release branch prepared; tagging and publication were skipped")
			return nil
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
		return err
	}
	if !report.Consistent || report.TotalWithPom != 1 || len(report.UniqueVersions) != 1 || report.UniqueVersions[targetStr] == 0 {
		return fmt.Errorf("release checkout versions do not match %s: %v", targetStr, report.UniqueVersions)
	}
	cfg.ParentVersion = targetStr
	if err := cfg.Save(); err != nil {
		p.Warning("Release succeeded; could not save CLI config: " + err.Error())
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

func checkoutReleaseBranch(repoDir, branch string) error {
	command := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	command.Dir = repoDir
	if command.Run() == nil {
		return git.RunCmd(repoDir, "checkout", branch)
	}
	return git.RunCmd(repoDir, "checkout", "-b", branch)
}

func prepareJaflyReleaseVersion(reposDir, target string) error {
	repoDir := filepath.Join(reposDir, "fireflyframework-jafly")
	dirty, err := git.IsDirty(repoDir)
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("working tree is not clean: %s", repoDir)
	}
	results, err := version.BumpAll(version.BumpOptions{ReposDir: reposDir, NewVersion: target}, nil)
	if err != nil {
		return err
	}
	if len(results) != 1 {
		return fmt.Errorf("expected one Java checkout, found %d", len(results))
	}
	result := results[0]
	if result.Error != nil {
		return result.Error
	}
	if result.Updated == 0 {
		return nil
	}
	args := []string{"add", "--"}
	for _, pom := range version.FindAllPoms(repoDir) {
		relative, err := filepath.Rel(repoDir, pom)
		if err != nil {
			return err
		}
		args = append(args, relative)
	}
	if err := git.RunCmd(repoDir, args...); err != nil {
		return err
	}
	return git.Commit(repoDir, "release: prepare "+target)
}

func processRepoForRelease(p *ui.Printer, reposDir, repo, releaseBranch, targetStr string) error {
	repoDir := filepath.Join(reposDir, repo)
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		return fmt.Errorf("not cloned")
	}

	if releaseDryRun {
		p.Info(repo + " [dry-run] would create " + releaseBranch + ", push, and open a PR")
		return nil
	}

	dirty, err := git.IsDirty(repoDir)
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("working tree is not clean: %s", repoDir)
	}
	if err := checkoutReleaseBranch(repoDir, releaseBranch); err != nil {
		return fmt.Errorf("checkout: %w", err)
	}

	if err := prepareJaflyReleaseVersion(reposDir, targetStr); err != nil {
		return err
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
		return git.RunCmd(repoDir, "push", "origin", "HEAD:main")
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

// waitForLayerRelease tags the merged release commit, then waits for the exact
// tag's complete publication workflow, including both registries.
func waitForLayerRelease(p *ui.Printer, layer []string, tag string) error {
	for _, repo := range layer {
		if !isPartOfBuild(repo) {
			continue
		}
		if err := mergeAndTagRepo(p, repo, tag); err != nil {
			return fmt.Errorf("%s tag: %w", repo, err)
		}
	}
	deadline := time.Now().Add(time.Duration(releasePerRepoTimeout) * time.Minute)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for layer release")
		}
		allDone := true
		for _, repo := range layer {
			if !isPartOfBuild(repo) {
				continue
			}
			sha, err := git.HeadSHA(filepath.Join(cfgRepoPath(), repo))
			if err != nil {
				return err
			}
			conc, err := ghPackagesStatus(repo, tag, sha)
			if err != nil {
				return err
			}
			if conc != "" && conc != "success" {
				return fmt.Errorf("%s release failed: %s", repo, conc)
			}
			if conc != "success" {
				allDone = false
				break
			}
		}
		if allDone {
			p.Success("Release workflow completed, including GitHub Packages and Maven Central")
			return nil
		}
		time.Sleep(30 * time.Second)
	}
}

// mergeAndTagRepo merges the open PR for the repo (if any), pulls main, and
// pushes a new annotated tag. Uses --admin to bypass branch protection when
// the gh token has admin rights on the repo.
func mergeAndTagRepo(p *ui.Printer, repo, tag string) error {
	if releaseDryRun {
		return nil
	}
	repoDir := filepath.Join(cfgRepoPath(), repo)
	if number := findOpenPRNumber(repoDir, "release/"+strings.TrimPrefix(tag, "v")); number != 0 {
		args := []string{"pr", "merge", fmt.Sprint(number), "--squash", "--delete-branch", "--auto"}
		if releaseAdminMerge {
			args = append(args, "--admin")
		}
		if out, err := runGH(repoDir, args...); err != nil {
			return fmt.Errorf("merge release PR: %w: %s", err, strings.TrimSpace(out))
		}
		deadline := time.Now().Add(time.Duration(releasePerRepoTimeout) * time.Minute)
		for {
			state, err := runGH(repoDir, "pr", "view", fmt.Sprint(number), "--json", "state", "--jq", ".state")
			if err != nil {
				return err
			}
			if strings.TrimSpace(state) == "MERGED" {
				break
			}
			if strings.TrimSpace(state) != "OPEN" {
				return fmt.Errorf("release PR is %s", strings.TrimSpace(state))
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for release PR")
			}
			time.Sleep(15 * time.Second)
		}
	}
	if err := git.RunCmd(repoDir, "checkout", "main"); err != nil {
		return err
	}
	if err := git.RunCmd(repoDir, "pull", "--ff-only"); err != nil {
		return err
	}
	target := strings.TrimPrefix(tag, "v")
	actual, err := version.ReadPomVersion(filepath.Join(repoDir, "build", "parent", "pom.xml"))
	if err != nil || actual != target {
		return fmt.Errorf("main framework version %q does not match tag %s: %v", actual, tag, err)
	}
	sha, err := git.HeadSHA(repoDir)
	if err != nil {
		return err
	}
	publishedMain := exec.Command("git", "rev-parse", "--verify", "refs/remotes/origin/main^{commit}")
	publishedMain.Dir = repoDir
	publishedSHA, err := publishedMain.Output()
	if err != nil {
		return fmt.Errorf("resolve published origin/main: %w", err)
	}
	if strings.TrimSpace(string(publishedSHA)) != sha {
		return fmt.Errorf("local main differs from published origin/main; synchronize main before tagging")
	}
	resolve := exec.Command("git", "rev-parse", "--verify", tag+"^{commit}")
	resolve.Dir = repoDir
	existing, resolveErr := resolve.Output()
	if resolveErr == nil {
		if strings.TrimSpace(string(existing)) != sha {
			return fmt.Errorf("existing tag %s points to another commit", tag)
		}
	} else if err := git.Tag(repoDir, tag); err != nil {
		return err
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

// ghPackagesStatus checks the entire destination release workflow at the exact
// tag and commit; a prior release or a branch build cannot satisfy this gate.
func ghPackagesStatus(repo, tag, sha string) (string, error) {
	out, err := runGH(filepath.Join(cfgRepoPath(), repo), "run", "list",
		"--workflow=release.yml", "--commit", sha, "--limit", "50",
		"--json", "headSha,headBranch,status,conclusion")
	if err != nil {
		return "", err
	}
	return releaseConclusion([]byte(out), tag, sha)
}

func releaseConclusion(payload []byte, tag, sha string) (string, error) {
	var runs []struct {
		HeadSHA    string `json:"headSha"`
		HeadBranch string `json:"headBranch"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}
	if err := json.Unmarshal(payload, &runs); err != nil {
		return "", err
	}
	for _, run := range runs {
		if run.HeadSHA == sha && run.HeadBranch == tag {
			if run.Status != "completed" {
				return "", nil
			}
			return run.Conclusion, nil
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
	Short: "Verify every exact release artifact against its publication SHA256",
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
	if parsed, err := version.Parse(ver); err != nil || parsed.String() != ver {
		return fmt.Errorf("a final padded CalVer is required: %q", ver)
	}
	repository := cfg.GithubOrg + "/fireflyframework-jafly"
	tag := "v" + ver
	commit, err := resolveReleaseTagCommit(repository, tag, func(endpoint string) ([]byte, error) {
		out, err := runGH("", "api", endpoint)
		return []byte(out), err
	})
	if err != nil {
		return err
	}
	temp, err := os.MkdirTemp("", "flywork-release-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	download := []string{"release", "download", tag, "--repo", repository, "--dir", temp}
	for _, name := range []string{"reactor-inventory.json", "github-manifest.json", "release-manifest.json", "release-commit.txt"} {
		download = append(download, "--pattern", name)
	}
	if out, err := runGH("", download...); err != nil {
		return fmt.Errorf("download exact release manifests: %w: %s", err, strings.TrimSpace(out))
	}
	read := func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(temp, name)) }
	commitEvidence, err := read("release-commit.txt")
	if err != nil {
		return err
	}
	if err := validateReleaseCommit(commitEvidence, commit); err != nil {
		return err
	}
	inventory, err := read("reactor-inventory.json")
	if err != nil {
		return err
	}
	githubManifest, err := read("github-manifest.json")
	if err != nil {
		return err
	}
	centralManifest, err := read("release-manifest.json")
	if err != nil {
		return err
	}
	repoDir := filepath.Join(temp, "source")
	if err := cloneReleaseSource("https://github.com/"+repository+".git", tag, commit, repoDir); err != nil {
		return err
	}
	script := filepath.Join(repoDir, "scripts", "release_tools.py")
	sourceInventoryPath := filepath.Join(temp, "source-inventory.json")
	check := exec.Command("python3", script, "check", "--tag", tag, "--output", sourceInventoryPath)
	check.Dir = repoDir
	if out, err := check.CombinedOutput(); err != nil {
		return fmt.Errorf("read exact tagged reactor: %w: %s", err, strings.TrimSpace(string(out)))
	}
	sourceInventory, err := os.ReadFile(sourceInventoryPath)
	if err != nil {
		return err
	}
	if err := validateReleaseInventory(inventory, sourceInventory, githubManifest, centralManifest, ver); err != nil {
		return err
	}
	for _, target := range []struct {
		manifest string
		url      string
		auth     bool
	}{
		{"github-manifest.json", "https://maven.pkg.github.com/" + cfg.GithubOrg + "/fireflyframework-jafly", true},
		{"release-manifest.json", "https://repo.maven.apache.org/maven2", false},
	} {
		args := []string{script, "verify", "--manifest", filepath.Join(temp, target.manifest), "--url", target.url, "--attempts", "1", "--delay", "0"}
		if target.auth {
			args = append(args, "--username-env", "GITHUB_ACTOR", "--token-env", "GITHUB_TOKEN")
		}
		command := exec.Command("python3", args...)
		command.Dir = repoDir
		if out, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("verify %s: %w: %s", target.url, err, strings.TrimSpace(string(out)))
		}
	}
	p.Success("Every artifact in the exact release inventory matches its SHA256 on GitHub Packages and Maven Central")
	return nil
}

// ── flywork release publish-mvn-central ─────────────────────────────────────
// Dispatches the tagged unified release workflow. Partial publication requires
// inspection because an immutable registry version cannot be overwritten.

var mvnCentralVersion string
var mvnCentralRepoOverride string
var mvnCentralWaitTime int

var releaseMvnCentralCmd = &cobra.Command{
	Use:   "publish-mvn-central",
	Short: "Dispatch the combined publication workflow at an existing final tag",
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
	if parsed, err := version.Parse(ver); err != nil || parsed.String() != ver {
		return fmt.Errorf("a final padded CalVer is required: %q", ver)
	}
	if mvnCentralRepoOverride != "" && mvnCentralRepoOverride != "fireflyframework-jafly" {
		return fmt.Errorf("Java publication uses fireflyframework-jafly")
	}
	tag := "v" + ver
	out, err := runGH("", "workflow", "run", "release.yml", "--repo", cfg.GithubOrg+"/fireflyframework-jafly", "--ref", tag, "-f", "tag="+tag)
	if err != nil {
		return fmt.Errorf("dispatch exact tagged release: %w: %s", err, strings.TrimSpace(out))
	}
	p.Info("Dispatched the combined GitHub Packages and Maven Central workflow at " + tag + "; completion is verified by the release workflow")
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
	releaseCmd.Flags().BoolVar(&releaseAdminMerge, "admin", false, "Use --admin to bypass branch protection / required reviews")
	releaseCmd.Flags().IntVar(&releasePerRepoTimeout, "timeout-minutes", 180, "Timeout in minutes for the release PR and, separately, the complete publication workflow")
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
