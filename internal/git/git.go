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

package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// IsInstalled checks if git is available on PATH.
func IsInstalled() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// Version returns the installed git version string.
func Version() (string, error) {
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Clone clones a repository into the target directory.
func Clone(repoURL, targetDir string) error {
	cmd := exec.Command("git", "clone", "--depth", "1", repoURL, targetDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CloneQuiet clones a repository without terminal output.
func CloneQuiet(repoURL, targetDir string) error {
	cmd := exec.Command("git", "clone", "--quiet", repoURL, targetDir)
	return cmd.Run()
}

// Init initializes a new git repository in the given directory.
func Init(dir string) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	return cmd.Run()
}

// Add stages all files.
func Add(dir string) error {
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	return cmd.Run()
}

// Commit creates a commit with the given message.
func Commit(dir, message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir
	return cmd.Run()
}

// Pull performs a git pull in the given directory.
func Pull(dir string) error {
	cmd := exec.Command("git", "pull", "--quiet")
	cmd.Dir = dir
	return cmd.Run()
}

// FetchQuiet runs git fetch --quiet in the given directory.
func FetchQuiet(dir string) error {
	cmd := exec.Command("git", "fetch", "--quiet")
	cmd.Dir = dir
	return cmd.Run()
}

// HeadCommit returns the short SHA of HEAD in the given directory.
func HeadCommit(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// RepoURL builds a GitHub clone URL for the fireflyframework org.
func RepoURL(org, repo string) string {
	return fmt.Sprintf("https://github.com/%s/%s.git", org, repo)
}
