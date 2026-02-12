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
	"github.com/fireflyframework/fireflyframework-cli/internal/git"
	"github.com/fireflyframework/fireflyframework-cli/internal/maven"
	"github.com/fireflyframework/fireflyframework-cli/internal/setup"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
	"github.com/fireflyframework/fireflyframework-cli/internal/version"
	"github.com/spf13/cobra"
)

// ── Parent command ───────────────────────────────────────────────────────────

var fwversionCmd = &cobra.Command{
	Use:   "fwversion",
	Short: "Manage framework-wide CalVer versions",
	Long: `Show, bump, check, and track CalVer versions across all Firefly Framework
repositories. The framework uses Calendar Versioning (CalVer) in the format
YY.MM.PP (e.g. 26.02.02).

Available Subcommands:
  show       Show current framework version across all repos
  bump       Bump framework version across all repos (updates pom.xml files)
  check      Validate version consistency across all repos
  families   Show version family release history

Examples:
  flywork fwversion show
  flywork fwversion bump --auto
  flywork fwversion bump --auto --push
  flywork fwversion bump --dry-run
  flywork fwversion check
  flywork fwversion families`,
}

// ── fwversion show ──────────────────────────────────────────────────────────

var fwversionShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current framework version across all repos",
	Long: `Displays the current POM version for each framework repository, highlights
mismatches against the configured target version, and reports dirty working
trees. Use -v for a detailed per-repository listing with version, git tag,
and dirty status.`,
	RunE: runFwversionShow,
}

func runFwversionShow(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	p.Header("Framework Version Status")
	p.Newline()
	p.KeyValue("Config version", cfg.ParentVersion)
	p.Newline()

	report, err := version.CheckAll(cfg.ReposPath)
	if err != nil {
		return fmt.Errorf("version check failed: %w", err)
	}

	// Summary counts
	atTarget := 0
	mismatched := 0
	missing := 0
	dirty := 0

	for _, rs := range report.Repos {
		if !rs.Exists {
			missing++
			continue
		}
		if !rs.HasPom {
			continue
		}
		if rs.PomVersion == cfg.ParentVersion {
			atTarget++
		} else {
			mismatched++
		}
		if rs.Dirty {
			dirty++
		}
	}

	p.KeyValue("Repos at target", fmt.Sprintf("%d/%d", atTarget, report.TotalWithPom))
	if mismatched > 0 {
		p.KeyValue("Version mismatch", fmt.Sprintf("%d", mismatched))
	}
	if missing > 0 {
		p.KeyValue("Not cloned", fmt.Sprintf("%d", missing))
	}
	if dirty > 0 {
		p.KeyValue("Dirty trees", fmt.Sprintf("%d", dirty))
	}

	if len(report.UniqueVersions) > 1 {
		p.Newline()
		p.Warning("Multiple versions detected:")
		for ver, count := range report.UniqueVersions {
			p.Info(fmt.Sprintf("  %s (%d repos)", ver, count))
		}
	} else if report.Consistent && report.TotalWithPom > 0 {
		p.Newline()
		for ver := range report.UniqueVersions {
			p.Success(fmt.Sprintf("All repos consistent at %s", ver))
		}
	}

	// Verbose per-repo listing
	if verbose {
		p.Newline()
		p.Header("Per-Repository Details")
		p.Newline()
		for _, rs := range report.Repos {
			if !rs.Exists {
				p.Info(fmt.Sprintf("%-45s %s", rs.Repo, ui.StyleMuted.Render("not cloned")))
				continue
			}
			if !rs.HasPom {
				p.Info(fmt.Sprintf("%-45s %s", rs.Repo, ui.StyleMuted.Render("no pom.xml")))
				continue
			}

			verStr := rs.PomVersion
			if rs.PomVersion != cfg.ParentVersion {
				verStr = ui.StyleWarning.Render(rs.PomVersion)
			} else {
				verStr = ui.StyleSuccess.Render(rs.PomVersion)
			}

			tagStr := ""
			if rs.GitTag != "" {
				tagStr = ui.StyleMuted.Render(" tag=" + rs.GitTag)
			}
			dirtyStr := ""
			if rs.Dirty {
				dirtyStr = ui.StyleWarning.Render(" [dirty]")
			}

			fmt.Printf("  %-45s %s%s%s\n", rs.Repo, verStr, tagStr, dirtyStr)
		}
	}

	return nil
}

// ── fwversion bump ──────────────────────────────────────────────────────────

