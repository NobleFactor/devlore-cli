# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Linux.Fedora/Deploy/prepare.star — Prepare phase
#
# TRIBAL KNOWLEDGE:
# Fedora/RHEL/CentOS have different conflicting packages than Debian.
# The Docker repo file can be downloaded directly (includes GPG key reference).
# Reference: https://docs.docker.com/engine/install/rhel/

def prepare(package, phase):
    """Prepare the system for Docker CE installation on Fedora/RHEL.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Remove conflicting packages
    # These are the Fedora/RHEL/CentOS packages that conflict with Docker CE
    conflicts = [
        "docker",
        "docker-client",
        "docker-client-latest",
        "docker-common",
        "docker-latest",
        "docker-latest-logrotate",
        "docker-logrotate",
        "docker-engine",
        "podman",
        "runc",
    ]

    for pkg in conflicts:
        plan.choose(
            when=plan.package.installed(pkg),
            then=lambda p=pkg: plan.package.remove(p),
        )

    # Install dnf plugins for repo management
    plan.package.install("dnf-plugins-core")

    # TODO: plan.download() not yet implemented
    # Download Docker's official repo file
    # This includes the GPG key reference, so no separate key download needed
    # plan.download(
    #     url="https://download.docker.com/linux/rhel/docker-ce.repo",
    #     dest="/etc/yum.repos.d/docker-ce.repo",
    # )

    # Update package lists to pick up the new repository
    plan.package.update()
