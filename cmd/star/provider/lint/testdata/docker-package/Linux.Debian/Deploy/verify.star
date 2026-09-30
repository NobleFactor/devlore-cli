# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Linux.Debian/Deploy/verify.star — Verify phase
#
# TRIBAL KNOWLEDGE:
# 1. The hello-world container is the canonical Docker smoke test.
#
# 2. ODROID-C4/C5 may fail verification if cgroup v1 mode not enabled.
#    The verify phase detects this and provides manual remediation steps.
#
# 3. User may need to logout/login for docker group membership to work.
#    If running as non-root without group membership, verification uses sudo.

def verify(package, phase):
    """Verify Docker installation is functional.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Verify docker command exists
    plan.verify("docker-command", check="which docker")

    # Verify docker daemon is running
    plan.verify("docker-daemon", check="docker info")

    # Verify docker compose is available
    plan.verify("docker-compose", check="docker compose version")

    # Run hello-world smoke test
    plan.verify("hello-world", check="docker run --rm hello-world")

    # Check for ODROID hardware that may need boot.ini modification
    # This is informational - logged as a note if detected
    plan.verify(
        "hardware-check",
        check="lshw -json 2>/dev/null | grep -q 'ODROID-C[45]' && echo 'ODROID detected - verify cgroup v1 mode if containers fail'",
        optional=True,
    )