var (
	bumpYear    int
	bumpMonth   int
	bumpPatch   int
	bumpAuto    bool
	bumpCommit  bool
	bumpTag     bool
	bumpPush    bool
	bumpDryRun  bool
	bumpInstall bool
)

var fwversionBumpCmd = &cobra.Command{
	Use:   "bump",
	Short: "Bump framework version across all repos",
	Long: `Updates all pom.xml files across every framework repository to a new CalVer
version, and optionally commits, tags, and pushes the changes.

By default the CLI auto-increments the patch number from the current version.
Use --auto to explicitly request auto-computation. Use --year, --month, and
--patch to set a specific version manually.

The bump process:
  1. Detects the current version from the parent POM
  2. Computes or accepts the target version
  3. Updates all pom.xml files across every cloned repository
  4. Updates the GenAI module version files (if present)
  5. Optionally commits changes (--commit, default: true)
  6. Optionally tags each repo with v<version> (--tag, default: true)
  7. Optionally pushes to remote (--push, default: false)
  8. Optionally runs mvn install after bumping (--install)
  9. Records a version family snapshot for history tracking
  10. Updates ~/.flywork/config.yaml with the new parent_version

Examples:
  flywork fwversion bump                Auto-increment patch version
  flywork fwversion bump --auto         Explicitly auto-compute next CalVer
  flywork fwversion bump --auto --push  Bump, commit, tag, and push
  flywork fwversion bump --dry-run      Preview changes without modifying files
  flywork fwversion bump --install      Bump + run mvn install after
  flywork fwversion bump --year 26 --month 2 --patch 1  Set explicit version`,
	RunE: runFwversionBump,
}

