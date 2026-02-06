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

package doctor

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/java"
	"github.com/fireflyframework/fireflyframework-cli/internal/maven"
	"github.com/fireflyframework/fireflyframework-cli/internal/setup"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
)

// PomXML minimal struct for parsing pom.xml.
type PomXML struct {
	XMLName    xml.Name    `xml:"project"`
	Parent     PomParent   `xml:"parent"`
	GroupID    string      `xml:"groupId"`
	ArtifactID string      `xml:"artifactId"`
	Packaging  string      `xml:"packaging"`
	Modules    PomModules  `xml:"modules"`
	Deps       PomDeps     `xml:"dependencies"`
}

type PomParent struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

type PomModules struct {
	Module []string `xml:"module"`
}

type PomDeps struct {
	Dependency []PomDep `xml:"dependency"`
}

type PomDep struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
}

// RunGlobal executes all global environment checks.
func RunGlobal(cfg *config.Config) []ui.CheckResult {
	var results []ui.CheckResult
	results = append(results, checkEnvironment())
	results = append(results, checkJava(cfg))
	results = append(results, checkJavaHome(cfg))
	results = append(results, checkMaven())
	results = append(results, checkMavenJava())
	results = append(results, checkGit())
	results = append(results, checkDocker())
	results = append(results, checkFlyworkConfig())
	results = append(results, checkReposCloned(cfg))
	results = append(results, checkParentPOM())
	results = append(results, checkBOM())
	results = append(results, checkSetupManifest())
	return results
}

// RunProject executes project-specific checks. Returns nil if no pom.xml found.
func RunProject(projectDir string) []ui.CheckResult {
	pom, pomErr := parsePom(filepath.Join(projectDir, "pom.xml"))
	if pomErr != nil {
		return nil
	}
	var results []ui.CheckResult
	results = append(results, checkPomParent(pom, pomErr))
	results = append(results, checkModuleStructure(projectDir, pom, pomErr))
	results = append(results, checkPackageConsistency(projectDir, pom))
	results = append(results, checkApplicationYaml(projectDir, pom, pomErr))
	results = append(results, checkFrameworkDeps(projectDir, pom, pomErr))
	results = append(results, checkSpringBootMainClass(projectDir, pom))
	return results
}

// RunAll is a backwards-compatible wrapper.
func RunAll(projectDir string) []ui.CheckResult {
	cfg, _ := config.Load()
	results := RunGlobal(cfg)
	if proj := RunProject(projectDir); proj != nil {
		results = append(results, proj...)
	}
	return results
}

func checkEnvironment() ui.CheckResult {
	detail := fmt.Sprintf("%s/%s, %s", runtime.GOOS, runtime.GOARCH, runtime.Version())
	if shell := os.Getenv("SHELL"); shell != "" {
		detail += ", shell=" + filepath.Base(shell)
	}
	return ui.CheckResult{Name: "Environment", Status: "pass", Detail: detail}
}

// MinJavaVersion is the absolute minimum Java version the framework supports.
const MinJavaVersion = 21

func checkJava(cfg *config.Config) ui.CheckResult {
	defaultVer := 25
	if cfg != nil && cfg.JavaVersion != "" {
		if v, err := strconv.Atoi(cfg.JavaVersion); err == nil {
			defaultVer = v
		}
	}
	checkName := fmt.Sprintf("Java %d+ (default %d)", MinJavaVersion, defaultVer)

	out, err := exec.Command("java", "--version").Output()
	if err != nil {
		return ui.CheckResult{Name: checkName, Status: "fail", Detail: "java not found"}
	}
	version := string(out)
	re := regexp.MustCompile(`(\d+)\.`)
	matches := re.FindStringSubmatch(version)
	if len(matches) >= 2 {
		major, _ := strconv.Atoi(matches[1])
		if major >= defaultVer {
			return ui.CheckResult{Name: checkName, Status: "pass", Detail: fmt.Sprintf("Java %d", major)}
		}
		if major >= MinJavaVersion {
			return ui.CheckResult{Name: checkName, Status: "warn", Detail: fmt.Sprintf("Java %d (compatible, but %d recommended)", major, defaultVer)}
		}
		return ui.CheckResult{Name: checkName, Status: "fail", Detail: fmt.Sprintf("Java %d (need %d+)", major, MinJavaVersion)}
	}
	return ui.CheckResult{Name: checkName, Status: "warn", Detail: "could not parse version"}
}

