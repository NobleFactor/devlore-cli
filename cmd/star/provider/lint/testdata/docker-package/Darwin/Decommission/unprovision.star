# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Darwin/Decommission/unprovision.star — Unprovision phase

def unprovision(package, phase):
    """Stop Docker Desktop before removal.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Stop all running containers
    plan.shell.exec("docker stop $(docker ps -q) 2>/dev/null || true")

    # Quit Docker Desktop
    plan.shell.exec("osascript -e 'quit app \"Docker\"' 2>/dev/null || true")

    # Wait for Docker to fully stop
    plan.shell.exec("while pgrep -x Docker >/dev/null; do sleep 1; done")
