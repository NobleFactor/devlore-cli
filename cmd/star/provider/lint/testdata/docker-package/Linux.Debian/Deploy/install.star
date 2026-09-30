# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Linux.Debian/Deploy/install.star — Install phase
#
# TRIBAL KNOWLEDGE:
# Docker CE on Debian/Ubuntu consists of multiple packages:
# - docker-ce: The Docker daemon
# - docker-ce-cli: The docker command-line tool
# - containerd.io: The container runtime (Docker's version, not system)
# - docker-buildx-plugin: BuildKit-based builder
# - docker-compose-plugin: Compose V2 as a Docker plugin
# - lshw: For hardware detection in verify phase (ODROID support)
#
# Reference: https://docs.docker.com/engine/install/ubuntu/

def install(package, phase):
    """Install Docker CE packages.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Core Docker packages
    plan.package.install(
        "docker-ce",
        "docker-ce-cli",
        "containerd.io",
        "docker-buildx-plugin",
        "docker-compose-plugin",
    )

    # Hardware detection utility (needed for ODROID verification)
    plan.package.install("lshw")

    # Rootless mode requires additional packages
    if package.has_feature("rootless"):
        plan.package.install("uidmap", "dbus-user-session")
