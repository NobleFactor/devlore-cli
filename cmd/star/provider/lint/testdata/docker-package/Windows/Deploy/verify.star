# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Windows/Deploy/verify.star — Verify phase

def verify(package, phase):
    """Verify Docker Desktop installation on Windows.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Wait for Docker daemon to be ready
    plan.shell.exec("powershell -Command \"while (-not (docker info 2>$null)) { Start-Sleep -Seconds 1 }\"")

    plan.verify("docker-daemon", check="docker info")
    plan.verify("docker-version", check="docker --version")
    plan.verify("hello-world", check="docker run --rm hello-world")
