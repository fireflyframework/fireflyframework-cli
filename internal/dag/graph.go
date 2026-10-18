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

package dag

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Graph represents a directed acyclic graph for dependency resolution.
type Graph struct {
	nodes   map[string]bool
	edges   map[string]map[string]bool // edges[A][B] = true means A depends on B
	reverse map[string]map[string]bool // reverse[B][A] = true means B is depended upon by A
	ordered []string                   // insertion order for deterministic output
}

// New creates an empty graph.
func New() *Graph {
	return &Graph{
		nodes:   make(map[string]bool),
		edges:   make(map[string]map[string]bool),
		reverse: make(map[string]map[string]bool),
	}
}

// AddNode adds a node to the graph. Duplicate adds are no-ops.
func (g *Graph) AddNode(id string) {
	if g.nodes[id] {
		return
	}
	g.nodes[id] = true
	g.edges[id] = make(map[string]bool)
	g.reverse[id] = make(map[string]bool)
	g.ordered = append(g.ordered, id)
}

// AddEdge adds a dependency edge: "from" depends on "to".
// Both nodes are created if they don't already exist.
func (g *Graph) AddEdge(from, to string) {
	g.AddNode(from)
	g.AddNode(to)
	g.edges[from][to] = true
	g.reverse[to][from] = true
}

// NodeCount returns the number of nodes.
func (g *Graph) NodeCount() int {
	return len(g.nodes)
}

// DependenciesOf returns the direct dependencies of a node.
func (g *Graph) DependenciesOf(id string) []string {
	deps := make([]string, 0, len(g.edges[id]))
	for dep := range g.edges[id] {
		deps = append(deps, dep)
	}
	return deps
}

// DependentsOf returns the nodes that directly depend on id.
func (g *Graph) DependentsOf(id string) []string {
	deps := make([]string, 0, len(g.reverse[id]))
	for dep := range g.reverse[id] {
		deps = append(deps, dep)
	}
	return deps
}

// TopologicalSort returns nodes in a valid build order using Kahn's algorithm.
// Returns an error if the graph contains a cycle.
func (g *Graph) TopologicalSort() ([]string, error) {
	inDegree := make(map[string]int, len(g.nodes))
	for id := range g.nodes {
		inDegree[id] = len(g.edges[id])
	}

	// Seed queue with nodes that have no dependencies (in insertion order for determinism)
	queue := make([]string, 0)
	for _, id := range g.ordered {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}

	sorted := make([]string, 0, len(g.nodes))
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		sorted = append(sorted, node)

		// For each node that depends on this one, reduce in-degree
		for dependent := range g.reverse[node] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(sorted) != len(g.nodes) {
		cycle := g.detectCycle()
		return nil, fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " → "))
	}

	return sorted, nil
}

// Layers returns nodes grouped by depth level.
// Layer 0 contains nodes with no dependencies.
// Layer N contains nodes whose dependencies are all in layers 0..N-1.
// Nodes within the same layer are independent and can be processed in parallel.
func (g *Graph) Layers() ([][]string, error) {
	inDegree := make(map[string]int, len(g.nodes))
	for id := range g.nodes {
		inDegree[id] = len(g.edges[id])
	}

	// Seed with zero-dependency nodes (in insertion order for determinism)
	current := make([]string, 0)
	for _, id := range g.ordered {
		if inDegree[id] == 0 {
			current = append(current, id)
		}
	}

	var layers [][]string
	visited := 0

	for len(current) > 0 {
		layers = append(layers, current)
		visited += len(current)

		next := make([]string, 0)
		for _, node := range current {
			for dependent := range g.reverse[node] {
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					next = append(next, dependent)
				}
			}
		}
		current = next
	}

	if visited != len(g.nodes) {
		cycle := g.detectCycle()
		return nil, fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " → "))
	}

	return layers, nil
}

// FlatOrder returns a flat list of nodes in valid dependency order (topological sort).
func (g *Graph) FlatOrder() ([]string, error) {
	return g.TopologicalSort()
}

// HasNode returns true if the graph contains a node with the given id.
func (g *Graph) HasNode(id string) bool {
	return g.nodes[id]
}

// Nodes returns all node IDs in insertion order.
func (g *Graph) Nodes() []string {
	out := make([]string, len(g.ordered))
	copy(out, g.ordered)
	return out
}

// TransitiveDependentsOf performs a BFS on reverse edges from the given node,
// returning all repos that transitively depend on it. The starting node itself
// is not included in the result.
func (g *Graph) TransitiveDependentsOf(id string) []string {
	if !g.nodes[id] {
		return nil
	}

	visited := map[string]bool{id: true}
	queue := []string{id}
	var result []string

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		for dependent := range g.reverse[node] {
			if !visited[dependent] {
				visited[dependent] = true
				result = append(result, dependent)
				queue = append(queue, dependent)
			}
		}
	}

	sort.Strings(result)
	return result
}

// Subgraph returns a new Graph containing only the specified nodes and the
// edges between them. Nodes not present in the original graph are ignored.
func (g *Graph) Subgraph(nodes map[string]bool) *Graph {
	sub := New()

	// Add nodes in original insertion order for determinism
	for _, id := range g.ordered {
		if nodes[id] {
			sub.AddNode(id)
		}
	}

	// Add only edges where both endpoints are in the subgraph
	for from := range nodes {
		if !g.nodes[from] {
			continue
		}
		for to := range g.edges[from] {
			if nodes[to] {
				sub.AddEdge(from, to)
			}
		}
	}

	return sub
}

// dagJSON is the serialization format for ExportJSON.
type dagJSON struct {
	Layers [][]string          `json:"layers"`
	Edges  map[string][]string `json:"edges"`
}

// ExportJSON exports the graph as JSON with layers and edges.
// The output is suitable for consumption by GitHub Actions workflows.
func (g *Graph) ExportJSON() ([]byte, error) {
	layers, err := g.Layers()
	if err != nil {
		return nil, err
	}

	edges := make(map[string][]string, len(g.nodes))
	for _, id := range g.ordered {
		deps := g.DependenciesOf(id)
		sort.Strings(deps)
		if len(deps) > 0 {
			edges[id] = deps
		}
	}

	return json.MarshalIndent(dagJSON{Layers: layers, Edges: edges}, "", "  ")
}

// detectCycle finds and returns one cycle in the graph using DFS.
func (g *Graph) detectCycle() []string {
	const (
		white = 0 // unvisited
		gray  = 1 // in current path
		black = 2 // fully processed
	)

	color := make(map[string]int, len(g.nodes))
	parent := make(map[string]string, len(g.nodes))

	var cycle []string

	var dfs func(node string) bool
	dfs = func(node string) bool {
		color[node] = gray
		for dep := range g.edges[node] {
			if color[dep] == gray {
				// Found cycle: reconstruct
				cycle = []string{dep, node}
				cur := node
				for cur != dep {
					cur = parent[cur]
					cycle = append(cycle, cur)
				}
				// Reverse to get correct order
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}
				return true
			}
			if color[dep] == white {
				parent[dep] = node
				if dfs(dep) {
					return true
				}
			}
		}
		color[node] = black
		return false
	}

	for _, id := range g.ordered {
		if color[id] == white {
			if dfs(id) {
				return cycle
			}
		}
	}

	return []string{"unknown cycle"}
}

// FrameworkGraph represents Git repositories, not Maven modules. Maven resolves
// the dependency graph inside the jafly reactor in a single checkout.
func FrameworkGraph() *Graph {
	g := New()
	g.AddNode("fireflyframework-jafly")
	return g
}
