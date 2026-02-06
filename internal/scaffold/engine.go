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

package scaffold

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

//go:embed archetypes/*.yaml
var archetypeFS embed.FS

//go:embed templates/*
var templateFS embed.FS

// Archetype represents a project archetype definition loaded from YAML.
type Archetype struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	MultiModule bool      `yaml:"multiModule"`
	Parent      Parent    `yaml:"parent"`
	Modules     []Module  `yaml:"modules"`
	// Single-module fields
	Dependencies     []Dep      `yaml:"dependencies"`
	TestDependencies []Dep      `yaml:"testDependencies"`
	Plugins          []string   `yaml:"plugins"`
	Packages         []string   `yaml:"packages"`
	Annotations      []string   `yaml:"annotations"`
	RootTemplates    []Template `yaml:"rootTemplates"`
}

type Parent struct {
	GroupID    string `yaml:"groupId"`
	ArtifactID string `yaml:"artifactId"`
	Version    string `yaml:"version"`
}

type Module struct {
	Suffix           string     `yaml:"suffix"`
	Description      string     `yaml:"description"`
	Packages         []string   `yaml:"packages"`
	Dependencies     []Dep      `yaml:"dependencies"`
	TestDependencies []Dep      `yaml:"testDependencies"`
	Plugins          []string   `yaml:"plugins"`
	SpringBoot       bool       `yaml:"springBoot"`
	Annotations      []string   `yaml:"annotations"`
	Templates        []Template `yaml:"templates"`
}

type Dep struct {
	GroupID    string `yaml:"groupId"`
	ArtifactID string `yaml:"artifactId"`
	Scope      string `yaml:"scope"`
	Optional   bool   `yaml:"optional"`
	Internal   bool   `yaml:"internal"`
}

type Template struct {
	Src  string `yaml:"src"`
	Dest string `yaml:"dest"`
}

// ProjectContext holds all template variables for rendering.
type ProjectContext struct {
	ProjectName          string
	ArtifactId           string
	GroupId              string
	BasePackage          string
	PackagePath          string
	Description          string
	Version              string
	JavaVersion          string
	ParentGroupId        string
	ParentArtifactId     string
	ParentVersion        string
	ApplicationClassName string
	ModulePrefix         string // PascalCase prefix for class names
	ArchetypeName        string
	Year                 string
	// Infrastructure defaults (used in application.yaml placeholders)
	DbHost     string
	DbPort     string
	DbName     string
	DbUser     string
	DbPass     string
	ServerPort string
}

// LoadArchetype loads an archetype YAML by name. It first checks ~/.flywork/archetypes/
// for user overrides, then falls back to the embedded defaults.
func LoadArchetype(name string) (*Archetype, error) {
	// Try user override first
	home, _ := os.UserHomeDir()
	userPath := filepath.Join(home, ".flywork", "archetypes", name+".yaml")
	if data, err := os.ReadFile(userPath); err == nil {
		var arch Archetype
		if err := yaml.Unmarshal(data, &arch); err != nil {
			return nil, fmt.Errorf("invalid user archetype %s: %w", userPath, err)
		}
		return &arch, nil
	}

	// Fall back to embedded
	data, err := archetypeFS.ReadFile("archetypes/" + name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("unknown archetype %q", name)
	}
	var arch Archetype
	if err := yaml.Unmarshal(data, &arch); err != nil {
		return nil, fmt.Errorf("invalid embedded archetype %s: %w", name, err)
	}
	return &arch, nil
}

// ListArchetypes returns the names of all available archetypes.
func ListArchetypes() []string {
	return []string{"core", "domain", "application", "library"}
}

// Generate creates a full project from an archetype and context.
func Generate(arch *Archetype, ctx *ProjectContext, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("cannot create output dir: %w", err)
	}

	// Render root-level templates
	for _, t := range arch.RootTemplates {
		dest := resolveTemplatePath(t.Dest, ctx)
		if err := renderTemplate(t.Src, filepath.Join(outputDir, dest), ctx); err != nil {
			return fmt.Errorf("rendering %s: %w", t.Src, err)
		}
	}

	// Render module templates (multi-module)
	if arch.MultiModule {
		for _, mod := range arch.Modules {
			moduleDir := filepath.Join(outputDir, ctx.ArtifactId+"-"+mod.Suffix)
			if err := os.MkdirAll(moduleDir, 0755); err != nil {
				return fmt.Errorf("cannot create module dir %s: %w", mod.Suffix, err)
			}

			// Create package directories
			for _, pkg := range mod.Packages {
				pkgDir := filepath.Join(moduleDir, "src", "main", "java",
					strings.ReplaceAll(ctx.BasePackage, ".", string(filepath.Separator)),
					mod.Suffix, pkg)
				os.MkdirAll(pkgDir, 0755)
			}

			for _, t := range mod.Templates {
				dest := resolveTemplatePath(t.Dest, ctx)
				if err := renderTemplate(t.Src, filepath.Join(moduleDir, dest), ctx); err != nil {
					return fmt.Errorf("rendering %s for module %s: %w", t.Src, mod.Suffix, err)
				}
			}
		}
	} else {
		// Single-module: create package directories
		for _, pkg := range arch.Packages {
			pkgDir := filepath.Join(outputDir, "src", "main", "java",
				strings.ReplaceAll(ctx.BasePackage, ".", string(filepath.Separator)),
				pkg)
			os.MkdirAll(pkgDir, 0755)
		}
	}

	return nil
}

func resolveTemplatePath(dest string, ctx *ProjectContext) string {
	dest = strings.ReplaceAll(dest, "{{.PackagePath}}", strings.ReplaceAll(ctx.BasePackage, ".", string(filepath.Separator)))
	dest = strings.ReplaceAll(dest, "{{.ApplicationClassName}}", ctx.ApplicationClassName)
	dest = strings.ReplaceAll(dest, "{{.ModulePrefix}}", ctx.ModulePrefix)
	return dest
}

func renderTemplate(src, destPath string, ctx *ProjectContext) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	data, err := templateFS.ReadFile("templates/" + src)
	if err != nil {
		return fmt.Errorf("template not found: %s", src)
	}

	funcMap := template.FuncMap{
		"lower":    strings.ToLower,
		"upper":    strings.ToUpper,
		"title":    strings.Title,
		"replace": func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
		"contains": strings.Contains,
		"trimSuffix": strings.TrimSuffix,
		"lastSegment": func(s, sep string) string {
			parts := strings.Split(s, sep)
			return parts[len(parts)-1]
		},
		"toPascalCase": toPascalCase,
		"toCamelCase":  toCamelCase,
	}

	tmpl, err := template.New(src).Funcs(funcMap).Parse(string(data))
	if err != nil {
		return fmt.Errorf("parse template %s: %w", src, err)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, ctx)
}

// ExportedPascalCase converts a kebab/snake/dot-separated string to PascalCase.
func ExportedPascalCase(s string) string {
	return toPascalCase(s)
}

func toPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	var result strings.Builder
	for _, p := range parts {
		if len(p) > 0 {
			result.WriteString(strings.ToUpper(p[:1]) + p[1:])
		}
	}
	return result.String()
}

func toCamelCase(s string) string {
	pascal := toPascalCase(s)
	if len(pascal) == 0 {
		return pascal
	}
	return strings.ToLower(pascal[:1]) + pascal[1:]
}
