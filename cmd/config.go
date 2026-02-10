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
	"strings"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and manage Flywork CLI configuration",
	Long: `View all configuration values. Use subcommands to get, set, or reset individual
keys. Configuration is stored in ~/.flywork/config.yaml.

Available Subcommands:
  get <key>          Get a single configuration value
  set <key> <value>  Set a configuration value
  reset              Reset all configuration to defaults

Valid configuration keys:
  repos_path         Where framework repos are cloned (default: ~/.flywork/repos)
  github_org         GitHub organization name (default: fireflyframework)
  default_group_id   Default Maven groupId for new projects (default: org.fireflyframework)
  java_version       Target Java version for compilation (default: 25)
  parent_version     Parent POM version for archetypes (default: 26.01.01)
  cli_auto_update    Auto-check for CLI updates on launch (default: false)
  branch             Git branch to clone during setup (default: develop)

Examples:
  flywork config                              Show all configuration
  flywork config get java_version             Get a single value
  flywork config set java_version 25          Set a value
  flywork config set branch main              Change the default branch
  flywork config reset                        Reset to defaults`,
	RunE: runConfigList,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Prints the value of a single configuration key to stdout with no formatting.
This is useful for scripting and CI/CD integration.

Valid keys: repos_path, github_org, default_group_id, java_version,
parent_version, cli_auto_update, branch`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: config.ValidKeys,
	RunE:      runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Sets a configuration key to the specified value and saves it to
~/.flywork/config.yaml.

Valid keys: repos_path, github_org, default_group_id, java_version,
parent_version, cli_auto_update, branch

For cli_auto_update, accepted values are: true, false, 1, 0, yes, no.`,
	Args:      cobra.ExactArgs(2),
	ValidArgs: config.ValidKeys,
	RunE:      runConfigSet,
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset configuration to defaults",
	Long: `Resets all configuration keys to their default values and saves the result
to ~/.flywork/config.yaml. The default values are displayed after the reset.`,
	RunE: runConfigReset,
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configResetCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigList(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	p.Header("Configuration")
	for _, kv := range cfg.Fields() {
		p.KeyValue(kv.Key, kv.Value)
	}
	p.Newline()
	p.Info(fmt.Sprintf("Config file: %s", config.FlyworkHome()+"/config.yaml"))
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	val, ok := cfg.GetField(args[0])
	if !ok {
		return fmt.Errorf("unknown key %q — valid keys: %s", args[0], strings.Join(config.ValidKeys, ", "))
	}
	fmt.Println(val)
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	key, value := args[0], args[1]
	if !cfg.SetField(key, value) {
		return fmt.Errorf("unknown key %q — valid keys: %s", key, strings.Join(config.ValidKeys, ", "))
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	p.Success(fmt.Sprintf("%s = %s", key, value))
	return nil
}

func runConfigReset(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	cfg := config.DefaultConfig()
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	p.Success("Configuration reset to defaults")
	for _, kv := range cfg.Fields() {
		p.KeyValue(kv.Key, kv.Value)
	}
	return nil
}
