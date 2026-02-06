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
	"fmt"
	"os"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/doctor"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose your Firefly Framework environment and project",
	Long:  "Runs global environment checks and, when inside a project directory, project-specific diagnostics.",
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()

	// ── Global environment ─────────────────────────────────────────────
	p.Header("Global Environment")
	p.Newline()

	cfg, _ := config.Load()
	globalResults := doctor.RunGlobal(cfg)
	p.PrintChecks(globalResults)

	// ── Project diagnostics ────────────────────────────────────────────
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not determine working directory: %w", err)
	}

	var projectResults []ui.CheckResult
	if proj := doctor.RunProject(dir); proj != nil {
		p.Newline()
		p.Header("Project Diagnostics")
		p.Newline()
		p.PrintChecks(proj)
		projectResults = proj
	}

	// ── Summary ─────────────────────────────────────────────────────────
	allResults := append(globalResults, projectResults...)
	pass, fail, warn := 0, 0, 0
	for _, r := range allResults {
		switch r.Status {
		case "pass":
			pass++
		case "fail":
			fail++
		case "warn":
			warn++
		}
	}

	p.Newline()
	summary := fmt.Sprintf("%d passed", pass)
	if warn > 0 {
		summary += fmt.Sprintf(", %d warnings", warn)
	}
	if fail > 0 {
		summary += fmt.Sprintf(", %d failed", fail)
		p.Error("Diagnosis: " + summary)
	} else if warn > 0 {
		p.Warning("Diagnosis: " + summary)
	} else {
		p.Success("Diagnosis: " + summary)
	}

	return nil
}