func runFwversionBump(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	overallStart := time.Now()

	// ── Phase 1: Preflight ──────────────────────────────────────────────
	p.StageHeader(1, "Preflight")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if _, err := os.Stat(cfg.ReposPath); os.IsNotExist(err) {
		return fmt.Errorf("repos directory not found: %s (run 'flywork setup' first)", cfg.ReposPath)
	}

	// Detect current version from parent POM
	parentPom := filepath.Join(cfg.ReposPath, "fireflyframework-parent", "pom.xml")
	oldVer := cfg.ParentVersion
	if detected, err := version.ReadPomVersion(parentPom); err == nil {
		oldVer = detected
	}
	p.KeyValue("Current version", oldVer)

	// ── Phase 2: Target resolution ──────────────────────────────────────
	var target version.CalVer
	if bumpAuto {
		current, err := version.Parse(oldVer)
		if err != nil {
			current = version.Current()
		}
		target = version.Next(current)
	} else if bumpYear > 0 || bumpMonth > 0 || bumpPatch > 0 {
		target = version.CalVer{Year: bumpYear, Month: bumpMonth, Patch: bumpPatch}
	} else {
		// Default: try parsing current and auto-increment
		current, err := version.Parse(oldVer)
		if err != nil {
			target = version.Current()
		} else {
			target = version.Next(current)
		}
	}

	newVer := target.String()
	p.KeyValue("Target version", newVer)

	if bumpDryRun {
		p.Info("DRY RUN — no files will be modified")
	}

	// ── Phase 3: Confirmation ───────────────────────────────────────────
	p.Newline()
	p.Info(fmt.Sprintf("Version change: %s → %s", ui.StyleWarning.Render(oldVer), ui.StyleSuccess.Render(newVer)))
	p.Newline()

	if !bumpDryRun {
		if !ui.Confirm("Proceed with version bump?", true) {
			p.Warning("Aborted.")
			return nil
		}
	}

	// ── Phase 4: POM updates ────────────────────────────────────────────
	p.StageHeader(2, "Updating POM Files")

	totalRepos := len(setup.FrameworkRepos)
	pomBar := ui.NewProgressBar(totalRepos, "repos")
	totalFiles := 0
	totalUpdated := 0
	repoErrors := 0

	results, err := version.BumpAll(version.BumpOptions{
		ReposDir:   cfg.ReposPath,
		OldVersion: oldVer,
		NewVersion: newVer,
		DoCommit:   bumpCommit && !bumpDryRun,
		DoTag:      bumpTag && !bumpDryRun,
		DoPush:     bumpPush && !bumpDryRun,
		DryRun:     bumpDryRun,
	}, func(idx, total int, r version.RepoResult) {
		totalFiles += r.FilesFound
		totalUpdated += r.Updated
		if r.Error != nil {
			repoErrors++
			p.Error(fmt.Sprintf("%-45s %s", r.Repo, r.Error))
		} else if verbose && r.Updated > 0 {
			p.Success(fmt.Sprintf("%-45s %d files", r.Repo, r.Updated))
		}
		pomBar.Increment()
	})
	if err != nil {
		return err
	}
	pomBar.Finish()

	p.Newline()
	p.Info(fmt.Sprintf("POM files: %d found, %d updated across %d repos", totalFiles, totalUpdated, totalRepos))

	// ── Phase 5: GenAI update ───────────────────────────────────────────
	genaiDir := filepath.Join(cfg.ReposPath, "fireflyframework-genai")
	if _, err := os.Stat(genaiDir); err == nil {
		p.Newline()
		spinner := ui.NewSpinner("Updating GenAI module...")
		spinner.Start()
		genaiErr := version.BumpGenAI(genaiDir, oldVer, newVer, bumpDryRun)
		spinner.Stop(genaiErr == nil)
		if genaiErr != nil {
			p.Warning("GenAI update: " + genaiErr.Error())
		}
	}

	// ── Phase 6: Optional install ───────────────────────────────────────
	if bumpInstall && !bumpDryRun {
		p.StageHeader(3, "Maven Install")
		installBar := ui.NewProgressBar(totalRepos, "installed")
		installFailed := 0

		for _, repo := range setup.FrameworkRepos {
			repoDir := filepath.Join(cfg.ReposPath, repo)
			pomPath := filepath.Join(repoDir, "pom.xml")
			if _, err := os.Stat(pomPath); os.IsNotExist(err) {
				installBar.Increment()
				continue
			}

			spinner := ui.NewSpinner(fmt.Sprintf("Installing %s...", repo))
			spinner.Start()
			installErr := maven.InstallQuiet(repoDir, true)
			spinner.Stop(installErr == nil)
			if installErr != nil {
				installFailed++
				p.Error(fmt.Sprintf("%-45s install failed", repo))
			}
			installBar.Increment()
		}

		installBar.Finish()
		if installFailed > 0 {
			p.Warning(fmt.Sprintf("Install: %d repos failed", installFailed))
		}
	}

	// ── Phase 7: Config update ──────────────────────────────────────────
	if !bumpDryRun {
		cfg.ParentVersion = newVer
		if err := cfg.Save(); err != nil {
			p.Warning("Could not save config: " + err.Error())
		}
	}

	// ── Phase 8: Family recording ───────────────────────────────────────
	if !bumpDryRun {
		families, err := version.LoadFamilies()
		if err != nil {
			p.Warning("Could not load version families: " + err.Error())
		} else {
			modules := make(map[string]string)
			for _, r := range results {
				if r.Updated > 0 {
					repoDir := filepath.Join(cfg.ReposPath, r.Repo)
					if sha, err := git.HeadCommit(repoDir); err == nil {
						modules[r.Repo] = sha
					}
				}
			}
			families.Record(newVer, modules)
			if err := families.Save(); err != nil {
				p.Warning("Could not save version families: " + err.Error())
			}
		}
	}

	// ── Summary ─────────────────────────────────────────────────────────
	elapsed := time.Since(overallStart).Truncate(time.Second)

	status := "Version Bump Complete"
	if bumpDryRun {
		status = "Dry Run Complete"
	}
	if repoErrors > 0 {
		status = "Version Bump Incomplete"
	}

	summaryLines := []string{
		fmt.Sprintf("Version       %s → %s", oldVer, newVer),
		fmt.Sprintf("POM files     %d updated", totalUpdated),
		fmt.Sprintf("Repositories  %d processed", totalRepos),
	}
	if bumpCommit && !bumpDryRun {
		committed := 0
		for _, r := range results {
			if r.Committed {
				committed++
			}
		}
		summaryLines = append(summaryLines, fmt.Sprintf("Committed     %d repos", committed))
	}
	if bumpTag && !bumpDryRun {
		tagged := 0
		for _, r := range results {
			if r.Tagged {
				tagged++
			}
		}
		summaryLines = append(summaryLines, fmt.Sprintf("Tagged        %d repos (v%s)", tagged, newVer))
	}
	if repoErrors > 0 {
		summaryLines = append(summaryLines, fmt.Sprintf("Errors        %d repos", repoErrors))
	}
	summaryLines = append(summaryLines, fmt.Sprintf("Total time    %s", elapsed))

	p.SummaryBox(status, summaryLines)
	return nil
}

// ── fwversion check ─────────────────────────────────────────────────────────

var fwversionCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate version consistency across all repos",
	Long: `Runs a comprehensive set of consistency checks across all framework
repositories:

  - POM version consistency: all repos should be at the same version
  - Config matches repos: ~/.flywork/config.yaml parent_version matches actual POMs
  - Git tags: each repo's latest tag should match its POM version (v<version>)
  - Clean working trees: no uncommitted changes in any repository
  - All repos cloned: verifies all expected repositories exist
  - Parent POM in .m2: the parent POM artifact is installed at the target version
  - BOM in .m2: the BOM artifact is installed at the target version

Each check reports pass, warn, or fail with a detail message.`,
	RunE: runFwversionCheck,
}

func runFwversionCheck(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	p.Header("Version Consistency Check")
	p.Newline()

	report, err := version.CheckAll(cfg.ReposPath)
	if err != nil {
		return fmt.Errorf("check failed: %w", err)
	}

	var results []ui.CheckResult

	// Check: all poms same version
	if report.Consistent && report.TotalWithPom > 0 {
		var ver string
		for v := range report.UniqueVersions {
			ver = v
		}
		results = append(results, ui.CheckResult{
			Name:   "POM version consistency",
			Status: "pass",
			Detail: fmt.Sprintf("all %d repos at %s", report.TotalWithPom, ver),
		})
	} else if report.TotalWithPom > 0 {
		detail := fmt.Sprintf("%d unique versions:", len(report.UniqueVersions))
		for ver, count := range report.UniqueVersions {
			detail += fmt.Sprintf(" %s(%d)", ver, count)
		}
		results = append(results, ui.CheckResult{
			Name:   "POM version consistency",
			Status: "fail",
			Detail: detail,
		})
	}

	// Check: config matches detected
	configMatch := false
	for ver := range report.UniqueVersions {
		if ver == cfg.ParentVersion {
			configMatch = true
		}
	}
	if configMatch || report.TotalWithPom == 0 {
		results = append(results, ui.CheckResult{
			Name:   "Config matches repos",
			Status: "pass",
			Detail: cfg.ParentVersion,
		})
	} else {
		results = append(results, ui.CheckResult{
			Name:   "Config matches repos",
			Status: "warn",
			Detail: fmt.Sprintf("config=%s, repos use different versions", cfg.ParentVersion),
		})
	}

	// Check: git tags match
	tagMismatch := 0
	tagMissing := 0
	for _, rs := range report.Repos {
		if !rs.Exists || !rs.HasPom {
			continue
		}
		expectedTag := "v" + rs.PomVersion
		if rs.GitTag == "" {
			tagMissing++
		} else if rs.GitTag != expectedTag {
			tagMismatch++
		}
	}
	if tagMismatch == 0 && tagMissing == 0 {
		results = append(results, ui.CheckResult{Name: "Git tags", Status: "pass"})
	} else {
		detail := ""
		if tagMismatch > 0 {
			detail += fmt.Sprintf("%d mismatched", tagMismatch)
		}
		if tagMissing > 0 {
			if detail != "" {
				detail += ", "
			}
			detail += fmt.Sprintf("%d missing", tagMissing)
		}
		results = append(results, ui.CheckResult{Name: "Git tags", Status: "warn", Detail: detail})
	}

	// Check: dirty trees
	dirtyCount := 0
	for _, rs := range report.Repos {
		if rs.Dirty {
			dirtyCount++
		}
	}
	if dirtyCount == 0 {
		results = append(results, ui.CheckResult{Name: "Clean working trees", Status: "pass"})
	} else {
		results = append(results, ui.CheckResult{
			Name:   "Clean working trees",
			Status: "warn",
			Detail: fmt.Sprintf("%d repos have uncommitted changes", dirtyCount),
		})
	}

	// Check: not-cloned repos
	notCloned := 0
	for _, rs := range report.Repos {
		if !rs.Exists {
			notCloned++
		}
	}
	if notCloned == 0 {
		results = append(results, ui.CheckResult{
			Name:   "All repos cloned",
			Status: "pass",
			Detail: fmt.Sprintf("%d/%d", report.TotalRepos, report.TotalRepos),
		})
	} else {
		results = append(results, ui.CheckResult{
			Name:   "All repos cloned",
			Status: "warn",
			Detail: fmt.Sprintf("%d/%d missing", notCloned, report.TotalRepos),
		})
	}

	// Check: parent POM in .m2 with current version
	if maven.ArtifactExistsInM2("org.fireflyframework", "fireflyframework-parent", cfg.ParentVersion) {
		results = append(results, ui.CheckResult{
			Name:   "Parent POM in .m2",
			Status: "pass",
			Detail: cfg.ParentVersion,
		})
	} else {
		results = append(results, ui.CheckResult{
			Name:   "Parent POM in .m2",
			Status: "fail",
			Detail: fmt.Sprintf("version %s not found", cfg.ParentVersion),
		})
	}

	// Check: BOM in .m2 with current version
	if maven.ArtifactExistsInM2("org.fireflyframework", "fireflyframework-bom", cfg.ParentVersion) {
		results = append(results, ui.CheckResult{
			Name:   "BOM in .m2",
			Status: "pass",
			Detail: cfg.ParentVersion,
		})
	} else {
		results = append(results, ui.CheckResult{
			Name:   "BOM in .m2",
			Status: "fail",
			Detail: fmt.Sprintf("version %s not found", cfg.ParentVersion),
		})
	}

	p.PrintChecks(results)

	// Summary
	pass, fail, warn := 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case "pass":
			pass++
		case "fail":
			fail++
		case "warn":
			warn++
		}
	}

	p.Newline()
	summary := fmt.Sprintf("%d passed", pass)
	if warn > 0 {
		summary += fmt.Sprintf(", %d warnings", warn)
	}
	if fail > 0 {
		summary += fmt.Sprintf(", %d failed", fail)
		p.Error("Version check: " + summary)
	} else if warn > 0 {
		p.Warning("Version check: " + summary)
	} else {
		p.Success("Version check: " + summary)
	}

	return nil
}

