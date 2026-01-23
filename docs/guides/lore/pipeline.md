---
title: "The Pipeline"
description: "Understand lore's four-phase deployment model"
tool: "lore"
category: "concept"
order: 3
---

# The Four-Phase Pipeline

Every package deployment in lore follows a structured four-phase pipeline.
This model separates concerns, enables partial re-runs, and makes the
installation process auditable and reproducible.

## Pipeline overview

```
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│ Prepare  │───>│ Install  │───>│ Provision│───>│  Verify  │
│          │    │          │    │          │    │          │
│ Pre-reqs │    │ Package  │    │ Post-cfg │    │ Validate │
└──────────┘    └──────────┘    └──────────┘    └──────────┘
```

Each phase runs independently and can succeed or fail without affecting the
structure of subsequent phases. If a phase fails, the pipeline stops and
reports which phase failed and why.

## Phase 1: Prepare

**Purpose:** Set up prerequisites before installation.

Common prepare actions:

- Add package repository sources (APT repos, Homebrew taps)
- Import GPG signing keys
- Remove conflicting packages
- Create required users or groups
- Download and cache large files

```python
# prepare.star — Docker on Ubuntu
def main(ctx):
    # Add Docker's official GPG key
    ctx.run("curl -fsSL https://download.docker.com/linux/ubuntu/gpg | "
            "sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg")

    # Add repository
    ctx.run("echo 'deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] "
            "https://download.docker.com/linux/ubuntu {} stable' | "
            "sudo tee /etc/apt/sources.list.d/docker.list".format(ctx.codename))

    ctx.run("sudo apt-get update")
```

## Phase 2: Install

**Purpose:** Install the package using the appropriate method.

Installation methods (in order of preference):

1. **Native package manager** — `brew install`, `apt install`, `dnf install`
2. **Binary download** — Fetch pre-built binary from upstream
3. **Build from source** — Clone and compile
4. **Custom script** — Arbitrary installation logic

```python
# install.star — Docker on Ubuntu
def main(ctx):
    packages = [
        "docker-ce",
        "docker-ce-cli",
        "containerd.io",
        "docker-buildx-plugin",
    ]

    if ctx.feature("compose"):
        packages.append("docker-compose-plugin")

    ctx.pmm.install(packages)
```

## Phase 3: Provision

**Purpose:** Configure the installed software for use.

Common provision actions:

- Enable and start system services
- Create default configuration files
- Set up shell completions
- Add user to required groups
- Create data directories

```python
# provision.star — Docker on Ubuntu
def main(ctx):
    # Add user to docker group
    ctx.run("sudo usermod -aG docker {}".format(ctx.user))

    # Enable and start service
    ctx.run("sudo systemctl enable docker")
    ctx.run("sudo systemctl start docker")

    if ctx.feature("rootless"):
        ctx.run("dockerd-rootless-setuptool.sh install")
```

## Phase 4: Verify

**Purpose:** Confirm the installation succeeded.

Common verification checks:

- Binary exists and is executable
- Version matches expected
- Service is running
- Network connectivity works
- Configuration is valid

```python
# verify.star — Docker
def main(ctx):
    # Check binary exists
    result = ctx.run("docker --version")
    ctx.assert_contains(result.stdout, "Docker version")

    # Check service is running
    result = ctx.run("docker info")
    ctx.assert_success(result)

    # Check compose if enabled
    if ctx.feature("compose"):
        result = ctx.run("docker compose version")
        ctx.assert_success(result)
```

## Features in the pipeline

Features control which steps run within each phase. A package manifest
declares available features, and users enable them at deploy time:

```bash
lore deploy docker --with rootless --with compose
```

Inside phase scripts, check for enabled features:

```python
def main(ctx):
    # Always install core
    ctx.pmm.install(["docker-ce", "docker-ce-cli", "containerd.io"])

    # Conditional on feature
    if ctx.feature("compose"):
        ctx.pmm.install(["docker-compose-plugin"])

    if ctx.feature("buildx"):
        ctx.pmm.install(["docker-buildx-plugin"])
```

## Testing the pipeline

Dry-run a manifest to see what each phase would do:

```bash
lore manifest test docker
lore manifest test docker --with rootless --debug
```

Break at a specific phase for interactive debugging:

```bash
lore manifest test docker --break provision
```

## Audit trail

Every phase execution is logged to the security audit log:

```bash
lore audit --event phase --package docker
```

Shows timestamps, exit codes, and operations performed in each phase.
