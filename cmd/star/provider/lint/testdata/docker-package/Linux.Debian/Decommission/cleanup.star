# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Linux.Debian/Decommission/cleanup.star — Cleanup phase
#
# TRIBAL KNOWLEDGE:
# Docker leaves behind several directories that may contain user data:
# - /var/lib/docker: Images, containers, volumes
# - /var/lib/containerd: Containerd state
# - /etc/docker: Configuration (daemon.json)
# - /etc/apt/sources.list.d/docker.list: Apt repository
# - /etc/apt/keyrings/docker.asc: GPG key
#
# This phase removes configuration but preserves data by default.
# Use --with purge-data to remove everything including images/volumes.

def cleanup(package, phase):
    """Clean up Docker configuration and optionally data.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Remove apt repository configuration
    # TODO: plan.file.remove() not yet implemented
    # plan.file.remove("/etc/apt/sources.list.d/docker.list")
    # plan.file.remove("/etc/apt/keyrings/docker.asc")

    # Remove Docker daemon configuration
    # plan.file.remove("/etc/docker/daemon.json")
    # plan.file.rmdir("/etc/docker")

    # Optionally purge all Docker data (images, containers, volumes)
    # This is destructive and requires explicit opt-in
    if package.has_feature("purge-data"):
        # TODO: plan.file.rmdir() not yet implemented
        # plan.file.rmdir("/var/lib/docker")
        # plan.file.rmdir("/var/lib/containerd")
        pass
