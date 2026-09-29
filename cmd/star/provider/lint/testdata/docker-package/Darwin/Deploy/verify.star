# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Darwin/Deploy/verify.star — Verify phase

def verify(package, phase):
    """Verify Docker Desktop installation on macOS.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Verify Docker.app is installed
    plan.verify(
        "docker-app",
        check="test -d /Applications/Docker.app",
    )

    # Verify docker CLI is available
    plan.verify("docker-command", check="which docker")

    # Verify daemon is running
    plan.verify("docker-daemon", check="docker info")

    # Verify docker compose
    plan.verify("docker-compose", check="docker compose version")

    # Run hello-world smoke test
    plan.verify("hello-world", check="docker run --rm hello-world")
