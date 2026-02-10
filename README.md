# Firefly Framework CLI

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)]()

```
  _____.__                _____.__
_/ ____\__|______   _____/ ____\  | ___.__.
\   __\|  \_  __ \_/ __ \   __\|  |<   |  |
 |  |  |  ||  | \/\  ___/|  |  |  |_\___  |
 |__|  |__||__|    \___  >__|  |____/ ____|
                       \/           \/
  _____                                                 __
_/ ____\___________    _____   ______  _  _____________|  | __
\   __\_  __ \__  \  /     \_/ __ \ \/ \/ /  _ \_  __ \  |/ /
 |  |   |  | \// __ \|  Y Y  \  ___/\     (  <_> )  | \/    <
 |__|   |__|  (____  /__|_|  /\___  >\/\_/ \____/|__|  |__|_ \
                   \/      \/     \/                        \/
```

The official command-line interface for the **Firefly Framework** — scaffold, setup, diagnose, and manage your Firefly-based Java microservices with a world-class developer experience.

---

## Quick Install

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/fireflyframework/fireflyframework-cli/main/install.sh | bash
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/fireflyframework/fireflyframework-cli/main/install.ps1 | iex
```

### From Source

```bash
git clone https://github.com/fireflyframework/fireflyframework-cli.git
cd fireflyframework-cli
make install
```

---

## Commands

### `flywork setup`

Bootstraps the entire Firefly Framework into your local environment. Clones all **34 framework repositories** in **DAG-resolved dependency order** and installs them to your local Maven cache (`~/.m2`).

The CLI resolves a dependency graph across all repositories, groups them into layers, and processes each layer sequentially to guarantee correct compilation order. Progress is shown with real-time progress bars, per-repo spinners with elapsed time, and a final summary box.

```bash
flywork setup                  # full setup (prompts whether to run tests)
flywork setup --skip-tests     # skip tests during Maven install
flywork setup -v               # verbose: show DAG layers and per-repo status
```

When `--skip-tests` is not provided, the CLI interactively asks whether to run tests (default: **yes**).

**What it does:**

1. Resolves the dependency DAG (34 repos across 6 layers)
2. **Phase 1** — Clones all repos layer-by-layer with a live progress bar
3. **Phase 2** — Runs `mvn clean install` on each repo in dependency order (with or without tests), with per-repo spinners showing elapsed time
4. Displays a summary box with total time, repos cloned/installed, and layer count

### `flywork create [archetype]`

Scaffolds a new project from one of four YAML-driven archetypes:

| Archetype       | Description                                                        |
|-----------------|--------------------------------------------------------------------|
| **core**        | Multi-module microservice with R2DBC, Flyway, MapStruct, OpenAPI SDK |
| **domain**      | Multi-module CQRS/Saga microservice with transactional engine        |
| **application** | Single-module application with plugin architecture                   |
| **library**     | Single-module library with Spring Boot auto-configuration            |

```bash
flywork create core
flywork create domain --group-id com.example --artifact-id my-service
flywork create application -g com.example -a my-app -d "My Application"
flywork create library -g com.example -a my-lib --no-git
```

If no flags are provided, the CLI enters interactive mode with prompts for archetype selection, group ID, artifact ID, package name, and description.

### `flywork doctor`

Runs diagnostic checks against your environment:

- Java version detection (configurable)
- JAVA_HOME resolution
- Maven version and Java compatibility
- Git installation
- Framework repositories cloned status
- Parent POM / BOM presence in `~/.m2`
- Project structure validation
- CLI version check

```bash
flywork doctor
```

### `flywork update`

Pulls the latest changes for all framework repositories and reinstalls them to `.m2`, respecting DAG dependency order.

```bash
flywork update                                        # pull + install all (prompts for tests)
flywork update --skip-tests                           # pull + install without tests
flywork update --pull-only                            # only git pull, skip maven
flywork update --repo fireflyframework-utils          # single repo
flywork update -v                                     # verbose with layer info
```

When `--skip-tests` is not provided (and not `--pull-only`), the CLI interactively asks whether to run tests (default: **yes**).

The update command uses the same DAG resolver as `setup`, with two distinct phases:

1. **Phase 1** — Git pull with progress bar
2. **Phase 2** — Maven install with per-repo spinners and elapsed time

### `flywork build`

Smart DAG-aware build with SHA-based change detection. Detects which repos have changed since the last successful build, computes the transitive closure of affected downstream repos, and builds them in dependency order.

```bash
flywork build                  # build changed repos + affected dependents
flywork build --all            # rebuild everything (ignore change detection)
flywork build --repo <name>    # build a specific repo and its dependents
flywork build --dry-run        # show what would be built without building
flywork build --skip-tests     # skip running tests during Maven install
flywork build --jdk /path      # use an explicit JAVA_HOME
```

**Phases:**

1. **Preflight** — Verifies Git, Maven, and Java are installed
2. **Change Detection** — Compares HEAD SHAs against the last-build manifest (`~/.flywork/build-manifest.json`) and computes transitive closure over the DAG
3. **Build Plan** — Shows affected repos grouped by layer, marks directly changed repos with `*`
4. **DAG Build** — Runs `mvn clean install` layer-by-layer with progress bars and per-repo spinners
5. **Summary** — Reports built/skipped/failed counts, total time, and log locations for failures

### `flywork publish`

Publishes Maven artifacts to GitHub Packages in DAG-resolved order. Uses the same change detection as `build` to only publish what has changed.

```bash
flywork publish                # publish changed repos
flywork publish --all          # publish everything
flywork publish --repo <name>  # publish a specific repo
flywork publish --dry-run      # show what would be published
flywork publish --skip-tests   # skip tests during deploy (default: true)
flywork publish --jdk /path    # use an explicit JAVA_HOME
```

Requires `GITHUB_TOKEN` environment variable set with `write:packages` scope.

**Phases:**

1. **Preflight** — Verifies `GITHUB_TOKEN`, Git, Maven, and Java
2. **Maven Settings** — Ensures `~/.m2/settings.xml` contains the GitHub Packages server configuration
3. **Publish Plan** — Shows repos to publish grouped by layer
4. **Maven Deploy** — Runs `mvn deploy` layer-by-layer with progress bars
5. **Summary** — Reports published/skipped/failed counts and total time

### `flywork dag`

Inspect and query the framework dependency graph. Useful for understanding build order, debugging dependency issues, and CI/CD integration.

```bash
flywork dag show                          # display full graph as ASCII tree
flywork dag layers                        # show repos grouped by build layer
flywork dag affected --from <repo>        # compute transitive closure of affected repos
flywork dag affected --from <repo> --json # machine-readable output
flywork dag export                        # export full DAG as JSON for CI/CD consumption
```

**Subcommands:**

| Subcommand | Description |
|------------|-------------|
| `show` | Full dependency graph with arrows showing dependencies |
| `layers` | Repos grouped by build layer (0 = no dependencies) |
| `affected` | Transitive closure of repos affected by a change in `--from` |
| `export` | JSON export of the entire DAG (nodes, edges, layers) |

### `flywork fwversion`

Manage framework-wide CalVer versions across all repositories. CalVer format: `YY.MM.PP` (e.g., `26.01.01`).

```bash
flywork fwversion show                    # show current version across all repos
flywork fwversion bump --auto             # auto-compute next CalVer and bump all POMs
flywork fwversion bump --auto --push      # bump, commit, tag, and push
flywork fwversion bump --dry-run          # preview changes without modifying files
flywork fwversion bump --install          # bump + run mvn install after
flywork fwversion check                   # validate version consistency across repos
flywork fwversion families                # show version family release history
```

**Subcommands:**

| Subcommand | Description |
|------------|-------------|
| `show` | Displays current POM versions, mismatches, dirty trees, and config alignment |
| `bump` | Updates all `pom.xml` files across every repo, optionally commits, tags, and pushes |
| `check` | Runs consistency checks: POM versions, config match, git tags, clean trees, `.m2` artifacts |
| `families` | Shows version family history — each bump records a snapshot of module SHAs |

**Bump flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--auto` | `false` | Auto-compute next CalVer from current version |
| `--commit` | `true` | Git commit the version changes |
| `--tag` | `true` | Git tag each repo with `v<version>` |
| `--push` | `false` | Git push after commit/tag |
| `--dry-run` | `false` | Show changes without modifying files |
| `--install` | `false` | Run `mvn install` in all repos after bumping |

