# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Windows/Decommission/cleanup.star — Cleanup phase
#
# TRIBAL KNOWLEDGE:
# Docker Desktop stores data in several locations:
# - %LOCALAPPDATA%\Docker\
# - %APPDATA%\Docker\
# - %USERPROFILE%\.docker\
# - WSL 2 distros: docker-desktop and docker-desktop-data

def cleanup(package, phase):
    """Clean up Docker Desktop data on Windows.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # TODO: plan.file.rmdir() not yet implemented
    # Remove Docker Desktop settings and state
    # plan.file.rmdir("%LOCALAPPDATA%\\Docker")
    # plan.file.rmdir("%APPDATA%\\Docker")
    # plan.file.rmdir("%APPDATA%\\Docker Desktop")

    # Remove Docker CLI config
    # plan.file.rmdir("%USERPROFILE%\\.docker")

    # Optionally purge all Docker data (images, containers, volumes)
    if package.has_feature("purge-data"):
        # Remove WSL 2 Docker distros
        plan.shell.exec("wsl --unregister docker-desktop 2>nul || echo Not found")
        plan.shell.exec("wsl --unregister docker-desktop-data 2>nul || echo Not found")
