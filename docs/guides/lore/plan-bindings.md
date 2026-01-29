---
title: "Plan Bindings Reference"
description: "Reference documentation for plan bindings in lore phase scripts"
tool: "lore"
category: "reference"
order: 6
---

# Plan Bindings Reference

This document describes the `plan` object available in lore phase scripts. Use these functions to declare what actions should be taken during package deployment.

## Overview

Phase scripts receive three arguments:

```starlark
def install(package, system, plan):
    # package - context about the package being deployed
    # system  - read-only queries about the current system
    # plan    - operations to add to the execution graph
```

The `plan` object provides namespaced functions for different operation types:

| Namespace | Purpose |
|-----------|---------|
| `plan.package.*` | Package manager operations |
| `plan.file.*` | File system operations |
| `plan.service()` | Service management |
| `plan.shell()` | Shell command execution |
| `plan.depends_on()` | Dependency ordering |

---

## Package Operations

The `plan.package` namespace provides cross-platform package management. Operations use the system's native package manager:

| Platform | Package Manager |
|----------|-----------------|
| macOS | Homebrew (brew) or MacPorts (port) |
| Linux (Debian/Ubuntu) | apt |
| Linux (Fedora/RHEL) | dnf |
| Windows | winget |

### plan.package.install(*packages)

Installs one or more packages using the system's package manager.

**Parameters:**
- `*packages` (strings): One or more package names

**Returns:** A node object for use with `plan.depends_on()`

**Example:**
```starlark
def install(package, system, plan):
    plan.package.install("curl", "jq", "ripgrep")
```

**macOS Package Manager Selection:**

On macOS, the package manager is auto-detected (MacPorts preferred over Homebrew if both are installed). You can force a specific manager with prefixes:

```starlark
def install(package, system, plan):
    plan.package.install("brew:wget")   # Force Homebrew
    plan.package.install("port:tree")   # Force MacPorts
```

### plan.package.upgrade(*packages)

Upgrades one or more packages to their latest version.

**Parameters:**
- `*packages` (strings): One or more package names

**Returns:** A node object

**Example:**
```starlark
def install(package, system, plan):
    plan.package.upgrade("curl", "openssl")
```

### plan.package.remove(*packages)

Removes one or more packages.

**Parameters:**
- `*packages` (strings): One or more package names

**Returns:** A node object

**Example:**
```starlark
def uninstall(package, system, plan):
    plan.package.remove("telnet", "ftp")
```

### plan.package.update()

Updates the package manager's index/cache. Run this before install/upgrade if you need the latest package versions.

**Parameters:** None

**Returns:** A node object

**Example:**
```starlark
def install(package, system, plan):
    update = plan.package.update()
    install = plan.package.install("nginx")
    plan.depends_on(install, update)  # Install after update completes
```

---

## File Operations

The `plan.file` namespace provides file system operations.

### plan.file.configure(source, target)

Processes a template file and copies the result to the target location. Templates use Go's `text/template` syntax with access to package settings.

**Parameters:**
- `source` (string): Path to template file (relative to package root)
- `target` (string): Destination path (supports `~` expansion)

**Returns:** A node object

**Example:**
```starlark
def configure(package, system, plan):
    plan.file.configure(
        source="configs/app.conf.tmpl",
        target="~/.config/myapp/config"
    )
```

**Template variables available:**
- `{{ .Name }}` - Package name
- `{{ .Version }}` - Package version
- `{{ .Settings.<key> }}` - User-defined settings
- `{{ .HomeDir }}` - User's home directory

### plan.file.copy(source, target)

Copies a file without template processing.

**Parameters:**
- `source` (string): Source file path (relative to package root)
- `target` (string): Destination path (supports `~` expansion)

**Returns:** A node object

**Example:**
```starlark
def install(package, system, plan):
    plan.file.copy(
        source="bin/mytool",
        target="~/.local/bin/mytool"
    )
```

### plan.file.link(source, target)

Creates a symbolic link.

**Parameters:**
- `source` (string): Link target (what the symlink points to)
- `target` (string): Symlink location (supports `~` expansion)

**Returns:** A node object

**Example:**
```starlark
def install(package, system, plan):
    plan.file.link(
        source="~/.config/nvim",
        target="~/.vim"
    )
```

