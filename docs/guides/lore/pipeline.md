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

```starlark
# prepare.star — Docker on Ubuntu
def prepare(package, system, plan):
    # Add Docker's official GPG key and repository
    plan.shell("curl -fsSL https://download.docker.com/linux/ubuntu/gpg | "
               "sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg")

    # Add repository (using system.platform for codename)
    plan.shell("echo 'deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] "
               "https://download.docker.com/linux/ubuntu {} stable' | "
               "sudo tee /etc/apt/sources.list.d/docker.list".format(system.platform.distro))

    plan.package.update()
```

## Phase 2: Install

**Purpose:** Install the package using the appropriate method.

Installation methods (in order of preference):

1. **Native package manager** — `brew install`, `apt install`, `dnf install`
2. **Binary download** — Fetch pre-built binary from upstream
3. **Build from source** — Clone and compile
4. **Custom script** — Arbitrary installation logic

```starlark
# install.star — Docker on Ubuntu
def install(package, system, plan):
    packages = [
        "docker-ce",
        "docker-ce-cli",
        "containerd.io",
        "docker-buildx-plugin",
    ]

    if package.has_feature("compose"):
        packages.append("docker-compose-plugin")

    plan.package.install(*packages)
```

## Phase 3: Provision

**Purpose:** Configure the installed software for use.

Common provision actions:

- Enable and start system services
- Create default configuration files
- Set up shell completions
- Add user to required groups
- Create data directories

```starlark
# provision.star — Docker on Ubuntu
def provision(package, system, plan):
    # Enable and start service
    plan.service("docker", "enable")
    plan.service("docker", "start")

    if package.has_feature("rootless"):
        plan.shell("dockerd-rootless-setuptool.sh install")
```

## Phase 4: Verify

**Purpose:** Confirm the installation succeeded.

Common verification checks:

- Binary exists and is executable
- Version matches expected
- Service is running
- Network connectivity works
- Configuration is valid

```starlark
# verify.star — Docker
def verify(package, system, plan):
    # Check binary exists
    plan.shell("docker --version")

    # Check service is running
    if not system.service.running("docker"):
        fail("Docker service is not running")

    # Check compose if enabled
    if package.has_feature("compose"):
        plan.shell("docker compose version")
```

## Features in the pipeline

Features control which steps run within each phase. A package manifest
declares available features, and users enable them at deploy time:

```bash
lore deploy docker --with rootless --with compose
```

Inside phase scripts, check for enabled features:

```starlark
def install(package, system, plan):
    # Always install core
    plan.package.install("docker-ce", "docker-ce-cli", "containerd.io")

    # Conditional on feature
    if package.has_feature("compose"):
        plan.package.install("docker-compose-plugin")

    if package.has_feature("buildx"):
        plan.package.install("docker-buildx-plugin")
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
