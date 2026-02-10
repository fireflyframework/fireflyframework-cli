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

package version

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FindAllPoms finds root pom.xml + one-level-deep submodule poms in a repo directory.
// Excludes target/ directories.
func FindAllPoms(repoDir string) []string {
	var poms []string

	root := filepath.Join(repoDir, "pom.xml")
	if _, err := os.Stat(root); err == nil {
		poms = append(poms, root)
	}

	// Check one-level-deep subdirectories for submodule poms
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return poms
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "target" || name == ".git" || name == "node_modules" {
			continue
		}
		subPom := filepath.Join(repoDir, name, "pom.xml")
		if _, err := os.Stat(subPom); err == nil {
			poms = append(poms, subPom)
		}
	}

	return poms
}

// ReplacePomVersion replaces all occurrences of oldVer with newVer in the given file.
// This is a simple string replacement which is safe for framework version strings
// (e.g. "1.0.0-SNAPSHOT" or "26.01.01") that don't collide with library versions.
func ReplacePomVersion(pomPath, oldVer, newVer string) error {
	data, err := os.ReadFile(pomPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", pomPath, err)
	}

	content := string(data)
	if !strings.Contains(content, oldVer) {
		return nil // nothing to replace
	}

	updated := strings.ReplaceAll(content, oldVer, newVer)
	return os.WriteFile(pomPath, []byte(updated), 0644)
}

// versionRe matches the first <version>...</version> inside a <project> or <parent> block.
var versionRe = regexp.MustCompile(`<version>\s*([^<]+?)\s*</version>`)

// parentBlockRe extracts the <parent>...</parent> block.
var parentBlockRe = regexp.MustCompile(`(?s)<parent>(.+?)</parent>`)

// ReadPomVersion extracts the project's own <version> from the POM file.
// It first looks for a <parent> block and returns the version from there.
// Falls back to the first top-level <version> tag.
func ReadPomVersion(pomPath string) (string, error) {
	data, err := os.ReadFile(pomPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", pomPath, err)
	}

	content := string(data)

	// Try parent block version first (most framework repos inherit version from parent)
	if m := parentBlockRe.FindStringSubmatch(content); len(m) >= 2 {
		if vm := versionRe.FindStringSubmatch(m[1]); len(vm) >= 2 {
			return vm[1], nil
		}
	}

	// Fall back to first <version> in file
	if m := versionRe.FindStringSubmatch(content); len(m) >= 2 {
		return m[1], nil
	}

	return "", fmt.Errorf("no <version> found in %s", pomPath)
}
