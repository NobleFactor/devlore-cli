---
title: "Create Manifests"
description: "Write and publish package lifecycle manifests"
tool: "lore"
category: "tutorial"
order: 4
---

# Create Manifests

A package manifest defines how to install, configure, and verify a piece of
software. This guide covers creating manifests from scratch, using AI assistance,
and publishing to the registry.

## Scaffold a manifest

Generate the directory structure for a new manifest:

```bash
lore manifest create mypackage
```

This creates:

```
staging/mypackage/
├── lifecycle.yaml      # Package metadata and phase declarations
├── prepare.star        # Prepare phase script
├── install.star        # Install phase script
├── provision.star      # Provision phase script
└── verify.star         # Verify phase script
```

### AI-assisted creation

Use AI to generate a manifest from upstream documentation:

```bash
lore manifest create postgresql --ai --from-url https://postgresql.org/download/
```

Or from an existing setup script:

```bash
lore manifest create pandoc --from ~/scripts/install-pandoc/
```

#### Configuring AI providers

By default, lore uses [Ollama](https://ollama.ai) for local inference. To use a cloud provider:

```bash
# GitHub Models (free with GitHub account)
DEVLORE_MODEL_PROVIDER=github DEVLORE_MODEL_API_KEY=$(gh auth token) \
  lore manifest create postgresql --ai

# Anthropic Claude
lore --model-provider=anthropic --model-api-key=sk-... \
  manifest create postgresql --ai

# Or configure in ~/.config/devlore/config.yaml
```

**Available providers:** `ollama` (default), `anthropic`, `openai`, `azure-openai`, `github`

See the [configuration reference](/cli/lore/) for all model options (`--model`, `--model-provider`, `--model-api-key`, `--model-endpoint`).

## The lifecycle file

`lifecycle.yaml` defines package metadata and declares which phases exist:

```yaml
name: mypackage
version: "1.0"
description: "Brief description of what this package provides"
homepage: https://example.com
platforms:
  - darwin
  - linux
features:
  - name: completions
    description: "Install shell completions"
  - name: plugins
    description: "Install recommended plugins"
phases:
  prepare: prepare.star
  install: install.star
  provision: provision.star
  verify: verify.star
```

## Writing phase scripts

Phase scripts are written in [Starlark](https://github.com/bazelbuild/starlark),
a Python-like language designed for configuration. Each phase script defines
a function named for the phase that receives two arguments:

```starlark
def install(package, phase):
    # package: information about the package being deployed (read-only)
    # phase:   phase context — retry policy, phase metadata
```

The `plan` object is a global — available in every script without being passed
as an argument. It provides all graph-building operations.

### Arguments

#### `package` — Package context (read-only)

| Property/Method | Description |
|-----------------|-------------|
| `package.name` | Package name being deployed |
| `package.version` | Version being deployed |
| `package.has_feature(name)` | Check if a feature is enabled |
| `package.setting(key)` | Get a setting value |
| `package.source_root` | Package source directory |
| `package.target_root` | Deployment target (usually `$HOME`) |

#### `phase` — Phase context

| Property/Method | Description |
|-----------------|-------------|
| `phase.name` | Current phase name (e.g., `"install"`) |
| `phase.retry(...)` | Set retry policy for this phase |

### `plan` — Graph operations (global)

`plan` is a global available in every lifecycle script. It builds the execution
graph declaratively — operations are scheduled, not executed immediately.

**Package operations** — `plan.package.*`

| Method | Description |
|--------|-------------|
| `plan.package.install(*pkgs)` | Install packages via native PM |
| `plan.package.upgrade(*pkgs)` | Upgrade packages via native PM |
| `plan.package.remove(*pkgs)` | Remove packages via native PM |
| `plan.package.update()` | Update package index |

**File operations** — `plan.file.*`

| Method | Description |
|--------|-------------|
| `plan.file.link(source, path)` | Create a symlink |
| `plan.file.copy(source, path)` | Copy a file |
| `plan.file.write(content, path)` | Write content directly to file |
| `plan.file.remove(path)` | Remove a file or directory |
| `plan.file.mkdir(path)` | Create directory (and parents) |

**Template and encryption** — `plan.template.*`, `plan.encryption.*`

| Method | Description |
|--------|-------------|
| `plan.template.render(source)` | Render a template |
| `plan.encryption.decrypt(source)` | Decrypt SOPS-encrypted content |

**Service operations** — `plan.service.*`

| Method | Description |
|--------|-------------|
| `plan.service.start(name)` | Start a service |
| `plan.service.stop(name)` | Stop a service |
| `plan.service.restart(name)` | Restart a service |
| `plan.service.enable(name)` | Enable a service at boot |
| `plan.service.disable(name)` | Disable a service at boot |

**Shell, network, and content** — `plan.shell.*`, `plan.net.*`, `plan.content.*`

| Method | Description |
|--------|-------------|
| `plan.shell.exec(command)` | Execute a shell command |
| `plan.net.download(url)` | Download a file |
| `plan.content.literal(content)` | Inline content |

**Archive and git** — `plan.archive.*`, `plan.git.*`

| Method | Description |
|--------|-------------|
| `plan.archive.extract(archive, prefix)` | Extract an archive |
| `plan.git.clone(repository, directory, **kwargs)` | Clone a repository; optional kwargs map 1:1 to `git clone` flags (`branch`, `depth`, `recurse_submodules`, `single_branch`, `bare`, `filter`, `origin`, `no_tags`, `no_checkout`, plus arbitrary pass-through) |
| `plan.git.checkout(repo, ref)` | Checkout a ref |
| `plan.git.pull(repo)` | Pull latest changes |

**Graph primitives**

| Method | Description |
|--------|-------------|
| `plan.source(path)` | Declare a source file |
| `plan.gather(items, limit, body)` | Parallel comprehension — body materializes once and dispatches per item, bounded by limit |
| `plan.choose(default, *cases)` | Conditional branch (evaluated at execution time); cases come from `plan.case(when, then)` |

**Output functions** (globals)

| Function | Description |
|----------|-------------|
| `note(msg)` | Print informational message |
| `warn(msg)` | Print warning message |
| `error(msg)` | Print error message |
| `success(msg)` | Print success message |
| `fail(msg)` | Abort with error |

### Cross-platform targeting

**Platform-specific logic belongs in the directory structure, not in script conditionals.** A package's platform
directories decide which scripts run on a machine:

```
Common/Deploy/install.star  → runs on every machine
Unix/Deploy/install.star    → runs on macOS, Linux and the BSDs
Linux/Deploy/install.star   → runs on every Linux machine
Debian/Deploy/install.star  → runs on every Debian-lineage machine: Debian, Ubuntu, Mint, Pop!_OS
Ubuntu/Deploy/install.star  → runs on Ubuntu, Mint and Pop!_OS
Darwin/Deploy/install.star  → runs on macOS only
```

The directory names are selectors, in the grammar writ's project directories use, without the project. `Common` is the
name with no selector words; every other name is an OS word, an architecture, or both, as in `Debian.arm64/`.
[Selectors](/guides/selectors/) is the one description of the grammar, its words and its order.

lore selects for the machine it runs on. The resolver reads the package's directories and takes `Common/` first, then
the directories that name this machine, from the most general link of its chain to the most specific. On Linux the
chain follows the lineage the distribution states in os-release, so on Ubuntu the order is `Common/`, `Unix/`,
`Linux/`, `Debian/`, `Ubuntu/`; on macOS it is `Common/`, `Unix/`, `Darwin/`. Every selected directory that holds the
phase's script contributes, general to specific. A directory named for another machine, such as `Fedora/` on Ubuntu,
is skipped. Planning on one machine for another is [#282](https://github.com/NobleFactor/devlore-cli/issues/282).

A directory name that breaks the grammar refuses the package before anything is planned, and the refusal lists every
such name. `Linux.Debian/` and `Linux.Fedora/`, the names lore read before
[#944](https://github.com/NobleFactor/devlore-cli/issues/944), carry two OS words and are refused: they are `Debian/`
and `Fedora/` now.

The lifecycle file's `platforms:` list and lore's `--platform` flags take no part in this selection. They are declared
but unread, which is [#969](https://github.com/NobleFactor/devlore-cli/issues/969).

**Do not use conditionals for platform branching.** Instead, put platform-specific
logic in the appropriate directory:

```starlark
# WRONG — platform conditional in a Common script
def install(package, phase):
    if somehow_check_os() == "darwin":
        plan.package.install("mypackage")
    else:
        plan.net.download("https://example.com/mypackage.tar.gz")

# RIGHT — separate scripts per platform
# Darwin/Deploy/install.star
def install(package, phase):
    plan.package.install("mypackage")

# Linux/Deploy/install.star
def install(package, phase):
    plan.net.download("https://example.com/mypackage.tar.gz")
    plan.archive.extract("/tmp/mypackage.tar.gz", "/usr/local/bin")
```

### Conditional logic at execution time

For conditions that depend on machine state (is a package installed? is a
service running?), use `plan.choose()` with predicates. These are evaluated
at execution time on the target machine, not at graph-building time:

```starlark
def install(package, phase):
    # Only install if not already present — checked on the target machine
    plan.choose(
        plan.shell.exec("true"),  # default: no-op
        plan.case(
            when=plan.package.not_installed("mypackage"),
            then=plan.package.install("mypackage"),
        ),
    )
```

### Example: complete manifest

```starlark
# Darwin/Deploy/install.star
def install(package, phase):
    plan.package.install("mypackage")
```

```starlark
# Linux/Deploy/install.star
def install(package, phase):
    plan.net.download("https://github.com/example/releases/download/v1.0/mypackage-linux-amd64")
    plan.shell.exec("sudo install /tmp/mypackage /usr/local/bin/mypackage")
```

```starlark
# Common/Deploy/provision.star
def provision(package, phase):
    if package.has_feature("completions"):
        plan.shell.exec("mypackage completion zsh > ~/.local/share/zsh/completions/_mypackage")
```

```starlark
# Common/Deploy/verify.star
def verify(package, phase):
    plan.shell.exec("mypackage --version")
```

## Validate a manifest

Check your manifest for errors before publishing:

```bash
lore manifest validate mypackage
```

Validates:

- Schema conformance (lifecycle.yaml structure)
- Phase files exist (referenced .star files are present)
- Starlark syntax (files parse without errors)
- Contract compliance (each phase script defines a function named for the phase)
- Feature consistency (features used in scripts match declarations)
- Platform coverage (conditionals cover declared platforms)

## Test a manifest

Dry-run on your current system:

```bash
lore manifest test mypackage
lore manifest test mypackage --with completions
lore manifest test mypackage --debug
```

Break at a specific phase:

```bash
lore manifest test mypackage --break install
```

## Update a manifest

Add features or platform support to an existing manifest:

```bash
lore manifest update mypackage --add-feature gpu-support
lore manifest update mypackage --add-platform windows
```

Import updates from upstream documentation:

```bash
lore manifest update docker --from-url https://docs.docker.com/engine/install/
```

## Publish to the registry

Submit a validated manifest for community review:

```bash
lore publish mypackage
```

This:

1. Runs final validation
2. Creates a pull request on the registry repository
3. Triggers automated testing on macOS, Linux, and Windows

The manifest is available to all lore users once the PR is merged.

## View manifest details

```bash
lore manifest show mypackage
```

Shows the full lifecycle definition, platform support, features, and
phase scripts.