// ── fwversion families ──────────────────────────────────────────────────────

var fwversionFamiliesCmd = &cobra.Command{
	Use:   "families",
	Short: "Show version family history",
	Long: `Shows the history of version bumps recorded in ~/.flywork/version-families.json.
Each entry includes the version string, release date, and the number of modules
that were updated. The most recent version is marked with '*'.

Use -v to also display the per-module git commit SHAs for each version family.`,
	RunE: runFwversionFamilies,
}

func runFwversionFamilies(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()

	families, err := version.LoadFamilies()
	if err != nil {
		return fmt.Errorf("failed to load version families: %w", err)
	}

	if len(families.Families) == 0 {
		p.Info("No version families recorded yet.")
		p.Info("Run 'flywork fwversion bump' to create the first entry.")
		return nil
	}

	p.Header("Version Family History")
	p.Newline()

	for i, fam := range families.Families {
		marker := " "
		if i == len(families.Families)-1 {
			marker = ui.StyleSuccess.Render("*")
		}

		date := fam.ReleasedAt.Format("2006-01-02 15:04")
		modules := fmt.Sprintf("%d modules", len(fam.Modules))

		fmt.Printf("  %s %-12s  %s  %s\n",
			marker,
			ui.StyleBold.Render(fam.Version),
			ui.StyleMuted.Render(date),
			ui.StyleMuted.Render(modules),
		)

		if fam.Notes != "" {
			fmt.Printf("    %s\n", ui.StyleMuted.Render(fam.Notes))
		}

		if verbose {
			for repo, sha := range fam.Modules {
				fmt.Printf("    %s %s\n",
					ui.StyleMuted.Render(repo),
					ui.StyleMuted.Render(sha),
				)
			}
		}
	}

	return nil
}

// ── init ─────────────────────────────────────────────────────────────────────

func init() {
	// bump flags
	fwversionBumpCmd.Flags().IntVar(&bumpYear, "year", 0, "CalVer year (YY)")
	fwversionBumpCmd.Flags().IntVar(&bumpMonth, "month", 0, "CalVer month (MM)")
	fwversionBumpCmd.Flags().IntVar(&bumpPatch, "patch", 0, "CalVer patch number")
	fwversionBumpCmd.Flags().BoolVar(&bumpAuto, "auto", false, "Auto-compute next CalVer from current")
	fwversionBumpCmd.Flags().BoolVar(&bumpCommit, "commit", true, "Git commit changes")
	fwversionBumpCmd.Flags().BoolVar(&bumpTag, "tag", true, "Git tag with version")
	fwversionBumpCmd.Flags().BoolVar(&bumpPush, "push", false, "Git push after commit/tag")
	fwversionBumpCmd.Flags().BoolVar(&bumpDryRun, "dry-run", false, "Show changes without modifying files")
	fwversionBumpCmd.Flags().BoolVar(&bumpInstall, "install", false, "Run mvn install after version bump")

	// Wire subcommands
	fwversionCmd.AddCommand(fwversionShowCmd)
	fwversionCmd.AddCommand(fwversionBumpCmd)
	fwversionCmd.AddCommand(fwversionCheckCmd)
	fwversionCmd.AddCommand(fwversionFamiliesCmd)

	rootCmd.AddCommand(fwversionCmd)
}
