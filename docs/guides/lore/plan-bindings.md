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

## System Bindings

The `system` object provides read-only queries about the current platform state. Use these to check conditions before scheduling operations.

### system.platform

Platform information about the current system.

| Property | Type | Description |
|----------|------|-------------|
| `system.platform.os` | string | Operating system: `"darwin"`, `"linux"`, `"windows"` |
| `system.platform.arch` | string | Architecture: `"amd64"`, `"arm64"` |
| `system.platform.distro` | string | Distribution codename (e.g., `"jammy"`, `"fedora"`, `"ventura"`) |

**Example:**
```starlark
def install(package, system, plan):
    if system.platform.os == "darwin":
        plan.package.install("cask:docker")
    elif system.platform.os == "linux":
        plan.package.install("docker-ce")

    if system.platform.arch == "arm64":
        # ARM-specific configuration
        pass
```

### system.package

Query the package manager for installed packages.

| Method | Returns | Description |
|--------|---------|-------------|
| `system.package.installed(name)` | bool | True if package is installed |
| `system.package.version(name)` | string | Installed version, or empty string if not installed |

**Example:**
```starlark
def install(package, system, plan):
    # Skip if already installed
    if system.package.installed("docker"):
        return

    # Check minimum version
    if system.package.installed("python3"):
        version = system.package.version("python3")
        print("Python version:", version)

    plan.package.install("docker")
```

### system.service

Query the service manager for service status.

| Method | Returns | Description |
|--------|---------|-------------|
| `system.service.exists(name)` | bool | True if service exists |
| `system.service.running(name)` | bool | True if service is currently running |
| `system.service.enabled(name)` | bool | True if service is enabled at boot |

**Example:**
```starlark
def verify(package, system, plan):
    # Check service is running after installation
    if not system.service.running("docker"):
        fail("Docker service is not running")

    if not system.service.enabled("docker"):
        fail("Docker service is not enabled at boot")
```

```starlark
def provision(package, system, plan):
    # Only start if not already running
    if not system.service.running("nginx"):
        plan.service(name="nginx", action="start")
```

---

## Package Context

The `package` object provides information about the package being deployed.

| Property/Method | Type | Description |
|-----------------|------|-------------|
| `package.name` | string | Package name being deployed |
| `package.version` | string | Version being deployed |
| `package.has_feature(name)` | bool | Check if a feature is enabled |
| `package.setting(key)` | string | Get a setting value (empty if not set) |
| `package.source_root` | string | Package source directory in registry cache |
| `package.target_root` | string | Deployment target directory (usually `$HOME`) |
| `package.dry_run` | bool | True if this is a preview (no actual changes) |

**Example:**
```starlark
def install(package, system, plan):
    # Feature-based conditional installation
    plan.package.install(package.name)

    if package.has_feature("completions"):
        plan.shell("{} completion bash > ~/.bash_completion.d/{}".format(
            package.name, package.name))

    if package.has_feature("plugins"):
        plan.file.copy(
            source=package.source_root + "/plugins/default.lua",
            target="~/.config/{}/plugins/default.lua".format(package.name)
        )

    # Use settings for customization
    theme = package.setting("theme")
    if theme:
        plan.file.write(
            "~/.config/{}/theme".format(package.name),
            theme
        )
```

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

| Prefix | Package Manager | Use Case |
|--------|-----------------|----------|
| `brew:` | Homebrew formula | CLI tools |
| `cask:` | Homebrew Cask | GUI applications |
| `port:` | MacPorts | CLI tools |

```starlark
def install(package, system, plan):
    plan.package.install("brew:wget")     # Force Homebrew formula
    plan.package.install("cask:iterm2")   # Homebrew Cask (GUI app)
    plan.package.install("port:tree")     # Force MacPorts
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

### macOS Idempotence

On macOS with both Homebrew and MacPorts installed, package operations are idempotent:

| Scenario | Behavior |
|----------|----------|
| Package installed via brew, upgrade requested | Upgrades via brew (not port) |
| Package installed via port, remove requested | Removes via port |
| Package installed via both | Operates on preferred PM; warns about the other |

**Example warning:**
```
[package] port remove wget
[warn] wget is also installed via brew; use 'brew:wget' to remove that copy
```

To explicitly manage a specific installation, use the prefix:

```starlark
def uninstall(package, system, plan):
    plan.package.remove("brew:wget")   # Remove only the brew version
    plan.package.remove("port:wget")   # Remove only the port version
