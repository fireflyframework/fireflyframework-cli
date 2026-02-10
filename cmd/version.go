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
	"runtime"

	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
	"github.com/spf13/cobra"
)

var (
	// Set via ldflags at build time
	Version   = "dev"
	GitCommit = "none"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print CLI version information",
	Long: `Prints the Flywork CLI version, git commit hash, build date, Go version, and
operating system/architecture. Version information is embedded at build time
via ldflags.`,
	Run: func(cmd *cobra.Command, args []string) {
		p := ui.NewPrinter()
		p.KeyValue("Version", Version)
		p.KeyValue("Git Commit", GitCommit)
		p.KeyValue("Build Date", BuildDate)
		p.KeyValue("Go Version", runtime.Version())
		p.KeyValue("OS/Arch", fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