### plan.file.mkdir(target)

Creates a directory (and parent directories if needed).

**Parameters:**
- `target` (string): Directory path (supports `~` expansion)

**Returns:** A node object

**Example:**
```starlark
def install(package, system, plan):
    plan.file.mkdir("~/.config/myapp")
    plan.file.mkdir("~/.local/share/myapp/data")
```

---

## Service Operations

### plan.service(name, action)

Manages system services (launchd on macOS, systemd on Linux, Windows Services on Windows).

**Parameters:**
- `name` (string): Service name
- `action` (string): One of: `"start"`, `"stop"`, `"restart"`, `"enable"`, `"disable"`

**Returns:** A node object

**Example:**
```starlark
def configure(package, system, plan):
    plan.service(name="nginx", action="restart")
```

```starlark
def install(package, system, plan):
    # Enable service to start at boot
    plan.service(name="docker", action="enable")
    plan.service(name="docker", action="start")
```

---

## Shell Operations

### plan.shell(command)

Executes a shell command. Use sparingly - prefer declarative operations when possible.

**Parameters:**
- `command` (string): Shell command to execute

**Returns:** A node object

**Example:**
```starlark
def install(package, system, plan):
    # Only use shell when no declarative alternative exists
    plan.shell("fc-cache -f -v")  # Refresh font cache
```

**Platform notes:**
- macOS/Linux: Runs via `/bin/sh -c`
- Windows: Runs via PowerShell

---

## Dependency Ordering

### plan.depends_on(from_node, to_node)

Creates a dependency between nodes. The `from_node` will execute only after `to_node` completes successfully.

**Parameters:**
- `from_node`: Node that depends on another
- `to_node`: Node that must complete first

**Returns:** None

**Example:**
```starlark
def install(package, system, plan):
    # Create directory before copying files into it
    dir_node = plan.file.mkdir("~/.config/myapp")
    config_node = plan.file.configure(
        source="config.tmpl",
        target="~/.config/myapp/config"
    )
    plan.depends_on(config_node, dir_node)
```

```starlark
def install(package, system, plan):
    # Update package index before installing
    update = plan.package.update()
    install = plan.package.install("nginx", "certbot")
    plan.depends_on(install, update)

    # Configure after install
    config = plan.file.configure(
        source="nginx.conf.tmpl",
        target="/etc/nginx/nginx.conf"
    )
    plan.depends_on(config, install)

    # Restart service after config
    restart = plan.service(name="nginx", action="restart")
    plan.depends_on(restart, config)
```

---

## Complete Example

```starlark
def install(package, system, plan):
    """Install the application and its dependencies."""

    # Skip if already installed
    if system.package.installed(package.name):
        return

    # Update package index first
    update = plan.package.update()

    # Install system dependencies
    deps = plan.package.install("curl", "jq")
    plan.depends_on(deps, update)

    # Create config directory
    config_dir = plan.file.mkdir("~/.config/myapp")

    # Deploy configuration (template processing)
    config = plan.file.configure(
        source="configs/settings.yaml.tmpl",
        target="~/.config/myapp/settings.yaml"
    )
    plan.depends_on(config, config_dir)


def configure(package, system, plan):
    """Update configuration files."""

    plan.file.configure(
        source="configs/settings.yaml.tmpl",
        target="~/.config/myapp/settings.yaml"
    )


def uninstall(package, system, plan):
    """Remove the application."""

    plan.package.remove(package.name)
    plan.shell("rm -rf ~/.config/myapp")
```

---

## Platform-Specific Behavior

### macOS (Darwin)

- **Package manager:** Homebrew or MacPorts (MacPorts preferred if both installed)
- **Service manager:** launchd
- **Shell:** `/bin/sh`
- **Cask support:** Use `plan.package.install_cask()` for GUI applications (Homebrew only)

### Linux

- **Package manager:** apt (Debian/Ubuntu), dnf (Fedora/RHEL)
- **Service manager:** systemd
- **Shell:** `/bin/sh`

### Windows

- **Package manager:** winget
- **Service manager:** Windows Services
- **Shell:** PowerShell
- **Note:** Symlinks require administrator privileges
