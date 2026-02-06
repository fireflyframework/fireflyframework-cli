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

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Placeholder represents a config placeholder found in application config files.
type Placeholder struct {
	Key        string // e.g. DB_HOST
	Property   string // e.g. spring.r2dbc.url
	Default    string // default value if specified in ${VAR:default}
	File       string // source file
	HasDefault bool
}

// ProjectInfo holds detected metadata about the project.
type ProjectInfo struct {
	Name        string   // artifactId from root pom.xml
	Archetype   string   // core, domain, application, library, or unknown
	MultiModule bool     // whether the project has sub-modules
	Modules     []string // sub-module directory names (e.g. test-web, test-core)
	WebModule   string   // resolved web module path
	Profiles    []string // detected Spring profiles
	ConfigFiles []string // config file names found in the web module
}

// AnalyzeProject builds a full ProjectInfo for the given directory.
func AnalyzeProject(dir string) (*ProjectInfo, error) {
	isBoot, err := DetectSpringBoot(dir)
	if err != nil {
		return nil, err
	}
	if !isBoot {
		return nil, fmt.Errorf("no Spring Boot / Firefly Framework project found")
	}

	info := &ProjectInfo{
		Name: detectProjectName(dir),
	}

	// Detect sub-modules
	info.Modules = detectSubModules(dir)
	info.MultiModule = len(info.Modules) > 0

	// Detect archetype
	info.Archetype = detectArchetype(dir, info.Modules)

	// Find web module
	info.WebModule = FindWebModule(dir)

	// Detect profiles
	info.Profiles = DetectProfiles(info.WebModule)

	// Detect config files
	info.ConfigFiles = detectConfigFiles(info.WebModule)

	return info, nil
}

// detectProjectName extracts the artifactId from the root pom.xml.
func detectProjectName(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "pom.xml"))
	if err != nil {
		return filepath.Base(dir)
	}
	content := string(data)
	// Simple regex to grab <artifactId> — first occurrence after <parent> block
	// We look for the artifactId that is NOT inside <parent>...</parent>
	re := regexp.MustCompile(`(?s)<parent>.*?</parent>`)
	withoutParent := re.ReplaceAllString(content, "")
	artRe := regexp.MustCompile(`<artifactId>([^<]+)</artifactId>`)
	if m := artRe.FindStringSubmatch(withoutParent); len(m) > 1 {
		return m[1]
	}
	return filepath.Base(dir)
}

// detectSubModules returns sub-module directory names from the root pom.xml <modules> section.
func detectSubModules(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, "pom.xml"))
	if err != nil {
		return nil
	}
	re := regexp.MustCompile(`<module>([^<]+)</module>`)
	matches := re.FindAllStringSubmatch(string(data), -1)
	var modules []string
	for _, m := range matches {
		modules = append(modules, m[1])
	}
	sort.Strings(modules)
	return modules
}

// detectArchetype identifies the project archetype based on its structure.
func detectArchetype(dir string, modules []string) string {
	if len(modules) > 0 {
		// Multi-module: distinguish core vs domain
		hasModels := false
		hasInfra := false
		for _, m := range modules {
			if strings.HasSuffix(m, "-models") {
				hasModels = true
			}
			if strings.HasSuffix(m, "-infra") {
				hasInfra = true
			}
		}
		if hasModels {
			return "core"
		}
		if hasInfra {
			return "domain"
		}
		return "unknown"
	}

	// Single-module
	pomData, _ := os.ReadFile(filepath.Join(dir, "pom.xml"))
	pomContent := string(pomData)

	// Library: has AutoConfiguration.imports
	autoConfigPath := filepath.Join(dir, "src", "main", "resources", "META-INF", "spring",
		"org.springframework.boot.autoconfigure.AutoConfiguration.imports")
	if _, err := os.Stat(autoConfigPath); err == nil {
		return "library"
	}

	// Application: uses fireflyframework-application
	if strings.Contains(pomContent, "fireflyframework-application") {
		return "application"
	}

	// Generic Spring Boot
	if strings.Contains(pomContent, "spring-boot") {
		return "unknown"
	}

	return "unknown"
}

// detectConfigFiles returns the config file names found in the resources directory.
func detectConfigFiles(moduleDir string) []string {
	resourceDir := filepath.Join(moduleDir, "src", "main", "resources")
	entries, err := os.ReadDir(resourceDir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "application") {
			ext := filepath.Ext(name)
			if ext == ".yaml" || ext == ".yml" || ext == ".properties" {
				files = append(files, name)
			}
		}
	}
	sort.Strings(files)
	return files
}

// DetectSpringBoot checks if the given directory contains a Spring Boot Maven project.
func DetectSpringBoot(dir string) (bool, error) {
	pomPath := filepath.Join(dir, "pom.xml")
	data, err := os.ReadFile(pomPath)
	if err != nil {
		return false, nil
	}
	content := string(data)
	return strings.Contains(content, "spring-boot") || strings.Contains(content, "fireflyframework"), nil
}

