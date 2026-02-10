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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
	"github.com/fireflyframework/fireflyframework-cli/internal/git"
	"github.com/fireflyframework/fireflyframework-cli/internal/scaffold"
	"github.com/fireflyframework/fireflyframework-cli/internal/ui"
	"github.com/spf13/cobra"
)

var (
	flagArchetype   string
	flagGroupID     string
	flagArtifactID  string
	flagPackage     string
	flagDescription string
	flagVersion     string
	flagOutputDir   string
	flagNoGit       bool
)

var createCmd = &cobra.Command{
	Use:   "create [archetype]",
	Short: "Scaffold a new Firefly Framework project",
	Long: `Creates a new project from one of four YAML-driven archetypes. If no archetype
is provided as an argument, the CLI enters interactive mode with prompts for
archetype selection, group ID, artifact ID, package name, and description.

Available archetypes:
  core          Multi-module microservice with R2DBC, Flyway, MapStruct, OpenAPI SDK,
                and reactive services (5 modules: interfaces, models, core, sdk, web)
  domain        Multi-module CQRS/Saga microservice with transactional engine
                (5 modules: interfaces, infra, core, sdk, web)
  application   Single-module application with plugin architecture and Spring Security
  library       Single-module library with Spring Boot auto-configuration

For core, domain, and application archetypes, an infrastructure wizard prompts
for default values (server port, database host/port/name/user/password) that
are embedded into the generated application.yaml file. These values can be
overridden at runtime via environment variables.

Custom archetypes can be placed in ~/.flywork/archetypes/<name>.yaml to override
built-in archetypes or define new ones.

Examples:
  flywork create                                      Interactive mode
  flywork create core                                 Core archetype with prompts
  flywork create domain -g com.example -a my-service  Provide group and artifact IDs
  flywork create application -o ./output-dir          Custom output directory
  flywork create library --no-git                     Skip git init
  flywork create core --version 1.0.0                 Custom initial version`,
	Args:      cobra.MaximumNArgs(1),
	RunE:      runCreate,
	ValidArgs: scaffold.ListArchetypes(),
}

