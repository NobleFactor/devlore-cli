# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Linux.Debian/Upgrade/install.star — Install phase for upgrade

def install(package, phase):
    """Upgrade Docker CE packages to latest version.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Upgrade all Docker packages
    # The package manager handles version resolution
    plan.package.upgrade(
        "docker-ce",
        "docker-ce-cli",
        "containerd.io",
        "docker-buildx-plugin",
        "docker-compose-plugin",
    )

    # Upgrade rootless packages if feature enabled
    if package.has_feature("rootless"):
        plan.package.upgrade("uidmap", "dbus-user-session")
