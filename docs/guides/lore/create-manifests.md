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
a Python-like language designed for configuration. Each phase script must define
a function with the phase name that receives three arguments:

```starlark
def install(package, system, plan):
    # package: information about the package being deployed
    # system:  read-only queries about the current platform
    # plan:    graph-building operations to schedule work
```

### The three arguments

#### `package` — Package context

| Property/Method | Description |
|-----------------|-------------|
| `package.name` | Package name being deployed |
| `package.version` | Version being deployed |
| `package.has_feature(name)` | Check if a feature is enabled |
| `package.setting(key)` | Get a setting value |
| `package.source_root` | Package source directory |
| `package.target_root` | Deployment target (usually `$HOME`) |

#### `system` — Platform queries (read-only)

| Property/Method | Description |
|-----------------|-------------|
| `system.platform.os` | Operating system (`darwin`, `linux`, `windows`) |
| `system.platform.arch` | Architecture (`amd64`, `arm64`) |
| `system.platform.distro` | Distribution codename (e.g., `jammy`) |
| `system.package.installed(name)` | Check if package is installed |
| `system.package.version(name)` | Get installed package version |
| `system.service.running(name)` | Check if service is running |
| `system.service.enabled(name)` | Check if service is enabled at boot |

#### `plan` — Graph operations

| Method | Description |
|--------|-------------|
| `plan.package.install(*pkgs)` | Install packages via native PM |
| `plan.package.upgrade(*pkgs)` | Upgrade packages via native PM |
| `plan.package.remove(*pkgs)` | Remove packages via native PM |
| `plan.package.update()` | Update package index |
| `plan.file.copy(src, dst)` | Copy a file |
| `plan.file.link(src, dst)` | Create a symlink |
| `plan.file.configure(src, dst)` | Template expansion + copy |
| `plan.file.mkdir(path)` | Create a directory |
| `plan.file.write(path, content)` | Write content directly to file |
| `plan.service(name, action)` | Manage a service (start/stop/enable/disable) |
| `plan.shell(command)` | Execute a shell command |
| `plan.depends_on(node1, node2)` | node1 runs after node2 |

### Example: complete manifest

```starlark
# install.star
def install(package, system, plan):
    if system.platform.os == "darwin":
        plan.package.install("mypackage")
    elif system.platform.os == "linux":
        # Download binary for Linux
        url = "https://github.com/example/releases/download/v1.0/mypackage-{}-{}".format(
            system.platform.os, system.platform.arch)
        plan.shell("curl -L {} -o /tmp/mypackage".format(url))
        plan.shell("sudo install /tmp/mypackage /usr/local/bin/mypackage")
```

```starlark
# provision.star
def provision(package, system, plan):
    if package.has_feature("completions"):
        plan.shell("mypackage completion zsh > ~/.local/share/zsh/completions/_mypackage")
```

```starlark
# verify.star
def verify(package, system, plan):
    plan.shell("mypackage --version")

    if not system.service.running("mypackage"):
        fail("mypackage service is not running")
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
- Contract compliance (each phase has `main()` function)
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
