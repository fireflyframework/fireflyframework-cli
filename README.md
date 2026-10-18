# Firefly Framework CLI
    
[![CI](https://github.com/fireflyframework/fireflyframework-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/fireflyframework/fireflyframework-cli/actions/workflows/ci.yml)

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
\   __\\_  __ \__  \  /     \_/ __ \ \/ \/ /  _ \_  __ \  |/ /
 |  |   |  | \// __ \|  Y Y  \  ___/\     (  <_> )  | \/    <
 |__|   |__|  (____  /__|_|  /\___  >\/\_/ \____/|__|  |__|_ \
                   \/      \/     \/                        \/
```

Use `flywork` to install Jafly, create Java services, check your development environment, and manage the framework release train.

---

## Install

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/fireflyframework/fireflyframework-cli/main/install.sh | bash
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/fireflyframework/fireflyframework-cli/main/install.ps1 | iex
```

### From source

```bash
git clone https://github.com/fireflyframework/fireflyframework-cli.git
cd fireflyframework-cli
make install
```

---

For an isolated Unix installation, set `FLYWORK_VERSION`, `FLYWORK_INSTALL_DIR` and `FLYWORK_NO_MODIFY_PATH=1` when running the installer. The last option prevents persistent shell-profile edits; PowerShell supports the same opt-out for persistent user PATH changes. The installer still verifies the binary in the chosen directory.

## Commands

### `flywork setup`

Clones the [Jafly monorepo](https://github.com/fireflyframework/fireflyframework-jafly) into `repos_path/fireflyframework-jafly` and installs its complete Maven reactor to your local cache (`~/.m2`). Maven resolves dependencies between modules inside that checkout. Existing legacy Java checkouts remain in place.

Setup keeps its own `setup-manifest-jafly.json`, preserving old setup state. After successful installation, it saves the installed parent version for subsequent scaffolds. Java 25 and Maven 3.9+ are required; version/release tools also require Python 3.

```bash
flywork setup # full interactive setup (prompts whether to run tests)
flywork setup --skip-tests # skip tests during Maven install
flywork setup --retry # retry only previously failed repositories
flywork setup --fresh # ignore previous manifest, start from scratch
flywork setup --fetch-updates # also fetch updates for already-cloned repos
flywork setup --jdk /path # use a specific JDK instead of auto-detection
flywork setup -v # verbose: show DAG layers and per-repo status
```

When `--skip-tests` is not provided, the CLI interactively asks whether to run tests (default: **yes**).

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--skip-tests` | `false` | Skip running tests during Maven install |
| `--retry` | `false` | Retry only previously failed repositories |
| `--fresh` | `false` | Force a fresh setup, ignoring any previous manifest |
| `--fetch-updates` | `false` | Fetch latest changes for already-cloned repos |
| `--jdk` | `""` | Explicit JAVA_HOME path (skip JDK auto-detection) |

Setup proceeds as follows:

1. Verify that Git, Maven, and Java are installed.
2. Read the previous setup manifest and offer to resume, retry, or restart.
3. Find an installed JDK matching the configured `java_version`.
4. Clone Jafly on the configured branch.
5. Run `mvn clean install` once at the reactor root.
6. Offer to retry a failed installation.
7. Displays a summary box with total time, repos cloned/installed, and layer count

### `flywork create [archetype]`

Scaffolds a new project from one of four YAML-driven archetypes:

| Archetype | Description |
|-----------------|--------------------------------------------------------------------|
| **core** | Multi-module microservice with R2DBC, Flyway, MapStruct, OpenAPI SDK |
| **domain** | Multi-module CQRS/Saga microservice with orchestration engine |
| **application** | Single-module application with plugin architecture |
| **library** | Single-module library with Spring Boot auto-configuration |

```bash
flywork create # fully interactive mode
flywork create core # core archetype with prompts
flywork create domain --group-id com.example --artifact-id my-service
flywork create application -g com.example -a my-app -d "My Application"
flywork create library -g com.example -a my-lib --no-git
flywork create core --version 1.0.0 -o ./output-dir # custom version and output
```

If no archetype or flags are provided, the CLI enters interactive mode with prompts for archetype selection, group ID, artifact ID, package name, description, and infrastructure defaults.

**Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--group-id` | `-g` | `""` | Maven groupId (interactive if omitted) |
| `--artifact-id` | `-a` | `""` | Maven artifactId (interactive if omitted) |
| `--package` | `-p` | `""` | Base Java package (derived from groupId if omitted) |
| `--description` | `-d` | `""` | Project description |
| `--version` | | `0.0.1-SNAPSHOT` | Initial project version |
| `--output` | `-o` | `""` | Output directory (defaults to artifactId) |
| `--no-git` | | `false` | Skip git init |

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

Pulls the Jafly checkout and reinstalls its reactor to `.m2`. Successful installation aligns the configured parent version with the checkout.

```bash
flywork update # pull + install all (prompts for tests)
flywork update --skip-tests # pull + install without tests
flywork update --pull-only # only git pull, skip maven
flywork update --repo fireflyframework-jafly # Java reactor
flywork update -v # verbose with layer info
```

When `--skip-tests` is not provided (and not `--pull-only`), the CLI interactively asks whether to run tests (default: **yes**).

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--pull-only` | `false` | Only git pull, skip Maven install |
| `--repo` | `""` | Update a single repository by name |
| `--skip-tests` | `false` | Skip running tests during Maven install |

The update command uses the same repository graph as `setup`:

1. Pull the checkout's configured branch. A failed pull stops the update.
2. Install the Maven reactor and report elapsed time.

### `flywork build`

Builds the Jafly reactor when its checkout SHA differs from the last successful build. Git operations use the repository root; Maven owns the dependency graph between nested modules.

```bash
flywork build # build changed repos + affected dependents
flywork build --all # rebuild everything (ignore change detection)
flywork build --repo <name> # build a specific repo and its dependents
flywork build --dry-run # show what would be built without building
flywork build --skip-tests # skip running tests during Maven install
flywork build --jdk /path # use an explicit JAVA_HOME
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | `false` | Rebuild everything (ignore change detection) |
| `--repo` | `""` | Build a specific repo and its dependents |
| `--dry-run` | `false` | Show what would be built without building |
| `--skip-tests` | `false` | Skip running tests during Maven install |
| `--jdk` | `""` | Explicit JAVA_HOME path |

Build proceeds as follows:

1. Verify that Git, Maven, and Java are installed.
2. Compare the checkout SHA with the last successful build in `~/.flywork/build-manifest.json`.
3. Show the repository selected for the build.
4. Run `mvn clean install` at the Jafly root.
5. Report the result, elapsed time, and log locations for failures.

### `flywork publish`

Deploys the Jafly reactor's publishable artifacts to GitHub Packages. The command uses the same checkout-level change detection as `build`; Maven orders the modules.

```bash
flywork publish # publish changed repos
flywork publish --all # publish everything
flywork publish --repo <name> # publish a specific repo
flywork publish --dry-run # show what would be published
flywork publish --skip-tests # skip tests during deploy (default: true)
flywork publish --jdk /path # use an explicit JAVA_HOME
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | `false` | Publish everything (ignore change detection) |
| `--repo` | `""` | Publish a specific repo only |
| `--dry-run` | `false` | Show what would be published without publishing |
| `--skip-tests` | `true` | Skip tests during deploy |
| `--jdk` | `""` | Explicit JAVA_HOME path |

Requires `GITHUB_TOKEN` environment variable set with `write:packages` scope.

Publication proceeds as follows:

1. Verify `GITHUB_TOKEN`, Git, Maven, and Java.
2. Configure the GitHub Packages server in `~/.m2/settings.xml`.
3. Show the checkout selected for publication.
4. Run `mvn deploy` at the reactor root.
5. Report the deployment result and elapsed time.

### `flywork dag`

Inspect the repository graph, which contains one Jafly node. Maven resolves module dependencies. `dag affected` lists downstream repositories and excludes its source, so Jafly has no downstream repository nodes; CI should dispatch the reactor directly.

```bash
flywork dag show # display full graph as ASCII tree
flywork dag layers # show repos grouped by build layer
flywork dag affected --from <repo> # compute transitive closure of affected repos
flywork dag affected --from <repo> --json # machine-readable output
flywork dag export # export full DAG as JSON for CI/CD consumption
```

**Subcommands:**

| Subcommand | Description |
|------------|-------------|
| `show` | Full dependency graph with arrows showing dependencies |
| `layers` | Repos grouped by build layer (0 = no dependencies) |
| `affected` | Transitive closure of repos affected by a change in `--from` (required flag) |
| `export` | JSON export of the entire DAG (nodes, edges, layers) for CI/CD |

### `flywork fwversion`

Manage the Jafly release train across nested POMs using the repository's `scripts/set_version.py`. Versions use padded `YY.MM.PP` (for example, `26.10.17`), where `PP` counts releases within the month. Valid months are 01 through 12. Python 3 must be on PATH.

```bash
flywork fwversion show # show current version across all repos
flywork fwversion bump --auto # auto-compute next CalVer and bump all POMs
flywork fwversion bump --auto --push # bump, commit, tag, and push
flywork fwversion bump --dry-run # preview changes without modifying files
flywork fwversion bump --install # bump + run mvn install after
flywork fwversion check # validate version consistency across repos
flywork fwversion families # show version family release history
```

**Subcommands:**

| Subcommand | Description |
|------------|-------------|
| `show` | Displays current POM versions, mismatches, dirty trees, and config alignment |
| `bump` | Updates framework versions and references, preserves application and third-party versions, and optionally commits, tags, and pushes |
| `check` | Runs consistency checks: POM versions, config match, git tags, clean trees, `.m2` artifacts |
| `families` | Shows version family history; each bump records the repository SHA |

**Bump flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--auto` | `false` | Auto-compute next CalVer from current version |
| `--commit` | `true` | Git commit the version changes |
| `--tag` | `true` | Git tag each repo with `v<version>` |
| `--push` | `false` | Git push after commit/tag |
| `--dry-run` | `false` | Show changes without modifying files |
| `--install` | `false` | Run `mvn install` in all repos after bumping |

### `flywork release`

Prepares one Jafly release train with the repository version tool, commits the POM changes, opens a release PR, waits for the merge, and tags the exact final commit. Existing tags must resolve to that same commit. Completion requires success of `release.yml` for the exact tag and SHA; the workflow verifies both GitHub Packages and Maven Central before creating the GitHub Release. Branch protection remains active unless `--admin` is explicitly supplied.

```bash
flywork release --year 26 --month 10 --patch 17
flywork release --dry-run
flywork release --skip-tag # prepare the PR without tagging or publication
flywork release verify --version 26.10.17
flywork release publish-mvn-central --version 26.10.17
```

`--dry-run` creates no branches, commits, tags, PRs, or workflow runs. `--from-layer` accepts only `0`; `--repo`, when supplied, must be `fireflyframework-jafly`. `--skip-prs` explicitly requests a direct, non-forced push to main. `--timeout-minutes` defaults to 180 for the PR merge and, separately, the publication workflow. `--admin` defaults to false.

`release verify` resolves the remote tag's commit and checks it against `release-commit.txt`. It clones that tag into a temporary directory, compares the published reactor inventory with the tagged source, and uses that source's `scripts/release_tools.py` to verify every remote file's SHA256. It requires Git, Python 3, an authenticated `gh`, and `GITHUB_ACTOR`/`GITHUB_TOKEN` for GitHub Packages. It fails on a commit mismatch, truncated inventory, absent manifests, missing files, wrong versions, or digest mismatches. Your working checkout stays unchanged.

The compatibility command `publish-mvn-central` dispatches the combined `release.yml` at the existing `v<version>` tag. It reports dispatch only; the workflow owns immutable publication checks and can fail safely if a partially published version cannot be retried. The old `--wait-seconds` flag is retained for compatibility; the destination workflow controls its own timeout.

### `flywork run`

Runs a Firefly Framework application with interactive configuration assistance. Detects the Spring Boot module, scans configuration files for missing environment variables, and launches an interactive wizard before starting the app.

```bash
flywork run # interactive run with configuration wizard
flywork run --profile dev # activate the 'dev' Spring profile
flywork run --skip-wizard # skip the configuration wizard
flywork run --profile local --skip-wizard # non-interactive run with 'local' profile
```

The runner performs these steps:

1. Detect the archetype and project layout, then locate the Spring Boot module.
2. Offer a profile selection when profiles are available and `--profile` is absent.
3. Read `${PLACEHOLDER}` variables from `application.yaml`, identifying values supplied by the environment, defaults, and missing required values.
4. Ask for missing values and offer default overrides. Common names have suggested values such as `DB_HOST=localhost` and `DB_PORT=5432`.
5. Start `mvn spring-boot:run` with the selected profile and environment values.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--profile` | `""` | Spring profile to activate (e.g. dev, local) |
| `--skip-wizard` | `false` | Skip the interactive configuration wizard |

### `flywork upgrade`

Self-updates the CLI binary from GitHub releases.

```bash
flywork upgrade # download and install latest version
flywork upgrade --check # just check if an update is available
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--check` | `false` | Only check for updates, don't install |

### `flywork config`

View and manage CLI configuration stored in `~/.flywork/config.yaml`.

```bash
flywork config # show all configuration
flywork config get java_version # get a single value
flywork config set java_version 25 # set a value
flywork config set parent_version 1.0.0-SNAPSHOT
flywork config reset # reset to defaults
```

### `flywork version`

Prints CLI version, git commit, build date, Go version, and OS/architecture.

```bash
flywork version
```

---

## Global flags

| Flag | Short | Description |
|------|-------|-------------|
| `--verbose` | `-v` | Enable verbose output (DAG layers, per-repo details, etc.) |
| `--help` | `-h` | Show help for any command |

The `--verbose` flag is available on all commands. It enables additional output such as DAG layer headers, per-repo status lines, and detailed version information.

---

## Repository and module ordering

The CLI repository graph has one node, `fireflyframework-jafly`. `dag export --json` exports one layer. Nested directories such as `build/parent`, `build/bom`, and `modules/kernel` are Maven modules, not independent Git repositories. Module ordering and cycles are checked by Maven's reactor.

---

## Configuration

Configuration is stored in `~/.flywork/config.yaml`.

Set `FLYWORK_HOME` to a separate directory to isolate configuration, setup state,
logs, and the default checkout location. For example:

```bash
FLYWORK_HOME="$PWD/.flywork-jafly" flywork setup --jdk /path/to/jdk-25
```

This does not change Maven's cache location. Use Maven's `-Dmaven.repo.local`
setting separately when an isolated artifact cache is required.

| Key | Default | Description |
|-----|---------|-------------|
| `repos_path` | `~/.flywork/repos` | Where framework repos are cloned |
| `github_org` | `fireflyframework` | GitHub organization name |
| `default_group_id` | `org.fireflyframework` | Default Maven groupId for new projects |
| `java_version` | `25` | Target Java version for compilation |
| `parent_version` | `26.10.17-SNAPSHOT` | Initial Jafly version; successful setup/update saves the installed version |
| `cli_auto_update` | `false` | Auto-check for CLI updates on launch |
| `branch` | `develop` | Git branch to clone during setup |

### Select the Java version

The CLI automatically detects installed Java versions:

- macOS uses `/usr/libexec/java_home -v <version>`.
- Linux scans `/usr/lib/jvm/` and `update-alternatives`.
- Windows scans locations including `Program Files\Java`, `Eclipse Adoptium`, and `Corretto`.

Change the required Java version:

```bash
flywork config set java_version 25
```

All `setup` and `update` commands will automatically resolve the correct `JAVA_HOME` for that version.

---

## Project archetypes

### Built-in archetypes

Archetypes are YAML-driven definitions embedded in the CLI binary. Each archetype defines:

- Single-module or multi-module layout
- Parent POM inherited from `fireflyframework-parent`
- Maven dependencies for each module
- Maven plugins, including Spring Boot, MapStruct, and OpenAPI Generator
- Go templates rendered with project metadata
- Java package directories

### Customize an archetype

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

### Create an archetype

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
- Python 3 (installer regression tests and Jafly version/release tools)
- Make

### Build targets

```bash
make build # build for current platform → bin/flywork
make install # build + install to /usr/local/bin
make test # run tests
make vet # run go vet
make clean # remove build artifacts
make build-all # cross-compile for 6 platforms (darwin/linux/windows × amd64/arm64)
make release # build-all + create .tar.gz / .zip archives
make checksums # release + SHA256 checksums
```

### Project structure

```
fireflyframework-cli/
├── cmd/ # Cobra command definitions
│ ├── root.go # Root command, banner, global flags
│ ├── setup.go # flywork setup (DAG + TUI)
│ ├── create.go # flywork create (interactive scaffolding)
│ ├── doctor.go # flywork doctor (environment checks)
│ ├── update.go # flywork update (DAG + TUI)
│ ├── build.go # flywork build (smart DAG build)
│ ├── publish.go # flywork publish (GitHub Packages deploy)
│ ├── dag.go # flywork dag (graph inspection)
│ ├── fwversion.go # flywork fwversion (CalVer management)
│ ├── release.go # flywork release (DAG-ordered cascade + verify + publish-mvn-central)
│ ├── upgrade.go # flywork upgrade (self-update)
│ ├── config.go # flywork config (get/set/reset)
│ ├── run.go # flywork run (application runner)
│ └── version.go # flywork version
├── internal/
│ ├── build/ # Smart build engine
│ │ ├── builder.go # DAG-ordered build execution
│ │ ├── changes.go # SHA-based change detection
│ │ └── manifest.go # Build manifest (last-known SHAs)
│ ├── config/config.go # YAML config management
│ ├── dag/graph.go # DAG engine (topological sort, layers, cycle detection)
│ ├── doctor/checks.go # Diagnostic checks
│ ├── git/git.go # Git operations
│ ├── java/java.go # Cross-platform Java detection
│ ├── maven/maven.go # Maven operations with JAVA_HOME support
│ ├── publish/ # Publish engine
│ │ ├── publisher.go # DAG-ordered Maven deploy
│ │ ├── python.go # Python package publishing
│ │ └── settings.go # Maven settings.xml management
│ ├── runner/ # Application runner with config wizard
│ ├── scaffold/ # Archetype engine
│ │ ├── engine.go # Template rendering and project generation
│ │ ├── archetypes/*.yaml # Embedded archetype definitions
│ │ └── templates/* # Embedded Go templates
│ ├── selfupdate/updater.go # CLI self-update from GitHub releases
│ ├── setup/ # Setup operations
│ │ ├── cloner.go # DAG-ordered git clone
│ │ └── installer.go # DAG-ordered maven install
│ ├── version/ # Framework version management
│ │ ├── calver.go # CalVer parsing and computation
│ │ ├── bumper.go # POM version bumping across all repos
│ │ ├── checker.go # Version consistency validation
│ │ └── families.go # Version family tracking and history
│ └── ui/ # TUI components
│ ├── printer.go # Styled output, spinners, progress bars, summary boxes
│ └── prompt.go # Interactive prompts
├── install.sh # curl | bash installer (macOS/Linux)
├── install.ps1 # irm | iex installer (Windows)
├── uninstall.sh # Uninstaller (macOS/Linux)
├── uninstall.ps1 # Uninstaller (Windows)
├── Makefile # Build targets for 6 platforms
├── go.mod / go.sum
├── LICENSE # Apache License 2.0
└── main.go # Entry point
```

---

## License

Copyright 2024-2026 Firefly Software Foundation.

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
