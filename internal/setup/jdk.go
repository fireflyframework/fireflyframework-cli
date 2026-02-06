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

package setup

import (
	"fmt"
	"strconv"

	"github.com/fireflyframework/fireflyframework-cli/internal/java"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
)

// SelectJDK detects all installed JDKs, presents an interactive picker, and
// returns the selected JAVA_HOME. If no JDKs are found, it falls back to
// java.DetectJavaHome for the configured version.
func SelectJDK(configuredVersion string) (string, error) {
	installs := java.ListInstalled()

	if len(installs) == 0 {
		// Fall back to auto-detection
		home, err := java.DetectJavaHome(configuredVersion)
		if err != nil {
			return "", fmt.Errorf("no Java installations found — install Java %s or set JAVA_HOME", configuredVersion)
		}
		return home, nil
	}

	// Build selection options
	cfgVer, _ := strconv.Atoi(configuredVersion)
	options := make([]string, 0, len(installs))
	defaultIdx := 0

	for i, inst := range installs {
		label := fmt.Sprintf("Java %d — %s  %s", inst.Version, inst.Vendor, inst.Home)
		if inst.Default {
			label += "  (system default)"
		}
		if inst.Version == cfgVer {
			label += "  ★ configured"
			defaultIdx = i
		}
		options = append(options, label)
	}

	selected := ui.Select("Select JDK for building the framework", options, defaultIdx)

	// Find the matching installation
	for i, opt := range options {
		if opt == selected {
			return installs[i].Home, nil
		}
	}

	// Shouldn't happen, but fall back
	return installs[defaultIdx].Home, nil
}
