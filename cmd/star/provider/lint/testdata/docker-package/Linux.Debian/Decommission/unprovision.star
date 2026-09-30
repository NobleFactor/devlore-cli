# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Linux.Debian/Decommission/unprovision.star — Unprovision phase
#
# TRIBAL KNOWLEDGE:
# Before removing Docker, we should:
# 1. Stop any running containers
# 2. Stop and disable the Docker service
# 3. Optionally remove user from docker group

def unprovision(package, phase):
    """Remove Docker provisioning before uninstall.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Stop all running containers
    plan.shell.exec("docker stop $(docker ps -q) 2>/dev/null || true")

    # Stop and disable Docker service
    plan.service.stop("docker")
    plan.service.disable("docker")

    # Stop and disable containerd
    plan.service.stop("containerd")
    plan.service.disable("containerd")

    # TODO: plan.user.remove_from_group() not yet implemented
    # Remove current user from docker group
    # user = phase.env("USER")
    # if user and user != "root":
    #     plan.user.remove_from_group(user, "docker")
