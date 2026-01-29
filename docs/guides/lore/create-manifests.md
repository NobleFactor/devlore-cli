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
a Python-like language designed for configuration. Each script must define
a `main(ctx)` function.

### The context object

The `ctx` argument provides:

| Method | Description |
|--------|-------------|
| `ctx.run(cmd)` | Execute a shell command |
| `ctx.pmm.install(pkgs)` | Install via native package manager |
| `ctx.pmm.remove(pkgs)` | Remove via native package manager |
| `ctx.feature(name)` | Check if a feature is enabled |
| `ctx.platform` | Current platform (`darwin`, `linux`) |
| `ctx.arch` | Architecture (`amd64`, `arm64`) |
| `ctx.user` | Current username |
| `ctx.home` | Home directory path |
| `ctx.codename` | OS codename (e.g., `jammy`) |
| `ctx.assert_success(r)` | Assert command succeeded |
| `ctx.assert_contains(s, sub)` | Assert string contains substring |

### Example: complete manifest

```python
# install.star
def main(ctx):
    if ctx.platform == "darwin":
        ctx.pmm.install(["mypackage"])
    elif ctx.platform == "linux":
        # Download binary for Linux
        url = "https://github.com/example/releases/download/v1.0/mypackage-{}-{}".format(
            ctx.platform, ctx.arch)
        ctx.run("curl -L {} -o /tmp/mypackage".format(url))
        ctx.run("sudo install /tmp/mypackage /usr/local/bin/mypackage")
```

```python
# provision.star
def main(ctx):
    if ctx.feature("completions"):
        ctx.run("mypackage completion zsh > ~/.local/share/zsh/completions/_mypackage")
```

```python
# verify.star
def main(ctx):
    result = ctx.run("mypackage --version")
    ctx.assert_success(result)
    ctx.assert_contains(result.stdout, "1.0")
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