func init() {
	createCmd.Flags().StringVarP(&flagGroupID, "group-id", "g", "", "Maven groupId")
	createCmd.Flags().StringVarP(&flagArtifactID, "artifact-id", "a", "", "Maven artifactId")
	createCmd.Flags().StringVarP(&flagPackage, "package", "p", "", "Base Java package (derived from groupId if omitted)")
	createCmd.Flags().StringVarP(&flagDescription, "description", "d", "", "Project description")
	createCmd.Flags().StringVar(&flagVersion, "version", "0.0.1-SNAPSHOT", "Initial project version")
	createCmd.Flags().StringVarP(&flagOutputDir, "output", "o", "", "Output directory (defaults to artifactId)")
	createCmd.Flags().BoolVar(&flagNoGit, "no-git", false, "Skip git init")

	rootCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	p := ui.NewPrinter()
	reader := bufio.NewReader(os.Stdin)

	p.Header("Firefly Project Scaffolding")
	p.Newline()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Archetype selection
	archetypeName := flagArchetype
	if len(args) > 0 {
		archetypeName = args[0]
	}
	if archetypeName == "" {
		archetypeName, err = promptSelect(reader, p, "Select archetype", scaffold.ListArchetypes())
		if err != nil {
			return err
		}
	}

	arch, err := scaffold.LoadArchetype(archetypeName)
	if err != nil {
		return fmt.Errorf("unknown archetype %q — available: %s", archetypeName, strings.Join(scaffold.ListArchetypes(), ", "))
	}

	p.Info(fmt.Sprintf("Archetype: %s — %s", arch.Name, arch.Description))
	p.Newline()

	// Gather project metadata
	groupID := flagGroupID
	if groupID == "" {
		groupID, err = promptWithDefault(reader, p, "Group ID", cfg.DefaultGroup)
		if err != nil {
			return err
		}
	}

	artifactID := flagArtifactID
	if artifactID == "" {
		artifactID, err = promptRequired(reader, p, "Artifact ID (e.g. my-service)")
		if err != nil {
			return err
		}
	}

	basePackage := flagPackage
	if basePackage == "" {
		// Derive from groupId + sanitized artifactId
		defaultPkg := groupID + "." + sanitizePackage(artifactID)
		basePackage, err = promptWithDefault(reader, p, "Base package", defaultPkg)
		if err != nil {
			return err
		}
	}

	description := flagDescription
	if description == "" {
		defaultDesc := fmt.Sprintf("%s %s microservice", strings.Title(archetypeName), artifactID)
		description, err = promptWithDefault(reader, p, "Description", defaultDesc)
		if err != nil {
			return err
		}
	}

	version := flagVersion
	outputDir := flagOutputDir
	if outputDir == "" {
		outputDir = artifactID
	}

	// Resolve output path
	absOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}

	// Check if directory already exists
	if _, err := os.Stat(absOutput); err == nil {
		return fmt.Errorf("directory %s already exists — remove it first or use --output", absOutput)
	}

	// Default infrastructure values
	dbHost := "localhost"
	dbPort := "5432"
	dbName := strings.ReplaceAll(artifactID, "-", "_")
	dbUser := "postgres"
	dbPass := "postgres"
	serverPort := "8080"

	// Infrastructure wizard for archetypes that use a database or server
	if archetypeName == "core" || archetypeName == "domain" || archetypeName == "application" {
		p.Newline()
		p.Header("Infrastructure Defaults")
		p.Info("These values become the defaults in application.yaml (overridable via env vars at runtime)")
		p.Newline()

		serverPort, err = promptWithDefault(reader, p, "Server port", serverPort)
		if err != nil {
			return err
		}

		if archetypeName == "core" {
			dbHost, err = promptWithDefault(reader, p, "Database host", dbHost)
			if err != nil {
				return err
			}
			dbPort, err = promptWithDefault(reader, p, "Database port", dbPort)
			if err != nil {
				return err
			}
			dbName, err = promptWithDefault(reader, p, "Database name", dbName)
			if err != nil {
				return err
			}
			dbUser, err = promptWithDefault(reader, p, "Database user", dbUser)
			if err != nil {
				return err
			}
			dbPass, err = promptWithDefault(reader, p, "Database password", dbPass)
			if err != nil {
				return err
			}
		}
	}

	// Build project context
	modulePrefix := scaffold.ExportedPascalCase(artifactID)
	ctx := &scaffold.ProjectContext{
		ProjectName:          modulePrefix,
		ArtifactId:           artifactID,
		GroupId:              groupID,
		BasePackage:          basePackage,
		PackagePath:          strings.ReplaceAll(basePackage, ".", string(filepath.Separator)),
		Description:          description,
		Version:              version,
		JavaVersion:          cfg.JavaVersion,
		ParentGroupId:        arch.Parent.GroupID,
		ParentArtifactId:     arch.Parent.ArtifactID,
		ParentVersion:        cfg.ParentVersion,
		ApplicationClassName: "Application",
		ModulePrefix:         modulePrefix,
		ArchetypeName:        archetypeName,
		Year:                 fmt.Sprintf("%d", time.Now().Year()),
		DbHost:               dbHost,
		DbPort:               dbPort,
		DbName:               dbName,
		DbUser:               dbUser,
		DbPass:               dbPass,
		ServerPort:           serverPort,
	}

	// Confirm before generating
	p.Newline()
	p.Header("Project Summary")
	p.KeyValue("Archetype", archetypeName)
	p.KeyValue("Group ID", groupID)
	p.KeyValue("Artifact ID", artifactID)
	p.KeyValue("Package", basePackage)
	p.KeyValue("Version", version)
	p.KeyValue("Output", absOutput)
	p.Newline()

	confirm, err := promptConfirm(reader, p, "Generate project?")
	if err != nil {
		return err
	}
	if !confirm {
		p.Warning("Aborted.")
		return nil
	}

	// Generate
	p.Newline()
	spinner := ui.NewSpinner("Generating project structure...")
	spinner.Start()

	if err := scaffold.Generate(arch, ctx, absOutput); err != nil {
		spinner.Stop(false)
		return fmt.Errorf("generation failed: %w", err)
	}
	spinner.Stop(true)

	// Git init
	if !flagNoGit {
		spinner = ui.NewSpinner("Initializing git repository...")
		spinner.Start()
		if err := git.Init(absOutput); err != nil {
			spinner.Stop(false)
			p.Warning("git init failed: " + err.Error())
		} else {
			spinner.Stop(true)
		}
	}

	// Done
	p.Newline()
	p.Success(fmt.Sprintf("Project created at %s", absOutput))
	p.Newline()
	p.Info("Next steps:")
	p.Step(fmt.Sprintf("cd %s", outputDir))
	if archetypeName == "core" || archetypeName == "domain" {
		p.Step("mvn clean install")
	} else {
		p.Step("mvn clean package")
	}
	p.Step("flywork doctor")
	p.Newline()

	return nil
}

// --- Interactive prompt helpers ---

func promptRequired(reader *bufio.Reader, p *ui.Printer, label string) (string, error) {
	for {
		fmt.Printf("  %s %s: ", ui.StylePrimary.Render("?"), label)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		input = strings.TrimSpace(input)
		if input != "" {
			return input, nil
		}
		p.Warning("This field is required")
	}
}

func promptWithDefault(reader *bufio.Reader, p *ui.Printer, label, defaultVal string) (string, error) {
	defaultHint := ui.StyleMuted.Render(fmt.Sprintf(" (%s)", defaultVal))
	fmt.Printf("  %s %s%s: ", ui.StylePrimary.Render("?"), label, defaultHint)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal, nil
	}
	return input, nil
}

func promptSelect(reader *bufio.Reader, p *ui.Printer, label string, options []string) (string, error) {
	fmt.Printf("  %s %s:\n", ui.StylePrimary.Render("?"), label)
	for i, opt := range options {
		fmt.Printf("    %s %s\n", ui.StyleInfo.Render(fmt.Sprintf("[%d]", i+1)), opt)
	}
	for {
		fmt.Printf("  %s Choice: ", ui.StylePrimary.Render(">"))
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		input = strings.TrimSpace(input)

		// Accept number or name
		for i, opt := range options {
			if input == fmt.Sprintf("%d", i+1) || strings.EqualFold(input, opt) {
				return opt, nil
			}
		}
		p.Warning("Invalid selection — enter a number or archetype name")
	}
}

func promptConfirm(reader *bufio.Reader, p *ui.Printer, label string) (bool, error) {
	defaultHint := ui.StyleMuted.Render(" (Y/n)")
	fmt.Printf("  %s %s%s: ", ui.StylePrimary.Render("?"), label, defaultHint)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "" || input == "y" || input == "yes", nil
}

// --- Helpers ---

func sanitizePackage(artifactID string) string {
	s := strings.ReplaceAll(artifactID, "-", ".")
	s = strings.ReplaceAll(s, "_", ".")
	return strings.ToLower(s)
}