func checkJavaHome(cfg *config.Config) ui.CheckResult {
	javaHome := os.Getenv("JAVA_HOME")
	if javaHome == "" {
		// Try configured version first, then fall back to minimum
		ver := "25"
		if cfg != nil && cfg.JavaVersion != "" {
			ver = cfg.JavaVersion
		}
		detected, err := java.DetectJavaHome(ver)
		if err != nil {
			// Fall back: try to find any 21+ JDK
			detected, err = java.DetectJavaHome(strconv.Itoa(MinJavaVersion))
		}
		if err != nil {
			return ui.CheckResult{Name: "JAVA_HOME", Status: "warn", Detail: "not set (set JAVA_HOME or use 'flywork config set java_version')"}
		}
		return ui.CheckResult{Name: "JAVA_HOME", Status: "pass", Detail: fmt.Sprintf("detected: %s", detected)}
	}
	return ui.CheckResult{Name: "JAVA_HOME", Status: "pass", Detail: javaHome}
}

func checkMavenJava() ui.CheckResult {
	out, err := exec.Command("mvn", "--version").Output()
	if err != nil {
		return ui.CheckResult{Name: "Maven→Java", Status: "warn", Detail: "mvn not found"}
	}
	// Extract "Java version: X.Y.Z" from mvn --version output
	re := regexp.MustCompile(`Java version: (\d+)`)
	matches := re.FindStringSubmatch(string(out))
	if len(matches) >= 2 {
		return ui.CheckResult{Name: "Maven→Java", Status: "pass", Detail: fmt.Sprintf("Java %s", matches[1])}
	}
	// Also try "runtime" line
	re2 := regexp.MustCompile(`java version "(\d+)`)
	matches2 := re2.FindStringSubmatch(string(out))
	if len(matches2) >= 2 {
		return ui.CheckResult{Name: "Maven→Java", Status: "pass", Detail: fmt.Sprintf("Java %s", matches2[1])}
	}
	return ui.CheckResult{Name: "Maven→Java", Status: "pass", Detail: "version detected"}
}

func checkReposCloned(cfg *config.Config) ui.CheckResult {
	if cfg == nil {
		return ui.CheckResult{Name: "Framework repos", Status: "warn", Detail: "config not loaded"}
	}
	total := len(setup.FrameworkRepos)
	cloned := 0
	for _, repo := range setup.FrameworkRepos {
		repoDir := filepath.Join(cfg.ReposPath, repo)
		if _, err := os.Stat(repoDir); err == nil {
			cloned++
		}
	}
	if cloned == 0 {
		return ui.CheckResult{Name: "Framework repos", Status: "fail", Detail: fmt.Sprintf("0/%d cloned (run 'flywork setup')", total)}
	}
	if cloned < total {
		return ui.CheckResult{Name: "Framework repos", Status: "warn", Detail: fmt.Sprintf("%d/%d cloned", cloned, total)}
	}
	return ui.CheckResult{Name: "Framework repos", Status: "pass", Detail: fmt.Sprintf("%d/%d", cloned, total)}
}

func checkMaven() ui.CheckResult {
	ver, err := maven.Version()
	if err != nil {
		return ui.CheckResult{Name: "Maven 3.9+", Status: "fail", Detail: "mvn not found"}
	}
	parts := strings.Split(ver, ".")
	if len(parts) >= 2 {
		major, _ := strconv.Atoi(parts[0])
		minor, _ := strconv.Atoi(parts[1])
		if major > 3 || (major == 3 && minor >= 9) {
			return ui.CheckResult{Name: "Maven 3.9+", Status: "pass", Detail: ver}
		}
		return ui.CheckResult{Name: "Maven 3.9+", Status: "fail", Detail: fmt.Sprintf("%s (need 3.9+)", ver)}
	}
	return ui.CheckResult{Name: "Maven 3.9+", Status: "warn", Detail: ver}
}

func checkGit() ui.CheckResult {
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		return ui.CheckResult{Name: "Git", Status: "fail", Detail: "git not found"}
	}
	return ui.CheckResult{Name: "Git", Status: "pass", Detail: strings.TrimSpace(string(out))}
}

func checkDocker() ui.CheckResult {
	out, err := exec.Command("docker", "--version").Output()
	if err != nil {
		return ui.CheckResult{Name: "Docker", Status: "warn", Detail: "not found (optional — needed for Testcontainers)"}
	}
	re := regexp.MustCompile(`Docker version ([^\s,]+)`)
	if m := re.FindStringSubmatch(string(out)); len(m) >= 2 {
		return ui.CheckResult{Name: "Docker", Status: "pass", Detail: m[1]}
	}
	return ui.CheckResult{Name: "Docker", Status: "pass", Detail: strings.TrimSpace(string(out))}
}

func checkFlyworkConfig() ui.CheckResult {
	cfgPath := filepath.Join(config.FlyworkHome(), config.ConfigFile)
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return ui.CheckResult{Name: "Flywork config", Status: "warn", Detail: "not found — run any flywork command to create defaults"}
	}
	cfg, err := config.Load()
	if err != nil {
		return ui.CheckResult{Name: "Flywork config", Status: "fail", Detail: "corrupt — " + err.Error()}
	}
	return ui.CheckResult{Name: "Flywork config", Status: "pass", Detail: fmt.Sprintf("java=%s, org=%s", cfg.JavaVersion, cfg.GithubOrg)}
}

