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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fireflyframework/fireflyframework-cli/internal/dag"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
	"github.com/spf13/cobra"
)

var dagCmd = &cobra.Command{
	Use:   "dag",
	Short: "Inspect the framework dependency graph",
	Long:  "Commands for viewing and querying the fireflyframework dependency DAG",
}

var dagShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display the full dependency graph as an ASCII tree",
	RunE:  runDagShow,
}

var dagLayersCmd = &cobra.Command{
	Use:   "layers",
	Short: "Show repositories grouped by build layer",
	RunE:  runDagLayers,
}

var (
	dagAffectedFrom string
	dagAffectedJSON bool
)

var dagAffectedCmd = &cobra.Command{
	Use:   "affected",
	Short: "Compute transitive closure of repos affected by a change",
	RunE:  runDagAffected,
}

var dagExportJSON bool

var dagExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the DAG as JSON for CI/CD consumption",
	RunE:  runDagExport,
}

func init() {
	dagAffectedCmd.Flags().StringVar(&dagAffectedFrom, "from", "", "Source repo to compute affected repos from (required)")
	dagAffectedCmd.Flags().BoolVar(&dagAffectedJSON, "json", false, "Output as JSON")
	_ = dagAffectedCmd.MarkFlagRequired("from")

	dagExportCmd.Flags().BoolVar(&dagExportJSON, "json", true, "Export as JSON (default)")

	dagCmd.AddCommand(dagShowCmd)
	dagCmd.AddCommand(dagLayersCmd)
	dagCmd.AddCommand(dagAffectedCmd)
	dagCmd.AddCommand(dagExportCmd)
	rootCmd.AddCommand(dagCmd)
}

func runDagShow(_ *cobra.Command, _ []string) error {
	p := ui.NewPrinter()
	g := dag.FrameworkGraph()

	layers, err := g.Layers()
	if err != nil {
		return err
	}

	p.Header("Dependency Graph")
	p.Newline()

	for i, layer := range layers {
		for _, repo := range layer {
			deps := g.DependenciesOf(repo)
			short := strings.TrimPrefix(repo, "fireflyframework-")

			if len(deps) == 0 {
				fmt.Printf("  %s\n", ui.StylePrimary.Render(short))
			} else {
				depNames := make([]string, len(deps))
				for j, d := range deps {
					depNames[j] = strings.TrimPrefix(d, "fireflyframework-")
				}
				arrow := ui.StyleMuted.Render(" → ")
				depList := ui.StyleMuted.Render(strings.Join(depNames, ", "))
				fmt.Printf("  %s%s%s\n", ui.StyleBold.Render(short), arrow, depList)
			}
		}
		if i < len(layers)-1 {
			fmt.Println(ui.StyleMuted.Render("  " + strings.Repeat("·", 50)))
		}
	}

	p.Newline()
	p.Info(fmt.Sprintf("%d repositories, %d layers", g.NodeCount(), len(layers)))

	return nil
}

func runDagLayers(_ *cobra.Command, _ []string) error {
	p := ui.NewPrinter()
	g := dag.FrameworkGraph()

	layers, err := g.Layers()
	if err != nil {
		return err
	}

	p.Header("Build Layers")
	p.Newline()

	for i, layer := range layers {
		label := fmt.Sprintf("Layer %d (%d repos)", i, len(layer))
		fmt.Printf("  %s\n", ui.StylePrimary.Render(label))

		for _, repo := range layer {
			short := strings.TrimPrefix(repo, "fireflyframework-")
			fmt.Printf("    %s %s\n", ui.StyleMuted.Render("•"), short)
		}
		p.Newline()
	}

	p.Info(fmt.Sprintf("Total: %d repositories across %d layers", g.NodeCount(), len(layers)))

	return nil
}

func runDagAffected(_ *cobra.Command, _ []string) error {
	g := dag.FrameworkGraph()

	if !g.HasNode(dagAffectedFrom) {
		return fmt.Errorf("unknown repository: %s", dagAffectedFrom)
	}

	affected := g.TransitiveDependentsOf(dagAffectedFrom)

	if dagAffectedJSON {
		out := struct {
			Source   string   `json:"source"`
			Affected []string `json:"affected"`
			Count    int      `json:"count"`
		}{
			Source:   dagAffectedFrom,
			Affected: affected,
			Count:    len(affected),
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	p := ui.NewPrinter()
	p.Header(fmt.Sprintf("Affected by %s", strings.TrimPrefix(dagAffectedFrom, "fireflyframework-")))
	p.Newline()

	if len(affected) == 0 {
		p.Info("No downstream dependents")
		return nil
	}

	for _, repo := range affected {
		short := strings.TrimPrefix(repo, "fireflyframework-")
		fmt.Printf("  %s %s\n", ui.StyleMuted.Render("•"), short)
	}
	p.Newline()
	p.Info(fmt.Sprintf("%d repos affected", len(affected)))

	return nil
}

func runDagExport(_ *cobra.Command, _ []string) error {
	g := dag.FrameworkGraph()

	data, err := g.ExportJSON()
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}
