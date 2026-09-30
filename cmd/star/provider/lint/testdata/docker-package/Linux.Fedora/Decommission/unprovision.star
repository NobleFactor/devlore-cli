# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Linux.Fedora/Decommission/unprovision.star — Unprovision phase

def unprovision(package, phase):
    """Remove Docker provisioning on Fedora/RHEL.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    plan.shell.exec("docker stop $(docker ps -q) 2>/dev/null || true")

    plan.service.stop("docker")
    plan.service.disable("docker")
    plan.service.stop("containerd")
    plan.service.disable("containerd")

    # TODO: plan.user.remove_from_group() not yet implemented
    # user = phase.env("USER")
    # if user and user != "root":
    #     plan.user.remove_from_group(user, "docker")