func checkSetupManifest() ui.CheckResult {
	manifest, err := setup.LoadManifest(setup.DefaultManifestPath())
	if err != nil {
		return ui.CheckResult{Name: "Setup manifest", Status: "warn", Detail: "corrupt — " + err.Error()}
	}
	if manifest == nil {
		return ui.CheckResult{Name: "Setup manifest", Status: "warn", Detail: "not found — run 'flywork setup'"}
	}
	if manifest.IsComplete() {
		s := manifest.Summary()
		return ui.CheckResult{Name: "Setup manifest", Status: "pass", Detail: fmt.Sprintf("complete — %d repos installed", s.InstallsOK)}
	}
	s := manifest.Summary()
	detail := fmt.Sprintf("incomplete — %d/%d cloned, %d/%d installed", s.ClonesOK, s.Total, s.InstallsOK, s.Total)
	if s.ClonesFailed+s.InstallsFailed > 0 {
		detail += fmt.Sprintf(", %d failed", s.ClonesFailed+s.InstallsFailed)
	}
	return ui.CheckResult{Name: "Setup manifest", Status: "warn", Detail: detail}
}

func checkParentPOM() ui.CheckResult {
	if maven.ArtifactExistsInM2("org.fireflyframework", "fireflyframework-parent", "1.0.0-SNAPSHOT") {
		return ui.CheckResult{Name: "Parent POM in .m2", Status: "pass"}
	}
	return ui.CheckResult{Name: "Parent POM in .m2", Status: "fail", Detail: "run 'flywork setup' to install"}
}

func checkBOM() ui.CheckResult {
	if maven.ArtifactExistsInM2("org.fireflyframework", "fireflyframework-bom", "1.0.0-SNAPSHOT") {
		return ui.CheckResult{Name: "BOM in .m2", Status: "pass"}
	}
	return ui.CheckResult{Name: "BOM in .m2", Status: "fail", Detail: "run 'flywork setup' to install"}
}

func parsePom(path string) (*PomXML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pom PomXML
	if err := xml.Unmarshal(data, &pom); err != nil {
		return nil, err
	}
	return &pom, nil
}

func checkPomParent(pom *PomXML, err error) ui.CheckResult {
	if err != nil {
		return ui.CheckResult{Name: "pom.xml parent", Status: "fail", Detail: "could not read pom.xml"}
	}
	if pom.Parent.ArtifactID == "fireflyframework-parent" && pom.Parent.GroupID == "org.fireflyframework" {
		return ui.CheckResult{Name: "pom.xml parent", Status: "pass", Detail: "fireflyframework-parent"}
	}
	// Check if the reactor modules use fireflyframework-parent (multi-module project)
	if pom.Packaging == "pom" && len(pom.Modules.Module) > 0 {
		return ui.CheckResult{Name: "pom.xml parent", Status: "pass", Detail: "reactor POM with fireflyframework-parent ancestor"}
	}
	return ui.CheckResult{Name: "pom.xml parent", Status: "warn", Detail: fmt.Sprintf("parent is %s:%s", pom.Parent.GroupID, pom.Parent.ArtifactID)}
}

func checkModuleStructure(dir string, pom *PomXML, err error) ui.CheckResult {
	if err != nil {
		return ui.CheckResult{Name: "Module structure", Status: "fail", Detail: "could not read pom.xml"}
	}
	if pom.Packaging != "pom" {
		// Single-module project (application or library)
		srcDir := filepath.Join(dir, "src", "main", "java")
		if _, serr := os.Stat(srcDir); serr == nil {
			return ui.CheckResult{Name: "Module structure", Status: "pass", Detail: "single-module"}
		}
		return ui.CheckResult{Name: "Module structure", Status: "warn", Detail: "no src/main/java found"}
	}
	// Multi-module: verify each declared module exists
	missing := 0
	for _, mod := range pom.Modules.Module {
		modDir := filepath.Join(dir, mod)
		if _, serr := os.Stat(modDir); os.IsNotExist(serr) {
			missing++
		}
	}
	if missing > 0 {
		return ui.CheckResult{Name: "Module structure", Status: "fail", Detail: fmt.Sprintf("%d declared module(s) missing on disk", missing)}
	}
	return ui.CheckResult{Name: "Module structure", Status: "pass", Detail: fmt.Sprintf("%d modules", len(pom.Modules.Module))}
}