### `flywork upgrade`

Self-updates the CLI binary from GitHub releases.

```bash
flywork upgrade            # download and install latest version
flywork upgrade --check    # just check if an update is available
```

### `flywork config`

View and manage CLI configuration stored in `~/.flywork/config.yaml`.

```bash
flywork config                          # show all configuration
flywork config get java_version         # get a single value
flywork config set java_version 25      # set a value
flywork config set parent_version 1.0.0-SNAPSHOT
flywork config reset                    # reset to defaults
```

### `flywork version`

Prints CLI version, git commit, build date, Go version, and OS/architecture.

```bash
flywork version
```

---

## DAG Dependency Resolution

The CLI maintains an internal **directed acyclic graph** of all 38 framework repositories with their real Maven dependency relationships. This ensures:

- **Correct build order** — repositories are always compiled after their dependencies
- **Layer grouping** — independent repos are grouped into layers for potential parallelization
- **Cycle detection** — the DAG engine detects and reports circular dependencies with the exact cycle path

**Dependency layers:**

| Layer | Repositories |
|-------|-------------|
| 0 (1) | `parent` |
| 1 (11) | `bom`, `utils`, `cache`, `eda`, `ecm`, `idp`, `config-server`, `client`, `validators`, `plugins`, `transactional-engine` |
| 2 (11) | `r2dbc`, `cqrs`, `web`, `workflow`, `ecm-esignature-adobe-sign`, `ecm-esignature-docusign`, `ecm-esignature-logalty`, `ecm-storage-aws`, `ecm-storage-azure`, `idp-aws-cognito`, `idp-keycloak` |
| 3 (6) | `eventsourcing`, `application`, `idp-internal-db`, `core`, `domain`, `data` |
| 4 (5) | `webhooks`, `callbacks`, `notifications`, `rule-engine`, `backoffice` |
| 5 (4) | `notifications-firebase`, `notifications-resend`, `notifications-sendgrid`, `notifications-twilio` |

