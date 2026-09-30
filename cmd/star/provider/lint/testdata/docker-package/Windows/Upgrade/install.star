# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Windows/Upgrade/install.star — Install phase for upgrade
#
# Docker Desktop upgrade is the same as fresh install - download new
# installer and run it. It overwrites the existing installation.

def install(package, phase):
    """Upgrade Docker Desktop on Windows.

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
