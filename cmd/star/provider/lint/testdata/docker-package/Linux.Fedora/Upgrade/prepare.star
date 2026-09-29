# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Linux.Fedora/Upgrade/prepare.star — Prepare phase for upgrade

def prepare(package, phase):
    """Prepare for Docker CE upgrade on Fedora/RHEL.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # TODO: plan.fail() not yet implemented
    # plan.choose(
    #     when=plan.package.not_installed("docker-ce"),
    #     then=lambda: plan.fail("Docker CE is not installed - use 'lore deploy docker' instead"),
    # )

    plan.package.update()
