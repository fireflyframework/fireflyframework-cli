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

package build

import (
	"os"
	"path/filepath"

	"github.com/fireflyframework/fireflyframework-cli/internal/dag"
	"github.com/fireflyframework/fireflyframework-cli/internal/git"
)

// DetectChanges compares the current HEAD SHA of each repo in the graph against
// the last successfully built SHA recorded in the manifest. Repos whose SHA
// differs (or that have no manifest entry) are marked as changed.
func DetectChanges(g *dag.Graph, reposDir string, manifest *BuildManifest) map[string]bool {
	changed := make(map[string]bool)

	for _, repo := range g.Nodes() {
		dir := filepath.Join(reposDir, repo)

		// Skip repos that aren't cloned yet
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		currentSHA, err := git.HeadSHA(dir)
		if err != nil {
			// Can't read SHA — treat as changed
			changed[repo] = true
			continue
		}

		lastSHA := manifest.LastSHA(repo)
		if lastSHA == "" || lastSHA != currentSHA {
			changed[repo] = true
		}
	}

	return changed
}

// TransitiveClosure expands a set of directly changed repos to include all
// downstream dependents. For each changed repo, it walks the reverse edges of
// the DAG via BFS to find every repo that transitively depends on the change.
func TransitiveClosure(g *dag.Graph, changed map[string]bool) map[string]bool {
	affected := make(map[string]bool)

	// Start with all directly changed repos
	for repo := range changed {
		affected[repo] = true
	}

	// Expand each changed repo to include its transitive dependents
	for repo := range changed {
		for _, dep := range g.TransitiveDependentsOf(repo) {
			affected[dep] = true
		}
	}

	return affected
}