```

**Note:** The `lore decommission` command removes from ALL package managers for complete cleanup.

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

### plan.file.write(target, content)

Writes content directly to a file. Use this for generating configuration files inline, writing apt sources, or creating small files without needing a template.

**Parameters:**
- `target` (string): Destination path (supports `~` expansion)
- `content` (string): Content to write to the file

**Returns:** A node object

**Example:**
```starlark
def prepare(package, system, plan):
    # Add Docker's apt repository (DEB822 format)
    plan.file.write(
        "/etc/apt/sources.list.d/docker.sources",
        """\
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: {}
Components: stable
Signed-By: /usr/share/keyrings/docker.asc
""".format(system.platform.distro)
    )
```

```starlark
def provision(package, system, plan):
    # Create a simple shell profile snippet
    plan.file.write(
        "~/.config/myapp/env.sh",
        """\
# Generated by lore - do not edit
export MYAPP_HOME="$HOME/.local/share/myapp"
export PATH="$MYAPP_HOME/bin:$PATH"
"""
    )
```

**Note:** Use multi-line strings with `"""\` (backslash after opening quotes) to avoid a leading newline. For complex templates with variable substitution, prefer `plan.file.configure()` instead.

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

## Node Objects

All plan operations return a **node object** that represents a unit of work in the execution graph. Use nodes with `plan.depends_on()` to control execution order.

### Node Properties

| Property | Type | Description |
|----------|------|-------------|
| `node.id` | string | Unique identifier for this node |
| `node.operations` | list | Operations to execute (e.g., `["link"]`, `["expand", "copy"]`) |
| `node.source` | string | Source path (for file operations) |
| `node.target` | string | Target path (for file operations) |
| `node.project` | string | Package/project name |
| `node.metadata` | dict | Additional operation-specific data |

### Example: Using Node Properties

```starlark
def install(package, system, plan):
    node = plan.file.copy(source="bin/mytool", target="~/.local/bin/mytool")
    print("Created node:", node.id)  # e.g., "copy-1"
    print("Target:", node.target)    # e.g., "/home/user/.local/bin/mytool"
```

---

## Platform-Specific Behavior

### macOS (Darwin)

- **Package manager:** Homebrew or MacPorts (MacPorts preferred if both installed)
- **Service manager:** launchd
- **Shell:** `/bin/sh`
- **Prefixes:** Use `brew:`, `cask:`, or `port:` to override auto-detection; `cask:` installs GUI apps via Homebrew Cask

**Service actions on macOS:**

| Action | launchd Equivalent |
|--------|-------------------|
| `start` | `launchctl load` / `launchctl bootstrap` |
| `stop` | `launchctl unload` / `launchctl bootout` |
| `restart` | Stop then start |
| `enable` | Load plist to appropriate domain |
| `disable` | Unload plist from domain |

**Service location paths:**
- User services: `~/Library/LaunchAgents/`
- System services: `/Library/LaunchDaemons/`

### Linux

- **Package manager:** apt (Debian/Ubuntu), dnf (Fedora/RHEL), pacman (Arch), zypper (openSUSE)
- **Service manager:** systemd
- **Shell:** `/bin/sh`

**Service actions on Linux:**

| Action | systemctl Equivalent |
|--------|---------------------|
| `start` | `systemctl start <service>` |
| `stop` | `systemctl stop <service>` |
| `restart` | `systemctl restart <service>` |
| `enable` | `systemctl enable <service>` |
| `disable` | `systemctl disable <service>` |

**Linux distribution detection:**

```starlark
def install(package, system, plan):
    distro = system.platform.distro  # e.g., "ubuntu", "fedora", "arch"

    if distro in ["debian", "ubuntu", "linuxmint"]:
        # Debian-family specific logic
        pass
    elif distro in ["fedora", "rhel", "centos"]:
        # Red Hat-family specific logic
        pass
```

### Windows

- **Package manager:** winget
- **Service manager:** Windows Services (sc.exe / PowerShell)
- **Shell:** PowerShell
- **Note:** Symlinks require administrator privileges

**Service actions on Windows:**

| Action | PowerShell Equivalent |
|--------|----------------------|
| `start` | `Start-Service` |
| `stop` | `Stop-Service` |
| `restart` | `Restart-Service` |
| `enable` | `Set-Service -StartupType Automatic` |
| `disable` | `Set-Service -StartupType Disabled` |

---

## Template Variables

When using `plan.file.configure()`, these variables are available in your template files:

| Variable | Description |
|----------|-------------|
| `{{ .Source }}` | Source file path |
| `{{ .Target }}` | Target file path |
| `{{ .Project }}` | Package/project name |

Additional context data may be passed through the execution context. Template files use Go's `text/template` syntax.

**Example template (`config.yaml.tmpl`):**
```yaml
# Configuration for {{ .Project }}
# Deployed to: {{ .Target }}
version: 1.0
data_dir: ~/.local/share/{{ .Project }}
```
