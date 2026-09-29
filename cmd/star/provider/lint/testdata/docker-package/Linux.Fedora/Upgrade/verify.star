# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Linux.Fedora/Upgrade/verify.star — Verify phase for upgrade

def verify(package, phase):
    """Verify Docker upgrade on Fedora/RHEL.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    plan.service.restart("docker")
    plan.verify("docker-daemon", check="docker info")
    plan.verify("docker-version", check="docker --version")
    plan.verify("hello-world", check="docker run --rm hello-world")
