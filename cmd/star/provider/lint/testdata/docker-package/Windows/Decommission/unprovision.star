# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# docker/Windows/Decommission/unprovision.star — Unprovision phase

def unprovision(package, phase):
    """Stop Docker Desktop before removal.

    Args:
        package: Package metadata and features (read-only, immediate)
        phase: Lifecycle phase context (controls plan, provides metadata)
    """

    # Stop all running containers
    plan.shell.exec("docker stop $(docker ps -q) 2>nul || echo No containers running")

    # Quit Docker Desktop
    plan.shell.exec("taskkill /IM \"Docker Desktop.exe\" /F 2>nul || echo Docker Desktop not running")
    plan.shell.exec("taskkill /IM \"com.docker.backend.exe\" /F 2>nul || echo Backend not running")

    # Wait for processes to stop
    plan.shell.exec("powershell -Command \"while (Get-Process 'Docker Desktop' -ErrorAction SilentlyContinue) { Start-Sleep -Seconds 1 }\"")
