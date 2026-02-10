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

package publish

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PublishPython builds a Python package with uv and uploads the wheel and sdist
// as GitHub Release assets. This avoids PyPI and uses GitHub Releases as the
// distribution channel, which is the standard approach for org-internal packages.
func PublishPython(repoDir, githubOrg string) error {
	// Check uv is available
	if _, err := exec.LookPath("uv"); err != nil {
		return fmt.Errorf("uv not found on PATH — install it with: curl -LsSf https://astral.sh/uv/install.sh | sh")
	}

	// Check gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found on PATH — install it with: brew install gh")
	}

	// Build the package
	buildCmd := exec.Command("uv", "build")
	buildCmd.Dir = repoDir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("uv build failed: %w", err)
	}

	// Find dist files
	distDir := filepath.Join(repoDir, "dist")
	entries, err := os.ReadDir(distDir)
	if err != nil {
		return fmt.Errorf("failed to read dist directory: %w", err)
	}

	var files []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".whl") || strings.HasSuffix(name, ".tar.gz") {
			files = append(files, filepath.Join(distDir, name))
		}
	}

	if len(files) == 0 {
		return fmt.Errorf("no .whl or .tar.gz files found in %s", distDir)
	}

	// Get latest tag for the release
	tagCmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	tagCmd.Dir = repoDir
	tagOut, err := tagCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to determine release tag: %w", err)
	}
	tag := strings.TrimSpace(string(tagOut))

	// Upload files to the GitHub release
	args := []string{"release", "upload", tag}
	args = append(args, files...)
	args = append(args, "--clobber")

	uploadCmd := exec.Command("gh", args...)
	uploadCmd.Dir = repoDir
	uploadCmd.Stdout = os.Stdout
	uploadCmd.Stderr = os.Stderr
	if err := uploadCmd.Run(); err != nil {
		return fmt.Errorf("gh release upload failed: %w", err)
	}

	return nil
}