func checkPackageConsistency(dir string, pom *PomXML) ui.CheckResult {
	// Determine expected group from POM
	groupID := pom.GroupID
	if groupID == "" {
		groupID = pom.Parent.GroupID
	}
	if groupID == "" {
		return ui.CheckResult{Name: "Package naming", Status: "warn", Detail: "could not determine groupId"}
	}

	javaRoot := filepath.Join(dir, "src", "main", "java")
	if pom.Packaging == "pom" && len(pom.Modules.Module) > 0 {
		javaRoot = filepath.Join(dir, pom.Modules.Module[0], "src", "main", "java")
	}
	if _, serr := os.Stat(javaRoot); os.IsNotExist(serr) {
		return ui.CheckResult{Name: "Package naming", Status: "warn", Detail: "no source directory found"}
	}

	expectedPrefix := groupID
	found := false
	filepath.Walk(javaRoot, func(path string, info os.FileInfo, werr error) error {
		if werr != nil || info.IsDir() || !strings.HasSuffix(path, ".java") || found {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if strings.Contains(string(data), "package "+expectedPrefix) {
			found = true
		}
		return nil
	})

	if found {
		return ui.CheckResult{Name: "Package naming", Status: "pass", Detail: expectedPrefix + ".*"}
	}
	return ui.CheckResult{Name: "Package naming", Status: "warn", Detail: fmt.Sprintf("no packages matching %s found", expectedPrefix)}
}

func checkApplicationYaml(dir string, pom *PomXML, err error) ui.CheckResult {
	if err != nil {
		return ui.CheckResult{Name: "application.yaml", Status: "fail", Detail: "could not read pom.xml"}
	}

	// For multi-module projects, look in the -web module
	searchDirs := []string{dir}
	if pom.Packaging == "pom" {
		for _, mod := range pom.Modules.Module {
			if strings.HasSuffix(mod, "-web") {
				searchDirs = []string{filepath.Join(dir, mod)}
				break
			}
		}
	}

	for _, d := range searchDirs {
	for _, name := range []string{"application.yaml", "application.yml", "application.properties"} {
			path := filepath.Join(d, "src", "main", "resources", name)
			if _, serr := os.Stat(path); serr == nil {
				return ui.CheckResult{Name: "application.yaml", Status: "pass", Detail: name}
			}
		}
	}

	return ui.CheckResult{Name: "application.yaml", Status: "warn", Detail: "no application.yaml found in resources"}
}

func checkFrameworkDeps(dir string, pom *PomXML, err error) ui.CheckResult {
	if err != nil {
		return ui.CheckResult{Name: "Framework dependencies", Status: "fail", Detail: "could not read pom.xml"}
	}

	// Collect all dependencies from root and submodule poms
	allDeps := collectDeps(pom)
	if pom.Packaging == "pom" {
		for _, mod := range pom.Modules.Module {
			subPom, serr := parsePom(filepath.Join(dir, mod, "pom.xml"))
			if serr == nil {
				allDeps = append(allDeps, collectDeps(subPom)...)
			}
		}
	}

	count := 0
	for _, d := range allDeps {
		if d.GroupID == "org.fireflyframework" {
			count++
		}
	}
	if count == 0 {
		return ui.CheckResult{Name: "Framework dependencies", Status: "warn", Detail: "no org.fireflyframework dependencies found"}
	}
	return ui.CheckResult{Name: "Framework dependencies", Status: "pass", Detail: fmt.Sprintf("%d org.fireflyframework dependencies", count)}
}

func checkSpringBootMainClass(dir string, pom *PomXML) ui.CheckResult {
	if pom.Packaging == "pom" {
		for _, mod := range pom.Modules.Module {
			if strings.HasSuffix(mod, "-web") {
				return scanForSpringBootMain(filepath.Join(dir, mod))
			}
		}
		if len(pom.Modules.Module) > 0 {
			return scanForSpringBootMain(filepath.Join(dir, pom.Modules.Module[0]))
		}
	}
	return scanForSpringBootMain(dir)
}

func scanForSpringBootMain(moduleDir string) ui.CheckResult {
	javaRoot := filepath.Join(moduleDir, "src", "main", "java")
	if _, err := os.Stat(javaRoot); os.IsNotExist(err) {
		return ui.CheckResult{Name: "Spring Boot main class", Status: "warn", Detail: "no source directory"}
	}
	found := false
	filepath.Walk(javaRoot, func(path string, info os.FileInfo, werr error) error {
		if werr != nil || info.IsDir() || !strings.HasSuffix(path, ".java") || found {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if strings.Contains(string(data), "@SpringBootApplication") {
			found = true
		}
		return nil
	})
	if found {
		return ui.CheckResult{Name: "Spring Boot main class", Status: "pass"}
	}
	return ui.CheckResult{Name: "Spring Boot main class", Status: "warn", Detail: "no @SpringBootApplication found (expected for runnable apps)"}
}

func collectDeps(pom *PomXML) []PomDep {
	return pom.Deps.Dependency
}
