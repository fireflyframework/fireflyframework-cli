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

package version

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FindAllPoms finds nested framework POMs while excluding build output and
// dependency directories. Git operations still run only at the checkout root.
func FindAllPoms(repoDir string) []string {
	var poms []string
	_ = filepath.WalkDir(repoDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "target", ".git", "node_modules", ".venv":
				return filepath.SkipDir
			}
		} else if entry.Name() == "pom.xml" && entry.Type().IsRegular() {
			poms = append(poms, path)
		}
		return nil
	})
	return poms
}

// ReplacePomVersion replaces all occurrences of oldVer with newVer in the given file.
// This is a simple string replacement which is safe for framework version strings
// (e.g. "1.0.0-SNAPSHOT" or "26.02.03") that don't collide with library versions.
func ReplacePomVersion(pomPath, oldVer, newVer string) error {
	data, err := os.ReadFile(pomPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", pomPath, err)
	}

	content := string(data)
	if !strings.Contains(content, oldVer) {
		return nil // nothing to replace
	}

	updated := strings.ReplaceAll(content, oldVer, newVer)
	return os.WriteFile(pomPath, []byte(updated), 0644)
}

type pomReference struct {
	GroupID string `xml:"groupId"`
	Version string `xml:"version"`
}

type pomDocument struct {
	GroupID             string         `xml:"groupId"`
	Version             string         `xml:"version"`
	Parent              pomReference   `xml:"parent"`
	Dependencies        []pomReference `xml:"dependencies>dependency"`
	ManagedDependencies []pomReference `xml:"dependencyManagement>dependencies>dependency"`
	Properties          struct {
		Values []struct {
			XMLName xml.Name
			Value   string `xml:",chardata"`
		} `xml:",any"`
	} `xml:"properties"`
}

func readPomDocument(pomPath string) (*pomDocument, error) {
	data, err := os.ReadFile(pomPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", pomPath, err)
	}
	var pom pomDocument
	if err := xml.Unmarshal(data, &pom); err != nil {
		return nil, fmt.Errorf("parse %s: %w", pomPath, err)
	}
	return &pom, nil
}

func (pom *pomDocument) version() string {
	if ver := strings.TrimSpace(pom.Version); ver != "" {
		return ver
	}
	return strings.TrimSpace(pom.Parent.Version)
}

func frameworkGroup(group string) bool {
	group = strings.TrimSpace(group)
	return group == "org.fireflyframework" || strings.HasPrefix(group, "org.fireflyframework.")
}

// ReadPomVersion returns the project's explicit version, or its inherited
// parent version when the project omits its own version.
func ReadPomVersion(pomPath string) (string, error) {
	pom, err := readPomDocument(pomPath)
	if err != nil {
		return "", err
	}
	if ver := pom.version(); ver != "" {
		return ver, nil
	}
	return "", fmt.Errorf("no <version> found in %s", pomPath)
}

func frameworkVersions(pomPath, train string) ([]string, error) {
	pom, err := readPomDocument(pomPath)
	if err != nil {
		return nil, err
	}
	group := strings.TrimSpace(pom.GroupID)
	if group == "" {
		group = pom.Parent.GroupID
	}
	var versions []string
	if frameworkGroup(group) {
		if ver := pom.version(); ver != "" {
			versions = append(versions, ver)
		} else {
			return nil, fmt.Errorf("no framework version found in %s", pomPath)
		}
	}
	properties := map[string]string{
		"project.version": pom.version(), "pom.version": pom.version(),
		"project.parent.version": pom.Parent.Version, "parent.version": pom.Parent.Version,
		"fireflyframework.version": train,
	}
	for _, property := range pom.Properties.Values {
		properties[property.XMLName.Local] = strings.TrimSpace(property.Value)
	}
	propertyReference := regexp.MustCompile(`\$\{([^}]+)\}`)
	references := append([]pomReference{pom.Parent}, pom.Dependencies...)
	references = append(references, pom.ManagedDependencies...)
	for _, reference := range references {
		if !frameworkGroup(reference.GroupID) {
			continue
		}
		ver := strings.TrimSpace(reference.Version)
		for attempt := 0; attempt < 10; attempt++ {
			resolved := propertyReference.ReplaceAllStringFunc(ver, func(reference string) string {
				if value, ok := properties[reference[2:len(reference)-1]]; ok {
					return value
				}
				return reference
			})
			if resolved == ver {
				break
			}
			ver = resolved
		}
		if ver != "" && !strings.Contains(ver, "${") {
			versions = append(versions, ver)
		}
	}
	return versions, nil
}
