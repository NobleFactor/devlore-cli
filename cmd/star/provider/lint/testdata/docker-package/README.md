# Docker — lore Package

Docker container runtime across all platforms:
- **Linux:** Docker CE (Community Edition) — daemon, CLI, containerd, buildx, and compose plugin
- **macOS/Windows:** Docker Desktop — proprietary graphical application with integrated VM

## Usage

```bash
# Deploy Docker
lore deploy docker

# Deploy with rootless mode (Linux only)
lore deploy docker --with rootless

# Upgrade to latest version
lore upgrade docker

# Remove Docker (preserves data)
lore decommission docker

# Remove Docker and all data (images, containers, volumes)
lore decommission docker --with purge-data
```

## Platform Notes

### Linux.Debian (Ubuntu, Debian)

Uses official Docker CE apt repository. Key considerations:
- Removes conflicting packages (`docker.io`, `podman-docker`, etc.)
- Requires adding user to `docker` group for non-root access
- ODROID-C4/C5 require cgroup v1 boot argument

### Linux.Fedora (Fedora, RHEL, CentOS, Rocky, AlmaLinux)

Uses official Docker CE dnf repository. Same package set as Debian, different package manager and conflict list (`podman`, `buildah`, etc.).

### Darwin (macOS)

Docker Desktop installed via DMG. Key considerations:
- Apple Silicon (arm64) and Intel (amd64) use different installers
- On Apple Silicon, installs Rosetta 2 for x86 container compatibility
- Data stored in `~/Library/Containers/` and `~/Library/Group Containers/`

### Windows

Docker Desktop installed via exe installer. Key considerations:
- Requires WSL 2 (Windows Subsystem for Linux) — enabled automatically
- User added to `docker-users` group for non-admin access
- Data stored in WSL 2 distros (`docker-desktop`, `docker-desktop-data`)

## Tribal Knowledge

### 1. Conflicting Packages (Linux)

Distribution-provided Docker packages conflict with Docker CE:

| Distribution | Conflicts |
|-------------|-----------|
| Debian/Ubuntu | `docker.io`, `docker-compose`, `podman-docker`, `containerd`, `runc` |
| Fedora/RHEL | `podman`, `podman-docker`, `buildah`, `containers-common` |

The prepare phase removes these automatically.

### 2. ODROID-C4/C5 Require cgroup v1 Mode (Linux.Debian)

Docker containers fail to start on ODROID-C4/C5 due to cgroup v2 incompatibility. Add boot argument:
```
systemd.unified_cgroup_hierarchy=0
```
to `/media/boot/boot.ini`. The verify phase detects ODROID hardware and warns if needed.

**Reference:** https://sipfront.com/blog/2024/01/running-docker-on-odroid-c4/

### 3. Rosetta 2 for x86 Containers (Darwin)

Apple Silicon Macs need Rosetta 2 to run x86_64 Linux containers. The prepare phase installs it automatically.

### 4. WSL 2 Backend (Windows)

Docker Desktop on Windows uses WSL 2 by default. The prepare phase enables the required Windows features and installs the WSL 2 kernel update.

## Features

| Feature | Default | Description |
|---------|---------|-------------|
| `rootless` | false | Run Docker daemon without root privileges (Linux only) |
| `purge-data` | false | Remove all data (images, containers, volumes) during decommission |

## Settings

| Setting | Default | Values | Description |
|---------|---------|--------|-------------|
| `storage-driver` | overlay2 | overlay2, btrfs, zfs, vfs | Docker storage driver (Linux) |
| `log-driver` | json-file | json-file, syslog, journald, none | Default logging driver |

## Directory Structure

```
docker/
├── lifecycle.yaml
├── README.md
├── Linux.Debian/
│   ├── Deploy/
│   │   ├── prepare.star
│   │   ├── install.star
│   │   ├── provision.star
│   │   └── verify.star
│   ├── Upgrade/
│   │   ├── prepare.star
│   │   ├── install.star
│   │   └── verify.star
│   └── Decommission/
│       ├── unprovision.star
│       ├── uninstall.star
│       └── cleanup.star
├── Linux.Fedora/
│   ├── Deploy/
│   │   ├── prepare.star
│   │   ├── install.star
│   │   ├── provision.star
│   │   └── verify.star
│   ├── Upgrade/
│   │   ├── prepare.star
│   │   ├── install.star
│   │   └── verify.star
│   └── Decommission/
│       ├── unprovision.star
│       ├── uninstall.star
│       └── cleanup.star
├── Darwin/
│   ├── Deploy/
│   │   ├── prepare.star
│   │   ├── install.star
│   │   ├── provision.star
│   │   └── verify.star
│   ├── Upgrade/
│   │   └── install.star
│   │   └── verify.star
│   └── Decommission/
│       ├── unprovision.star
│       ├── uninstall.star
│       └── cleanup.star
└── Windows/
    ├── Deploy/
    │   ├── prepare.star
    │   ├── install.star
    │   ├── provision.star
    │   └── verify.star
    ├── Upgrade/
    │   ├── prepare.star
    │   ├── install.star
    │   └── verify.star
    └── Decommission/
        ├── unprovision.star
        ├── uninstall.star
        └── cleanup.star
```

## Phase Script API

Phase scripts receive two inputs. `plan` is a global.

```python
def install(package, phase):
    """
    Args:
        package: Package metadata and features (read-only, immediate)
        phase:   Phase context (name, action, retry)
    """
    plan.choose(
        when=plan.pkg.installed("conflicting-pkg"),
        then=lambda: plan.pkg.remove("conflicting-pkg"),
    )

    plan.pkg.install("docker-ce")

    if package.has_feature("rootless"):
        plan.pkg.install("uidmap")
```

**Scripts express intent, not commands.** Never shell out to package managers:

```python
# CORRECT
plan.package.install("docker-ce")

# WRONG
plan.shell("apt install docker-ce")
```

## Sources

- [Docker Engine Install — Ubuntu](https://docs.docker.com/engine/install/ubuntu/)
- [Docker Engine Install — RHEL](https://docs.docker.com/engine/install/rhel/)
- [Docker Desktop — macOS](https://docs.docker.com/desktop/install/mac-install/)
- [Docker Desktop — Windows](https://docs.docker.com/desktop/install/windows-install/)
- [ODROID-C4 Docker Fix](https://sipfront.com/blog/2024/01/running-docker-on-odroid-c4/)
