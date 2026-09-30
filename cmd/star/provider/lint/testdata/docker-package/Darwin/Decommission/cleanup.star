# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Darwin/Decommission/cleanup.star — Cleanup phase
#
# TRIBAL KNOWLEDGE:
# Docker Desktop stores data in several locations:
# - ~/Library/Group Containers/group.com.docker/
# - ~/Library/Containers/com.docker.docker/
# - ~/Library/Application Support/Docker Desktop/
# - ~/.docker/

def cleanup(package, phase):
    """Clean up Docker Desktop data on macOS.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # TODO: phase.env() and plan.file.rmdir() need implementation
    # user_home = phase.env("HOME")

    # Remove Docker Desktop preferences and state
    # plan.file.rmdir("%s/Library/Application Support/Docker Desktop" % user_home)
    # plan.file.rmdir("%s/Library/Preferences/com.docker.docker.plist" % user_home)
    # plan.file.rmdir("%s/Library/Saved Application State/com.electron.docker-frontend.savedState" % user_home)

    # Remove Docker CLI config
    # plan.file.rmdir("%s/.docker" % user_home)

    # Optionally purge all Docker data (images, containers, volumes)
    if package.has_feature("purge-data"):
        # plan.file.rmdir("%s/Library/Group Containers/group.com.docker" % user_home)
        # plan.file.rmdir("%s/Library/Containers/com.docker.docker" % user_home)
        pass
