# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Darwin/Deploy/prepare.star — Prepare phase
#
# TRIBAL KNOWLEDGE:
# Docker Desktop for Mac is a proprietary application, not Docker CE.
# Licensing: Free for small businesses (<250 employees, <$10M revenue),
# personal use, education, non-commercial OSS. Requires paid subscription
# for larger enterprises.
#
# Apple Silicon Macs benefit from Rosetta 2 for some CLI tools.
# Reference: https://docs.docker.com/desktop/setup/install/mac-install/

def prepare(package, phase):
    """Prepare for Docker Desktop installation on macOS.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Check macOS version (Docker Desktop requires current or 2 previous major versions)
    # This is informational - the installer will fail if unsupported
    plan.verify(
        "macos-version",
        check="sw_vers -productVersion",
        optional=True,
    )

    # Install Rosetta 2 on Apple Silicon (recommended but not required)
    # TODO: platform.arch and path predicates need implementation
    # arch = platform.arch
    # if arch == "arm64":
    #     plan.shell.exec("softwareupdate --install-rosetta --agree-to-license")

    # TODO: plan.notify() not yet implemented
    # Quit applications that might interfere with Docker installation
    # (VS Code, terminals with Docker plugins, etc.)
    # plan.notify(
    #     "Please quit applications that may use Docker (VS Code, terminals) before proceeding."
    # )
    pass
