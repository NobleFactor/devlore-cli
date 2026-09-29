# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Darwin/Upgrade/prepare.star — Prepare phase for upgrade

def prepare(package, phase):
    """Prepare for Docker Desktop upgrade on macOS.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # TODO: path predicates need implementation
    # plan.choose(
    #     when=plan.file.not_exists("/Applications/Docker.app"),
    #     then=lambda: plan.fail("Docker Desktop is not installed - use 'lore deploy docker' instead"),
    # )

    # Quit Docker Desktop before upgrade
    plan.shell.exec("osascript -e 'quit app \"Docker\"' 2>/dev/null || true")

    # Wait for Docker to fully stop
    plan.shell.exec("while pgrep -x Docker >/dev/null; do sleep 1; done")
