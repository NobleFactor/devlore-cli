# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Darwin/Deploy/install.star — Install phase
#
# TRIBAL KNOWLEDGE:
# Docker Desktop for Mac is distributed as a DMG containing Docker.app.
# Installation is: mount DMG, run installer CLI, detach DMG.
# Different DMGs for Apple Silicon (arm64) vs Intel (amd64).
# Reference: https://docs.docker.com/desktop/setup/install/mac-install/

def install(package, phase):
    """Install Docker Desktop on macOS.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # TODO: platform.arch needs implementation
    # Select correct DMG for architecture
    # arch = platform.arch
    # if arch == "arm64":
    #     dmg_url = "https://desktop.docker.com/mac/main/arm64/Docker.dmg"
    # else:
    #     dmg_url = "https://desktop.docker.com/mac/main/amd64/Docker.dmg"

    # TODO: plan.download() not yet implemented
    # Download Docker Desktop DMG
    # plan.download(
    #     url=dmg_url,
    #     dest="/tmp/Docker.dmg",
    # )

    # Mount the DMG
    plan.shell.exec("hdiutil attach /tmp/Docker.dmg -nobrowse -quiet")

    # TODO: phase.env() needs implementation
    # Run the installer with license acceptance
    # --user flag performs privileged setup during install
    # user = phase.env("USER")
    # plan.shell.exec(
    #     "/Volumes/Docker/Docker.app/Contents/MacOS/install --accept-license --user=%s" % user
    # )

    # Detach the DMG
    plan.shell.exec("hdiutil detach /Volumes/Docker -quiet")

    # TODO: plan.file.remove() not yet implemented
    # Clean up downloaded DMG
    # plan.file.remove("/tmp/Docker.dmg")
