# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Linux.Fedora/Decommission/uninstall.star — Uninstall phase

def uninstall(package, phase):
    """Remove Docker CE packages on Fedora/RHEL.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    plan.package.remove(
        "docker-ce",
        "docker-ce-cli",
        "containerd.io",
        "docker-buildx-plugin",
        "docker-compose-plugin",
    )

    if package.has_feature("rootless"):
        plan.package.remove("fuse-overlayfs", "slirp4netns")

    # TODO: plan.package.autoremove() not yet implemented
    # plan.package.autoremove()
