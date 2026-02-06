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

	"github.com/fireflyframework/fireflyframework-cli/internal/selfupdate"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
	"github.com/spf13/cobra"
)

var upgradeCheckOnly bool

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the Flywork CLI to the latest version",
	Long:  "Checks GitHub releases for a newer version and self-updates the binary in place",
	RunE:  runUpgrade,
}

func init() {
	upgradeCmd.Flags().BoolVar(&upgradeCheckOnly, "check", false, "Only check for updates, don't install")
	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()

	p.Header("Flywork CLI Upgrade")
	p.Newline()

	spinner := ui.NewSpinner("Checking for updates...")
	spinner.Start()

	result, err := selfupdate.CheckForUpdate(Version)
	if err != nil {
		spinner.Stop(false)
		return fmt.Errorf("update check failed: %w", err)
	}
	spinner.Stop(true)

	p.KeyValue("Current", result.CurrentVersion)
	p.KeyValue("Latest", result.LatestVersion)

	if !result.UpdateAvail {
		p.Newline()
		p.Success("You are already on the latest version!")
		return nil
	}

	p.Newline()
	p.Info(fmt.Sprintf("Update available: %s → %s", result.CurrentVersion, result.LatestVersion))

	if upgradeCheckOnly {
		return nil
	}

	p.Newline()
	spinner = ui.NewSpinner(fmt.Sprintf("Downloading %s...", result.AssetName))
	spinner.Start()

	if err := selfupdate.Apply(result); err != nil {
		spinner.Stop(false)
		return fmt.Errorf("upgrade failed: %w", err)
	}
	spinner.Stop(true)

	p.Newline()
	p.Success(fmt.Sprintf("Upgraded to %s!", result.LatestVersion))
	return nil
}
