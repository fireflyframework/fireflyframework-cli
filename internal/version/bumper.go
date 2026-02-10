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
	"strings"

	"github.com/fireflyframework/fireflyframework-cli/internal/dag"
	"github.com/fireflyframework/fireflyframework-cli/internal/git"
)

// BumpOptions controls the behaviour of BumpAll.
type BumpOptions struct {
	ReposDir   string
	OldVersion string
	NewVersion string
	DoCommit   bool
	DoTag      bool
	DoPush     bool
	CommitMsg  string
	DryRun     bool
}

// RepoResult holds the outcome for a single repo during a version bump.
type RepoResult struct {
	Repo       string
	FilesFound int
	Updated    int
	Committed  bool
	Tagged     bool
	Pushed     bool
	Error      error
}

// BumpCallback is invoked after each repo is processed.
type BumpCallback func(idx, total int, result RepoResult)

// BumpAll iterates all repos in DAG order, updating pom.xml versions.
func BumpAll(opts BumpOptions, cb BumpCallback) ([]RepoResult, error) {
	g := dag.FrameworkGraph()
	order, err := g.FlatOrder()
	if err != nil {
		return nil, fmt.Errorf("dependency graph error: %w", err)
	}

	results := make([]RepoResult, 0, len(order))

	for i, repo := range order {
		r := bumpRepo(opts, repo)
		results = append(results, r)
		if cb != nil {
			cb(i+1, len(order), r)
		}
	}

	return results, nil
}

func bumpRepo(opts BumpOptions, repo string) RepoResult {
	r := RepoResult{Repo: repo}
	repoDir := filepath.Join(opts.ReposDir, repo)

	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		r.Error = fmt.Errorf("repo directory not found: %s", repoDir)
		return r
	}

	poms := FindAllPoms(repoDir)
	r.FilesFound = len(poms)

	if len(poms) == 0 {
		return r // nothing to update (e.g. non-Maven repo)
	}

	if opts.DryRun {
		// Count files that would be changed without modifying them
		for _, p := range poms {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			if strings.Contains(string(data), opts.OldVersion) {
				r.Updated++
			}
		}
		return r
	}

	// Replace versions
	for _, p := range poms {
		if err := ReplacePomVersion(p, opts.OldVersion, opts.NewVersion); err != nil {
			r.Error = fmt.Errorf("replace in %s: %w", filepath.Base(p), err)
			return r
		}
		// Check if the file was actually changed
		data, _ := os.ReadFile(p)
		if strings.Contains(string(data), opts.NewVersion) {
			r.Updated++
		}
	}

	// Git operations
	if opts.DoCommit && r.Updated > 0 {
		if err := git.Add(repoDir); err != nil {
			r.Error = fmt.Errorf("git add: %w", err)
			return r
		}
		msg := opts.CommitMsg
		if msg == "" {
			msg = fmt.Sprintf("release: bump version to %s", opts.NewVersion)
		}
		if err := git.Commit(repoDir, msg); err != nil {
			r.Error = fmt.Errorf("git commit: %w", err)
			return r
		}
		r.Committed = true
	}

	if opts.DoTag && r.Updated > 0 {
		tag := "v" + opts.NewVersion
		if err := git.Tag(repoDir, tag); err != nil {
			r.Error = fmt.Errorf("git tag: %w", err)
			return r
		}
		r.Tagged = true
	}

	if opts.DoPush && r.Updated > 0 {
		if err := git.Push(repoDir); err != nil {
			r.Error = fmt.Errorf("git push: %w", err)
			return r
		}
		if opts.DoTag {
			if err := git.PushTags(repoDir); err != nil {
				r.Error = fmt.Errorf("git push tags: %w", err)
				return r
			}
		}
		r.Pushed = true
	}

	return r
}

// BumpGenAI updates version references in the GenAI Python module.
func BumpGenAI(repoDir, oldVer, newVer string, dryRun bool) error {
	files := []string{
		"pyproject.toml",
		"src/fireflyframework_genai/_version.py",
		"scripts/install.sh",
		"scripts/uninstall.sh",
		"scripts/install.ps1",
		"scripts/uninstall.ps1",
	}

	for _, f := range files {
		path := filepath.Join(repoDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}

		if dryRun {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}

		content := string(data)
		if !strings.Contains(content, oldVer) {
			continue
		}

		updated := strings.ReplaceAll(content, oldVer, newVer)
		if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
			return fmt.Errorf("write %s: %w", f, err)
		}
	}

	return nil
}