---

## Configuration

Configuration is stored in `~/.flywork/config.yaml`.

| Key | Default | Description |
|-----|---------|-------------|
| `repos_path` | `~/.flywork/repos` | Where framework repos are cloned |
| `github_org` | `fireflyframework` | GitHub organization name |
| `default_group_id` | `org.fireflyframework` | Default Maven groupId for new projects |
| `java_version` | `25` | Target Java version for compilation |
| `parent_version` | `1.0.0-SNAPSHOT` | Parent POM version for archetypes |
| `cli_auto_update` | `false` | Auto-check for CLI updates on launch |

### Dynamic Java Version

The CLI automatically detects installed Java versions:

- **macOS** — Uses `/usr/libexec/java_home -v <version>`
- **Linux** — Scans `/usr/lib/jvm/` and `update-alternatives`
- **Windows** — Scans `Program Files\Java`, `Eclipse Adoptium`, `Corretto`, etc.

Change the required Java version:

```bash
flywork config set java_version 25
```

All `setup` and `update` commands will automatically resolve the correct `JAVA_HOME` for that version.

---

## Archetype System

### Built-in Archetypes

Archetypes are YAML-driven definitions embedded in the CLI binary. Each archetype defines:

- **Module structure** — multi-module or single-module layout
- **Parent POM** — inherited from `fireflyframework-parent`
- **Dependencies** — per-module Maven dependencies
- **Plugins** — Maven plugins (Spring Boot, MapStruct, OpenAPI Generator, etc.)
- **Templates** — Go template files rendered with project metadata
- **Package layout** — Java package directories to create

### Customizing Archetypes

You can override any built-in archetype or create new ones by placing YAML files in:

```
~/.flywork/archetypes/
```

For example, to customize the `core` archetype:

```bash
mkdir -p ~/.flywork/archetypes
vim ~/.flywork/archetypes/core.yaml
```

The YAML structure follows this schema:

```yaml
name: my-archetype
description: Description of the archetype
multiModule: true

parent:
  groupId: org.fireflyframework
  artifactId: fireflyframework-parent
  version: 1.0.0-SNAPSHOT

modules:
  - suffix: interfaces
    description: Shared DTOs and contracts
    packages:
      - dtos
      - enums
    dependencies:
      - { groupId: "org.fireflyframework", artifactId: "fireflyframework-utils" }
    templates:
      - { src: "core/interfaces-pom.xml.tmpl", dest: "pom.xml" }

rootTemplates:
  - { src: "shared/gitignore.tmpl", dest: ".gitignore" }
  - { src: "shared/README.md.tmpl", dest: "README.md" }
```

