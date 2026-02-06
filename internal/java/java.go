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

package java

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Installation represents a detected Java installation on the system.
type Installation struct {
	Version  int    // Major version (e.g. 25)
	Home     string // JAVA_HOME path
	Vendor   string // Vendor hint extracted from path
	Default  bool   // Whether this is the current default
}

// CurrentVersion returns the major version from `java --version`.
func CurrentVersion() (int, error) {
	out, err := exec.Command("java", "--version").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("java not found: %w", err)
	}
	return parseMajorVersion(string(out))
}

// DetectJavaHome finds the JAVA_HOME for a specific major version.
// It tries platform-specific discovery, then falls back to JAVA_HOME env var.
func DetectJavaHome(version string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return detectDarwin(version)
	case "linux":
		return detectLinux(version)
	case "windows":
		return detectWindows(version)
	default:
		return detectFallback(version)
	}
}

// ListInstalled returns all Java installations detected on the system.
func ListInstalled() []Installation {
	switch runtime.GOOS {
	case "darwin":
		return listDarwin()
	case "linux":
		return listLinux()
	case "windows":
		return listWindows()
	default:
		return listFallback()
	}
}

// IsInstalled checks if java is available on PATH.
func IsInstalled() bool {
	_, err := exec.LookPath("java")
	return err == nil
}

// ── Darwin (macOS) ──────────────────────────────────────────────────────────

func detectDarwin(version string) (string, error) {
	out, err := exec.Command("/usr/libexec/java_home", "-v", version).Output()
	if err == nil {
		home := strings.TrimSpace(string(out))
		if home != "" {
			return home, nil
		}
	}
	return detectFallback(version)
}

func listDarwin() []Installation {
	out, err := exec.Command("/usr/libexec/java_home", "-V").CombinedOutput()
	if err != nil {
		return listFallback()
	}

	var installs []Installation
	currentVer, _ := CurrentVersion()

	// Each line looks like: "    25.0.1 (arm64) "AdoptOpenJDK" - "OpenJDK ..." /path/to/jdk"
	// or: "    25.0.1, arm64:	"Temurin" - "OpenJDK 25.0.1" /Library/Java/..."
	re := regexp.MustCompile(`(\d+)[\d.]*[^/]+(/.+)`)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 3 {
			major, _ := strconv.Atoi(matches[1])
			home := strings.TrimSpace(matches[2])
			if major > 0 && home != "" {
				vendor := extractVendor(home)
				installs = append(installs, Installation{
					Version: major,
					Home:    home,
					Vendor:  vendor,
					Default: major == currentVer,
				})
			}
		}
	}

	sort.Slice(installs, func(i, j int) bool {
		return installs[i].Version > installs[j].Version
	})
	return installs
}

// ── Linux ───────────────────────────────────────────────────────────────────

func detectLinux(version string) (string, error) {
	// Check common JDK locations
	searchPaths := []string{
		"/usr/lib/jvm",
		"/usr/local/lib/jvm",
		"/usr/java",
	}

	for _, base := range searchPaths {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.Contains(name, version) || strings.Contains(name, "java-"+version) {
				candidate := filepath.Join(base, name)
				if isValidJavaHome(candidate) {
					return candidate, nil
				}
			}
		}
	}

	// Try update-alternatives
	out, err := exec.Command("update-alternatives", "--list", "java").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, version) {
				// /usr/lib/jvm/java-25-openjdk-amd64/bin/java → parent twice
				home := filepath.Dir(filepath.Dir(line))
				if isValidJavaHome(home) {
					return home, nil
				}
			}
		}
	}

	return detectFallback(version)
}

func listLinux() []Installation {
	var installs []Installation
	currentVer, _ := CurrentVersion()

	searchPaths := []string{
		"/usr/lib/jvm",
		"/usr/local/lib/jvm",
	}

	re := regexp.MustCompile(`java-(\d+)`)
	for _, base := range searchPaths {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			candidate := filepath.Join(base, e.Name())
			if !isValidJavaHome(candidate) {
				continue
			}
			matches := re.FindStringSubmatch(e.Name())
			if len(matches) >= 2 {
				major, _ := strconv.Atoi(matches[1])
				if major > 0 {
					installs = append(installs, Installation{
						Version: major,
						Home:    candidate,
						Vendor:  extractVendor(candidate),
						Default: major == currentVer,
					})
				}
			}
		}
	}

	sort.Slice(installs, func(i, j int) bool {
		return installs[i].Version > installs[j].Version
	})
	return installs
}

