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

// Package publish provides functionality for publishing artifacts to GitHub
// Packages (Maven) and GitHub Releases (Python). It manages Maven settings.xml
// configuration, Maven deploy execution, and Python wheel/sdist uploads.
package publish

import (
	"os"
	"path/filepath"
	"strings"
)

const githubServerBlock = `
    <server>
      <id>github</id>
      <username>${env.GITHUB_ACTOR}</username>
      <password>${env.GITHUB_TOKEN}</password>
    </server>`

const settingsTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<settings xmlns="http://maven.apache.org/SETTINGS/1.0.0"
          xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
          xsi:schemaLocation="http://maven.apache.org/SETTINGS/1.0.0
                              https://maven.apache.org/xsd/settings-1.0.0.xsd">
  <servers>
    <server>
      <id>github</id>
      <username>${env.GITHUB_ACTOR}</username>
      <password>${env.GITHUB_TOKEN}</password>
    </server>
  </servers>
</settings>
`

// EnsureSettingsXML checks that ~/.m2/settings.xml contains a <server> entry
// for GitHub Packages. If the file doesn't exist, it creates one with the
// required server block. If it exists but is missing the github server, it
// injects the block into the existing <servers> section.
func EnsureSettingsXML() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}

	m2Dir := filepath.Join(home, ".m2")
	settingsPath := filepath.Join(m2Dir, "settings.xml")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, err
		}
		// File doesn't exist — create it with full template
		if err := os.MkdirAll(m2Dir, 0755); err != nil {
			return false, err
		}
		return true, os.WriteFile(settingsPath, []byte(settingsTemplate), 0644)
	}

	content := string(data)

	// Already has github server configured
	if strings.Contains(content, "<id>github</id>") {
		return false, nil
	}

	// Has a <servers> section — inject into it
	if strings.Contains(content, "<servers>") {
		content = strings.Replace(content, "<servers>", "<servers>"+githubServerBlock, 1)
		return true, os.WriteFile(settingsPath, []byte(content), 0644)
	}

	// Has </settings> but no <servers> — add servers section
	if strings.Contains(content, "</settings>") {
		serversSection := "\n  <servers>" + githubServerBlock + "\n  </servers>\n"
		content = strings.Replace(content, "</settings>", serversSection+"</settings>", 1)
		return true, os.WriteFile(settingsPath, []byte(content), 0644)
	}

	// Fallback: file exists but doesn't look like valid settings.xml — rewrite
	return true, os.WriteFile(settingsPath, []byte(settingsTemplate), 0644)
}
