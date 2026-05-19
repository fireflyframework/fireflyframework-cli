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

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	verbose bool

	bannerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B35")).
			Bold(true)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6C757D")).
			Italic(true)
)

const banner = `
   __ _              __ _         __                                            _
  / _(_)_ __ ___    / _| |_   _  / _|_ __ __ _ _ __ ___   _____      _____  _ __| | __
 | |_| | '__/ _ \  | |_| | | | || |_| '__/ ` + "`" + `_` + "`" + ` | '_ ` + "`" + ` _ \ / _ \ \ /\ / / _ \| '__| |/ /
 |  _| | | |  __/  |  _| | |_| ||  _| | | (_| | | | | | |  __/\ V  V / (_) | |  |   <
 |_| |_|_|  \___|  |_| |_|\__, ||_| |_|  \__,_|_| |_| |_|\___| \_/\_/ \___/|_|  |_|\_\
                          |___/                                                       `

// skipBanner lists command names (or parent+child) that should NOT print the banner.
var skipBanner = map[string]bool{
	"version": true,
	"config get": true,
	"config set": true,
	"config reset": true,
	"help": true,
	"completion": true,
}

func shouldSkipBanner(cmd *cobra.Command) bool {
	// Skip if --help/-h flag was set
	if cmd.Flags().Changed("help") {
		return true
	}
	// Skip if --json flag was set (prevents banner from corrupting JSON output)
	if f := cmd.Flags().Lookup("json"); f != nil && f.Changed {
		return true
	}
	// Build command path like "config get" (stop at root)
	parts := []string{}
	for c := cmd; c != nil && c.Parent() != nil; c = c.Parent() {
		parts = append([]string{c.Name()}, parts...)
	}
	path := strings.Join(parts, " ")
	return skipBanner[path]
}

var rootCmd = &cobra.Command{
	Use:   "flywork",
	Short: "Firefly Framework CLI",
	Long: bannerStyle.Render(banner) + "\n" + subtitleStyle.Render("  The Firefly Framework command-line interface") + `

The official CLI for the Firefly Framework. Scaffold, setup, build, publish,
and manage your Firefly-based Java microservices from the terminal.

Available Commands:
  setup       Bootstrap the Firefly Framework (clone + install all repos)
  create      Scaffold a new project from an archetype (core, domain, application, library)
  doctor      Diagnose your environment and project health
  update      Pull latest changes and reinstall framework repos
  build       Smart DAG-aware build with change detection
  publish     Publish Maven artifacts to GitHub Packages
  run         Run a Firefly Framework application with configuration assistance
  dag         Inspect the framework dependency graph
  fwversion   Manage framework-wide CalVer versions
  release     Orchestrate a full layer-by-layer framework release (push, PR, tag, publish)
  config      View and manage CLI configuration
  upgrade     Self-update the CLI binary from GitHub releases
  version     Print CLI version information

Getting Started:
  flywork setup              Bootstrap the framework into your local environment
  flywork create core        Scaffold a new Core microservice project
  flywork doctor             Verify your environment is correctly configured

Configuration:
  Config file: ~/.flywork/config.yaml
  Repos path:  ~/.flywork/repos`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if !shouldSkipBanner(cmd) {
			fmt.Println(bannerStyle.Render(banner))
			fmt.Println(subtitleStyle.Render("  The Firefly Framework command-line interface"))
			fmt.Println()
		}
	},
}

func Execute() {
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, lipgloss.NewStyle().Foreground(lipgloss.Color("#DC3545")).Render("Error: "+err.Error()))
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
}
