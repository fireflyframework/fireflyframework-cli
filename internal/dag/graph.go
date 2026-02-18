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

package dag

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Graph represents a directed acyclic graph for dependency resolution.
type Graph struct {
	nodes    map[string]bool
	edges    map[string]map[string]bool // edges[A][B] = true means A depends on B
	reverse  map[string]map[string]bool // reverse[B][A] = true means B is depended upon by A
	ordered  []string                   // insertion order for deterministic output
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
	Layers [][]string            `json:"layers"`
	Edges  map[string][]string   `json:"edges"`
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

// FrameworkGraph returns the pre-wired dependency graph for all fireflyframework repos.
func FrameworkGraph() *Graph {
	g := New()

	// Aliases for readability
	const (
		parent           = "fireflyframework-parent"
		bom              = "fireflyframework-bom"
		kernel           = "fireflyframework-kernel"
		utils            = "fireflyframework-utils"
		validators       = "fireflyframework-validators"
		plugins          = "fireflyframework-plugins"
		cache            = "fireflyframework-cache"
		r2dbc            = "fireflyframework-r2dbc"
		eda              = "fireflyframework-eda"
		cqrs             = "fireflyframework-cqrs"
		eventsourcing    = "fireflyframework-eventsourcing"
		transactionalEng = "fireflyframework-transactional-engine"
		client           = "fireflyframework-client"
		web              = "fireflyframework-web"
		core             = "fireflyframework-core"
		domain           = "fireflyframework-domain"
		data             = "fireflyframework-data"
		workflow         = "fireflyframework-workflow"
		ecm              = "fireflyframework-ecm"
		ecmEsigAdobe     = "fireflyframework-ecm-esignature-adobe-sign"
		ecmEsigDocusign  = "fireflyframework-ecm-esignature-docusign"
		ecmEsigLogalty   = "fireflyframework-ecm-esignature-logalty"
		ecmStorageAWS    = "fireflyframework-ecm-storage-aws"
		ecmStorageAzure  = "fireflyframework-ecm-storage-azure"
		idp              = "fireflyframework-idp"
		idpCognito       = "fireflyframework-idp-aws-cognito"
		idpInternalDB    = "fireflyframework-idp-internal-db"
		idpKeycloak      = "fireflyframework-idp-keycloak"
		idpAzureAD       = "fireflyframework-idp-azure-ad"
		notifications    = "fireflyframework-notifications"
		notifFirebase    = "fireflyframework-notifications-firebase"
		notifResend      = "fireflyframework-notifications-resend"
		notifSendgrid    = "fireflyframework-notifications-sendgrid"
		notifTwilio      = "fireflyframework-notifications-twilio"
		ruleEngine       = "fireflyframework-rule-engine"
		webhooks         = "fireflyframework-webhooks"
		callbacks        = "fireflyframework-callbacks"
		configServer     = "fireflyframework-config-server"
		application      = "fireflyframework-application"
		backoffice       = "fireflyframework-backoffice"
		observability    = "fireflyframework-observability"
	)

	// ── Layer 0: root ──────────────────────────────────────────────────
	g.AddNode(parent)

	// ── Layer 0.5: kernel (foundational — only depends on parent) ────
	g.AddEdge(kernel, parent)

	// ── Layer 1: bom + leaf modules (depend on parent POM + kernel) ──
	g.AddEdge(bom, parent)
	for _, mod := range []string{
		utils, cache, eda, ecm, idp, configServer,
		client, validators, plugins, transactionalEng, observability,
	} {
		g.AddEdge(mod, parent)
	}

	// All framework modules depend on kernel for unified exception hierarchy
	for _, mod := range []string{
		idp, utils, cache, eda, cqrs, eventsourcing, workflow, client,
		web, transactionalEng, application, plugins, ruleEngine, data,
	} {
		g.AddEdge(mod, kernel)
	}

	// ── Layer 1.5: observability consumers (depend on observability) ────
	// These modules gain an edge to observability for centralized metrics/tracing/health
	g.AddEdge(eda, observability)
	g.AddEdge(client, observability)
	g.AddEdge(transactionalEng, observability)
	g.AddEdge(ecm, observability)

	// ── Layer 2: modules with single-level framework dependencies ─────

	// r2dbc depends on utils
	g.AddEdge(r2dbc, utils)

	// cqrs depends on validators, cache, eda (optional bridge), observability
	g.AddEdge(cqrs, validators)
	g.AddEdge(cqrs, cache)
	g.AddEdge(cqrs, eda)
	g.AddEdge(cqrs, observability)

	// web depends on cache, observability
	g.AddEdge(web, cache)
	g.AddEdge(web, observability)

	// workflow depends on cache, eda, observability
	g.AddEdge(workflow, cache)
	g.AddEdge(workflow, eda)
	g.AddEdge(workflow, observability)

	// ECM implementation modules
	g.AddEdge(ecmEsigAdobe, ecm)
	g.AddEdge(ecmEsigDocusign, ecm)
	g.AddEdge(ecmEsigLogalty, ecm)
	g.AddEdge(ecmStorageAWS, ecm)
	g.AddEdge(ecmStorageAzure, ecm)

	// IDP implementation modules
	g.AddEdge(idpCognito, idp)
	g.AddEdge(idpKeycloak, idp)
	g.AddEdge(idpAzureAD, idp)

	// ── Layer 3: modules with deeper dependencies ─────────────────────

	// eventsourcing depends on r2dbc, eda, cache, observability
	g.AddEdge(eventsourcing, r2dbc)
	g.AddEdge(eventsourcing, eda)
	g.AddEdge(eventsourcing, cache)
	g.AddEdge(eventsourcing, observability)

	// application depends on client, cache, cqrs, eda, observability
	g.AddEdge(application, client)
	g.AddEdge(application, cache)
	g.AddEdge(application, cqrs)
	g.AddEdge(application, eda)
	g.AddEdge(application, observability)

	// idp-internal-db depends on idp, r2dbc
	g.AddEdge(idpInternalDB, idp)
	g.AddEdge(idpInternalDB, r2dbc)

	// core depends on eda, cqrs, transactional-engine, observability
	g.AddEdge(core, cqrs)
	g.AddEdge(core, eda)
	g.AddEdge(core, transactionalEng)
	g.AddEdge(core, observability)

	// domain depends on validators, transactional-engine, cqrs, client, eda, observability
	g.AddEdge(domain, validators)
	g.AddEdge(domain, transactionalEng)
	g.AddEdge(domain, cqrs)
	g.AddEdge(domain, client)
	g.AddEdge(domain, eda)
	g.AddEdge(domain, observability)

	// data depends on client, cqrs, eda, cache, transactional-engine, observability
	g.AddEdge(data, client)
	g.AddEdge(data, cqrs)
	g.AddEdge(data, eda)
	g.AddEdge(data, cache)
	g.AddEdge(data, transactionalEng)
	g.AddEdge(data, observability)

	// ── Layer 4: modules that depend on core/domain/data ──────────────

	// notifications depends on core
	g.AddEdge(notifications, core)

	// rule-engine depends on core, cache, utils, validators, web, r2dbc
	g.AddEdge(ruleEngine, core)
	g.AddEdge(ruleEngine, cache)
	g.AddEdge(ruleEngine, utils)
	g.AddEdge(ruleEngine, validators)
	g.AddEdge(ruleEngine, web)
	g.AddEdge(ruleEngine, r2dbc)

	// webhooks depends on core, eda, cache, web, observability
	g.AddEdge(webhooks, core)
	g.AddEdge(webhooks, eda)
	g.AddEdge(webhooks, cache)
	g.AddEdge(webhooks, web)
	g.AddEdge(webhooks, observability)

	// callbacks depends on core, eda, r2dbc, web, observability
	g.AddEdge(callbacks, core)
	g.AddEdge(callbacks, eda)
	g.AddEdge(callbacks, r2dbc)
	g.AddEdge(callbacks, web)
	g.AddEdge(callbacks, observability)

	// backoffice depends on core, domain, data, client, cache, cqrs, eda, application, observability
	g.AddEdge(backoffice, core)
	g.AddEdge(backoffice, domain)
	g.AddEdge(backoffice, data)
	g.AddEdge(backoffice, client)
	g.AddEdge(backoffice, cache)
	g.AddEdge(backoffice, cqrs)
	g.AddEdge(backoffice, eda)
	g.AddEdge(backoffice, application)
	g.AddEdge(backoffice, observability)

	// ── Layer 5: notification implementations → notifications ─────────
	g.AddEdge(notifFirebase, notifications)
	g.AddEdge(notifResend, notifications)
	g.AddEdge(notifSendgrid, notifications)
	g.AddEdge(notifTwilio, notifications)

	return g
}