### Creating New Archetypes

1. Create a YAML file in `~/.flywork/archetypes/<name>.yaml`
2. Define modules, dependencies, and template references
3. Place corresponding `.tmpl` template files in the templates directory
4. Use Go template variables: `{{.ProjectName}}`, `{{.ArtifactId}}`, `{{.GroupId}}`, `{{.BasePackage}}`, `{{.PackagePath}}`, `{{.Version}}`, `{{.JavaVersion}}`, `{{.Year}}`, etc.
5. Run `flywork create <name>` to use your custom archetype

---

## Uninstall

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/fireflyframework/fireflyframework-cli/main/uninstall.sh | bash
```

### Windows

```powershell
irm https://raw.githubusercontent.com/fireflyframework/fireflyframework-cli/main/uninstall.ps1 | iex
```

---

## Development

### Prerequisites

- Go 1.25+
- Make

### Build Targets

```bash
make build       # build for current platform → bin/flywork
make install     # build + install to /usr/local/bin
make test        # run tests
make vet         # run go vet
make clean       # remove build artifacts
make build-all   # cross-compile for 6 platforms (darwin/linux/windows × amd64/arm64)
make release     # build-all + create .tar.gz / .zip archives
make checksums   # release + SHA256 checksums
```

### Project Structure

```
fireflyframework-cli/
├── cmd/                          # Cobra command definitions
│   ├── root.go                   # Root command, banner, global flags
│   ├── setup.go                  # flywork setup (DAG + TUI)
│   ├── create.go                 # flywork create (interactive scaffolding)
│   ├── doctor.go                 # flywork doctor (environment checks)
│   ├── update.go                 # flywork update (DAG + TUI)
│   ├── build.go                  # flywork build (smart DAG build)
│   ├── publish.go                # flywork publish (GitHub Packages deploy)
│   ├── dag.go                    # flywork dag (graph inspection)
│   ├── fwversion.go              # flywork fwversion (CalVer management)
│   ├── upgrade.go                # flywork upgrade (self-update)
│   ├── config.go                 # flywork config (get/set/reset)
│   └── version.go                # flywork version
├── internal/
│   ├── build/                    # Smart build engine
│   │   ├── builder.go            # DAG-ordered build execution
│   │   ├── changes.go            # SHA-based change detection
│   │   └── manifest.go           # Build manifest (last-known SHAs)
│   ├── config/config.go          # YAML config management
│   ├── dag/graph.go              # DAG engine (topological sort, layers, cycle detection)
│   ├── doctor/checks.go          # Diagnostic checks
│   ├── git/git.go                # Git operations
│   ├── java/java.go              # Cross-platform Java detection
│   ├── maven/maven.go            # Maven operations with JAVA_HOME support
│   ├── publish/                  # Publish engine
│   │   ├── publisher.go          # DAG-ordered Maven deploy
│   │   ├── python.go             # Python package publishing
│   │   └── settings.go           # Maven settings.xml management
│   ├── scaffold/                 # Archetype engine
│   │   ├── engine.go             # Template rendering and project generation
│   │   ├── archetypes/*.yaml     # Embedded archetype definitions
│   │   └── templates/*           # Embedded Go templates
│   ├── selfupdate/updater.go     # CLI self-update from GitHub releases
│   ├── setup/                    # Setup operations
│   │   ├── cloner.go             # DAG-ordered git clone
│   │   └── installer.go          # DAG-ordered maven install
│   ├── version/                  # Framework version management
│   │   ├── calver.go             # CalVer parsing and computation
│   │   ├── bumper.go             # POM version bumping across all repos
│   │   ├── checker.go            # Version consistency validation
│   │   └── families.go           # Version family tracking and history
│   └── ui/                       # TUI components
│       ├── printer.go            # Styled output, spinners, progress bars, summary boxes
│       └── prompt.go             # Interactive prompts
├── install.sh                    # curl | bash installer (macOS/Linux)
├── install.ps1                   # irm | iex installer (Windows)
├── uninstall.sh                  # Uninstaller (macOS/Linux)
├── uninstall.ps1                 # Uninstaller (Windows)
├── Makefile                      # Build targets for 6 platforms
├── go.mod / go.sum
├── LICENSE                       # Apache License 2.0
└── main.go                       # Entry point
```

---

## License

Copyright 2024-2026 Firefly Software Solutions Inc.

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
