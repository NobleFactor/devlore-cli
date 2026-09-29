#!/usr/bin/env pwsh
# PowerShell: Initialize SSH identity on Windows
#
# A migrate fixture: it stands for what a user is migrating from, not for how we write scripts. It is
# linted anyway -- ruled 2026-09-28, everything is linted, no exceptions without a compelling argument
# -- so it carries the shebang and param() block the gate requires and nothing more. What it represents
# is the layout around it, which is unchanged.

[CmdletBinding()]
param()

$sshDir = "$env:USERPROFILE\.ssh"
if (-not (Test-Path $sshDir)) {
    New-Item -ItemType Directory -Path $sshDir
}
ssh-keygen -t ed25519 -f "$sshDir\id_ed25519"
