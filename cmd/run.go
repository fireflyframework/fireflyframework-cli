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
	"path/filepath"
	"strings"

	"github.com/fireflyframework/fireflyframework-cli/internal/runner"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
	"github.com/spf13/cobra"
)

var (
	flagProfile   string
	flagSkipWizard bool
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a Firefly Framework application with configuration assistance",
	Long:  "Detects the Spring Boot module, scans application config for missing variables, and launches an interactive wizard before running the application",
	RunE:  runRun,
}

func init() {
	runCmd.Flags().StringVar(&flagProfile, "profile", "", "Spring profile to activate (e.g. dev, local)")
	runCmd.Flags().BoolVar(&flagSkipWizard, "skip-wizard", false, "Skip the interactive configuration wizard")

	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	p.Header("Firefly Application Runner")
	p.Newline()

	// ── 1. Analyze project ─────────────────────────────────────────────
	info, err := runner.AnalyzeProject(".")
	if err != nil {
		return err
	}

	// ── 2. Project overview ─────────────────────────────────────────────
	p.Header("Project")
	p.KeyValue("Name", info.Name)

	archetypeLabel := info.Archetype
	switch info.Archetype {
	case "core":
		archetypeLabel = "core  (R2DBC, Flyway, OpenAPI, reactive services)"
	case "domain":
		archetypeLabel = "domain  (CQRS, Saga, bounded-context)"
	case "application":
		archetypeLabel = "application  (single-module, security, plugin architecture)"
	case "library":
		archetypeLabel = "library  (Spring Boot auto-configuration)"
	case "unknown":
		archetypeLabel = ui.StyleWarning.Render("unknown") + "  (not a recognized Firefly archetype)"
	}
	p.KeyValue("Archetype", archetypeLabel)

	if info.MultiModule {
		p.KeyValue("Layout", fmt.Sprintf("multi-module  (%d modules)", len(info.Modules)))
		for _, m := range info.Modules {
			p.Step(m)
		}
	} else {
		p.KeyValue("Layout", "single-module")
	}

	moduleDir := info.WebModule
	if info.MultiModule {
		relPath, _ := filepath.Rel(".", moduleDir)
		p.KeyValue("Web module", relPath)
	}

	if len(info.ConfigFiles) > 0 {
		p.KeyValue("Config files", strings.Join(info.ConfigFiles, ", "))
	}
	if len(info.Profiles) > 0 {
		p.KeyValue("Profiles", strings.Join(info.Profiles, ", "))
	}

	// ── 3. Profile selection ────────────────────────────────────────────
	selectedProfile := flagProfile
	if selectedProfile == "" && len(info.Profiles) > 0 && !flagSkipWizard {
		p.Newline()
		options := append([]string{"(none)"}, info.Profiles...)
		choice := ui.Select("Select Spring profile", options, 0)
		if choice != "(none)" {
			selectedProfile = choice
		}
	}

	// ── 4. Scan configuration placeholders ──────────────────────────────
	placeholders, err := runner.ScanPlaceholders(moduleDir)
	if err != nil {
		return fmt.Errorf("failed to scan config: %w", err)
	}

	envOverrides := make(map[string]string)

	if !flagSkipWizard && len(placeholders) > 0 {
		setFromEnv := runner.FindEnvSetVars(placeholders)
		withDefaults := runner.FindDefaultedVars(placeholders)
		missing := runner.FindMissingEnvVars(placeholders)

		// ── 4a. Configuration table ────────────────────────────────────
		p.Newline()
		p.Header(fmt.Sprintf("Configuration  (%d variables)", len(placeholders)))

		if len(setFromEnv) > 0 {
			p.Newline()
			p.Success(fmt.Sprintf("%d set from environment:", len(setFromEnv)))
			for _, ph := range setFromEnv {
				p.KeyValue("  "+ph.Key, os.Getenv(ph.Key))
			}
		}

		if len(withDefaults) > 0 {
			p.Newline()
			p.Info(fmt.Sprintf("%d with defaults:", len(withDefaults)))
			for _, ph := range withDefaults {
				p.KeyValue("  "+ph.Key, ph.Default)
			}
		}

		if len(missing) > 0 {
			p.Newline()
			p.Error(fmt.Sprintf("%d NOT SET (no default, no env):", len(missing)))
			for _, ph := range missing {
				p.KeyValue("  "+ph.Key, ui.StyleError.Render("REQUIRED"))
			}
		}

		// ── 4b. Force-fill missing variables ─────────────────────────
		if len(missing) > 0 {
			p.Newline()
			p.Warning("The application will fail to start without these variables.")
			p.Newline()
			for _, m := range missing {
				hint := guessDefault(m.Key)
				val := ui.Prompt(m.Key, hint)
				if val != "" {
					envOverrides[m.Key] = val
				}
			}
		}

		// ── 4c. Offer to override defaults ─────────────────────────
		if len(withDefaults) > 0 {
			p.Newline()
			if ui.Confirm("Override any default values?", false) {
				p.Newline()
				for _, d := range withDefaults {
					val := ui.Prompt(d.Key, d.Default)
					if val != "" && val != d.Default {
						envOverrides[d.Key] = val
					}
				}
			}
		}
	} else if !flagSkipWizard {
		p.Newline()
		p.Info("No config placeholders found — running with defaults")
	}

	// ── 5. Launch summary ───────────────────────────────────────────────
	p.Newline()
	p.Header("Launch Configuration")

	if info.MultiModule {
		relPath, _ := filepath.Rel(".", moduleDir)
		p.KeyValue("Module", relPath)
	} else {
		p.KeyValue("Module", ".")
	}

	if selectedProfile != "" {
		p.KeyValue("Profile", selectedProfile)
	} else {
		p.KeyValue("Profile", "(default)")
	}

	if len(envOverrides) > 0 {
		p.KeyValue("Overrides", fmt.Sprintf("%d variable(s)", len(envOverrides)))
		for k, v := range envOverrides {
			p.Step(fmt.Sprintf("%s = %s", k, v))
		}
	} else {
		p.KeyValue("Overrides", "none")
	}

	p.Newline()
	if !ui.Confirm("Start application?", true) {
		p.Warning("Aborted.")
		return nil
	}

	p.Newline()
	p.Info("Starting application with mvn spring-boot:run ...")
	p.Newline()

	return runner.RunSpringBoot(moduleDir, selectedProfile, envOverrides)
}

// guessDefault provides sensible defaults for common env var names.
func guessDefault(key string) string {
	k := strings.ToUpper(key)
	switch {
	case strings.Contains(k, "DB_HOST") || strings.Contains(k, "DATABASE_HOST"):
		return "localhost"
	case strings.Contains(k, "DB_PORT") || strings.Contains(k, "DATABASE_PORT"):
		return "5432"
	case strings.Contains(k, "DB_NAME") || strings.Contains(k, "DATABASE_NAME"):
		return "postgres"
	case strings.Contains(k, "DB_USER") || strings.Contains(k, "DATABASE_USER"):
		return "postgres"
	case strings.Contains(k, "DB_PASS") || strings.Contains(k, "DATABASE_PASS"):
		return "postgres"
	case strings.Contains(k, "SERVER_PORT") || strings.Contains(k, "PORT"):
		return "8080"
	case strings.Contains(k, "REDIS_HOST"):
		return "localhost"
	case strings.Contains(k, "REDIS_PORT"):
		return "6379"
	default:
		return ""
	}
}

func relativeModule(root, moduleDir string) string {
	rel, err := filepath.Rel(root, moduleDir)
	if err != nil || rel == "." {
		return "."
	}
	return rel
}
