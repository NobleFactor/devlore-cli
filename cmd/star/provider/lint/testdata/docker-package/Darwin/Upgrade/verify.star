# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Darwin/Upgrade/verify.star — Verify phase for upgrade

def verify(package, phase):
    """Verify Docker Desktop upgrade on macOS.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Start Docker Desktop
    plan.shell.exec("open -a Docker")

    # Wait for daemon to be ready
    plan.shell.exec("while ! docker info >/dev/null 2>&1; do sleep 1; done")

    plan.verify("docker-daemon", check="docker info")
    plan.verify("docker-version", check="docker --version")
    plan.verify("hello-world", check="docker run --rm hello-world")
