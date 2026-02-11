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

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	FireflyDir  = ".flywork"
	ConfigFile  = "config.yaml"
	ReposDir    = "repos"
	ArchetypDir = "archetypes"
)

// ValidKeys lists all config keys that can be read/written.
var ValidKeys = []string{
	"repos_path",
	"github_org",
	"default_group_id",
	"java_version",
	"parent_version",
	"cli_auto_update",
	"branch",
}

type Config struct {
	ReposPath     string `yaml:"repos_path"`
	GithubOrg     string `yaml:"github_org"`
	DefaultGroup  string `yaml:"default_group_id"`
	JavaVersion   string `yaml:"java_version"`
	ParentVersion string `yaml:"parent_version"`
	CLIAutoUpdate bool   `yaml:"cli_auto_update"`
	Branch        string `yaml:"branch"`
}

// GetField returns the value of a config key.
func (c *Config) GetField(key string) (string, bool) {
	switch key {
	case "repos_path":
		return c.ReposPath, true
	case "github_org":
		return c.GithubOrg, true
	case "default_group_id":
		return c.DefaultGroup, true
	case "java_version":
		return c.JavaVersion, true
	case "parent_version":
		return c.ParentVersion, true
	case "cli_auto_update":
		if c.CLIAutoUpdate {
			return "true", true
		}
		return "false", true
	case "branch":
		return c.Branch, true
	default:
		return "", false
	}
}

// SetField sets the value of a config key.
func (c *Config) SetField(key, value string) bool {
	switch key {
	case "repos_path":
		c.ReposPath = value
	case "github_org":
		c.GithubOrg = value
	case "default_group_id":
		c.DefaultGroup = value
	case "java_version":
		c.JavaVersion = value
	case "parent_version":
		c.ParentVersion = value
	case "cli_auto_update":
		c.CLIAutoUpdate = value == "true" || value == "1" || value == "yes"
	case "branch":
		c.Branch = value
	default:
		return false
	}
	return true
}

// Fields returns all config key-value pairs.
func (c *Config) Fields() []KeyValue {
	return []KeyValue{
		{"repos_path", c.ReposPath},
		{"github_org", c.GithubOrg},
		{"default_group_id", c.DefaultGroup},
		{"java_version", c.JavaVersion},
		{"parent_version", c.ParentVersion},
		{"cli_auto_update", fmt.Sprintf("%v", c.CLIAutoUpdate)},
		{"branch", c.Branch},
	}
}

// KeyValue is a simple key-value pair.
type KeyValue struct {
	Key   string
	Value string
}

func DefaultConfig() *Config {
	return &Config{
		ReposPath:     filepath.Join(HomeDir(), FireflyDir, ReposDir),
		GithubOrg:     "fireflyframework",
		DefaultGroup:  "org.fireflyframework",
		JavaVersion:   "25",
		ParentVersion: "26.02.01",
		Branch:        "develop",
	}
}

func HomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func FlyworkHome() string {
	return filepath.Join(HomeDir(), FireflyDir)
}

func Load() (*Config, error) {
	cfg := DefaultConfig()
	path := filepath.Join(FlyworkHome(), ConfigFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Save() error {
	dir := FlyworkHome()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, ConfigFile), data, 0644)
}
