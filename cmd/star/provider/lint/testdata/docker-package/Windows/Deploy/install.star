# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Windows/Deploy/install.star — Install phase
#
# TRIBAL KNOWLEDGE:
# Docker Desktop installer supports silent installation with:
#   install --quiet --accept-license
# The installer handles all component setup including CLI tools.
# Reference: https://docs.docker.com/desktop/install/windows-install/

def install(package, phase):
    """Install Docker Desktop on Windows.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # TODO: plan.download() not yet implemented
    # plan.download(
    #     url="https://desktop.docker.com/win/main/amd64/Docker%20Desktop%20Installer.exe",
    #     dest="%TEMP%\\DockerDesktopInstaller.exe",
    # )

    plan.shell.exec(
        "%TEMP%\\DockerDesktopInstaller.exe install --quiet --accept-license"
    )

    # TODO: plan.file.remove() not yet implemented
    # plan.file.remove("%TEMP%\\DockerDesktopInstaller.exe")
