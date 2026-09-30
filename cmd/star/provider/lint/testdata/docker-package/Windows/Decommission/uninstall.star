# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Windows/Decommission/uninstall.star — Uninstall phase
#
# TRIBAL KNOWLEDGE:
# Docker Desktop for Windows can be uninstalled via:
# - The installer with --uninstall flag
# - Control Panel > Programs > Uninstall
# Reference: https://docs.docker.com/desktop/uninstall/

def uninstall(package, phase):
    """Remove Docker Desktop from Windows.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Use the uninstaller if it exists
    plan.shell.exec(
        "\"%ProgramFiles%\\Docker\\Docker\\Docker Desktop Installer.exe\" uninstall --quiet 2>nul || echo Using fallback"
    )

    # TODO: plan.file.rmdir() not yet implemented
    # Fallback: remove program directory
    # plan.file.rmdir("%ProgramFiles%\\Docker")