// FindWebModule searches for the Spring Boot web module in a multi-module project.
// Returns the module directory that contains application.yaml/yml, or "." for single-module.
func FindWebModule(dir string) string {
	// Check root for application config
	if hasAppConfig(dir) {
		return dir
	}

	// Search *-web module
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dir
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), "-web") {
			candidate := filepath.Join(dir, e.Name())
			if hasAppConfig(candidate) {
				return candidate
			}
		}
	}

	// Search any module that has application config
	for _, e := range entries {
		if e.IsDir() {
			candidate := filepath.Join(dir, e.Name())
			if hasAppConfig(candidate) {
				return candidate
			}
		}
	}

	return dir
}

func hasAppConfig(dir string) bool {
	for _, name := range configFileNames() {
		if _, err := os.Stat(filepath.Join(dir, "src", "main", "resources", name)); err == nil {
			return true
		}
	}
	return false
}

func configFileNames() []string {
	return []string{"application.yaml", "application.yml", "application.properties"}
}

// ScanPlaceholders reads config files and extracts ${VAR} or ${VAR:default} placeholders.
func ScanPlaceholders(moduleDir string) ([]Placeholder, error) {
	resourceDir := filepath.Join(moduleDir, "src", "main", "resources")
	var placeholders []Placeholder
	seen := make(map[string]bool)

	// Regex: ${VAR} or ${VAR:default}
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_.]*?)(?::([^}]*))?\}`)

	for _, name := range configFileNames() {
		path := filepath.Join(resourceDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			matches := re.FindAllStringSubmatch(line, -1)
			for _, m := range matches {
				key := m[1]
				// Skip Maven resource filtering vars and standard Spring internal vars
				if strings.HasPrefix(key, "project.") || key == "java.version" ||
					strings.HasPrefix(key, "maven.") || key == "spring.profiles.active" {
					continue
				}

				if seen[key] {
					continue
				}
				seen[key] = true

				p := Placeholder{
					Key:  key,
					File: name,
				}
				if len(m) > 2 && m[2] != "" {
					p.Default = m[2]
					p.HasDefault = true
				}
				placeholders = append(placeholders, p)
			}
		}
	}

	sort.Slice(placeholders, func(i, j int) bool {
		return placeholders[i].Key < placeholders[j].Key
	})

	return placeholders, nil
}

// FindEnvSetVars returns placeholders whose env var is already set in the environment.
func FindEnvSetVars(placeholders []Placeholder) []Placeholder {
	var set []Placeholder
	for _, p := range placeholders {
		if os.Getenv(p.Key) != "" {
			set = append(set, p)
		}
	}
	return set
}

// FindMissingEnvVars returns placeholders whose env var is not set and has no default.
func FindMissingEnvVars(placeholders []Placeholder) []Placeholder {
	var missing []Placeholder
	for _, p := range placeholders {
		if p.HasDefault {
			continue
		}
		if os.Getenv(p.Key) == "" {
			missing = append(missing, p)
		}
	}
	return missing
}

// FindDefaultedVars returns placeholders that have a default value and are not already
// overridden by an environment variable.
func FindDefaultedVars(placeholders []Placeholder) []Placeholder {
	var defaulted []Placeholder
	for _, p := range placeholders {
		if p.HasDefault && os.Getenv(p.Key) == "" {
			defaulted = append(defaulted, p)
		}
	}
	return defaulted
}

// RunSpringBoot executes mvn spring-boot:run with optional -D properties and env overrides.
func RunSpringBoot(moduleDir string, profiles string, envOverrides map[string]string) error {
	args := []string{"spring-boot:run"}

	if profiles != "" {
		args = append(args, fmt.Sprintf("-Dspring-boot.run.profiles=%s", profiles))
	}

	// Pass env overrides as spring-boot.run.jvmArguments
	if len(envOverrides) > 0 {
		var jvmArgs []string
		for k, v := range envOverrides {
			jvmArgs = append(jvmArgs, fmt.Sprintf("-D%s=%s", k, v))
		}
		sort.Strings(jvmArgs)
		args = append(args, fmt.Sprintf(`-Dspring-boot.run.jvmArguments=%s`, strings.Join(jvmArgs, " ")))
	}

	cmd := exec.Command("mvn", args...)
	cmd.Dir = moduleDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Set env vars
	cmd.Env = os.Environ()
	for k, v := range envOverrides {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	return cmd.Run()
}

// DetectProfiles scans application-{profile}.yaml/yml files.
func DetectProfiles(moduleDir string) []string {
	resourceDir := filepath.Join(moduleDir, "src", "main", "resources")
	entries, err := os.ReadDir(resourceDir)
	if err != nil {
		return nil
	}

	var profiles []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "application-") {
			ext := filepath.Ext(name)
			if ext == ".yaml" || ext == ".yml" || ext == ".properties" {
				profile := strings.TrimSuffix(strings.TrimPrefix(name, "application-"), ext)
				if profile != "" {
					profiles = append(profiles, profile)
				}
			}
		}
	}
	sort.Strings(profiles)
	return profiles
}