// ── Windows ─────────────────────────────────────────────────────────────────

func detectWindows(version string) (string, error) {
	// Check common install locations
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}

	searchPaths := []string{
		filepath.Join(programFiles, "Java"),
		filepath.Join(programFiles, "Eclipse Adoptium"),
		filepath.Join(programFiles, "Microsoft"),
		filepath.Join(programFiles, "Zulu"),
	}

	for _, base := range searchPaths {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if strings.Contains(e.Name(), version) {
				candidate := filepath.Join(base, e.Name())
				if isValidJavaHome(candidate) {
					return candidate, nil
				}
			}
		}
	}

	return detectFallback(version)
}

func listWindows() []Installation {
	var installs []Installation
	currentVer, _ := CurrentVersion()

	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}

	searchPaths := []string{
		filepath.Join(programFiles, "Java"),
		filepath.Join(programFiles, "Eclipse Adoptium"),
	}

	re := regexp.MustCompile(`(\d+)`)
	for _, base := range searchPaths {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			candidate := filepath.Join(base, e.Name())
			if !isValidJavaHome(candidate) {
				continue
			}
			matches := re.FindStringSubmatch(e.Name())
			if len(matches) >= 2 {
				major, _ := strconv.Atoi(matches[1])
				if major >= 8 {
					installs = append(installs, Installation{
						Version: major,
						Home:    candidate,
						Vendor:  extractVendor(candidate),
						Default: major == currentVer,
					})
				}
			}
		}
	}

	sort.Slice(installs, func(i, j int) bool {
		return installs[i].Version > installs[j].Version
	})
	return installs
}

// ── Fallback ────────────────────────────────────────────────────────────────

func detectFallback(version string) (string, error) {
	javaHome := os.Getenv("JAVA_HOME")
	if javaHome != "" && isValidJavaHome(javaHome) {
		return javaHome, nil
	}
	return "", fmt.Errorf("could not detect JAVA_HOME for Java %s — set JAVA_HOME manually or install Java %s", version, version)
}

func listFallback() []Installation {
	javaHome := os.Getenv("JAVA_HOME")
	if javaHome == "" || !isValidJavaHome(javaHome) {
		return nil
	}
	ver, _ := CurrentVersion()
	return []Installation{{
		Version: ver,
		Home:    javaHome,
		Vendor:  extractVendor(javaHome),
		Default: true,
	}}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func isValidJavaHome(path string) bool {
	javaBin := "java"
	if runtime.GOOS == "windows" {
		javaBin = "java.exe"
	}
	_, err := os.Stat(filepath.Join(path, "bin", javaBin))
	return err == nil
}

func parseMajorVersion(output string) (int, error) {
	re := regexp.MustCompile(`(\d+)\.`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		return strconv.Atoi(matches[1])
	}
	// Try standalone number pattern (e.g. "openjdk 25 2025-09-16")
	re2 := regexp.MustCompile(`(?:java|jdk|openjdk)\s+(\d+)`)
	matches2 := re2.FindStringSubmatch(strings.ToLower(output))
	if len(matches2) >= 2 {
		return strconv.Atoi(matches2[1])
	}
	return 0, fmt.Errorf("could not parse Java version from: %s", output)
}

func extractVendor(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "temurin") || strings.Contains(lower, "adoptium"):
		return "Eclipse Temurin"
	case strings.Contains(lower, "corretto"):
		return "Amazon Corretto"
	case strings.Contains(lower, "zulu"):
		return "Azul Zulu"
	case strings.Contains(lower, "graalvm"):
		return "GraalVM"
	case strings.Contains(lower, "openjdk"):
		return "OpenJDK"
	case strings.Contains(lower, "oracle"):
		return "Oracle"
	case strings.Contains(lower, "microsoft"):
		return "Microsoft"
	default:
		return "Unknown"
	}
}
