#!/usr/bin/env pwsh
# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

<#
.SYNOPSIS
    Runs install.ps1 as a user does, in a scratch account, and checks what it did.

.DESCRIPTION
    Runs the checkout's install.ps1 under whichever PowerShell runs this script: Windows PowerShell 5.1
    (powershell.exe) or PowerShell 7 (pwsh). The XDG homes and the temporary directory are under one scratch
    directory, and every run passes -Prefix into it, so this machine's own installation and layer registrations are
    never touched. The one exception is `irm | iex`, which takes no parameters and installs into ~/.local: that case
    runs only in CI ($env:GITHUB_ACTIONS), on a machine thrown away afterwards.

    Prints PASS or FAIL for each check, with what the installer printed under each failure, and exits 1 if any
    check failed. Run by .github/workflows/installers.yaml on Windows, under both editions (#950, #965).

    Environment: $env:GH_TOKEN, optional, is passed to the installer, which sends it to GitHub's API;
    $env:DEVLORE_VERSION, optional, picks the release tag to install.

.EXAMPLE
    powershell.exe -NoProfile -File scripts/Test-InstallScript.ps1

.EXAMPLE
    pwsh -NoProfile -File scripts/Test-InstallScript.ps1
#>

#Requires -Version 5.1

[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# The checkout this script belongs to, whose install.ps1 is under test, and which also serves as a personal layer:
# a working-tree root.
$repo = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$installer = Join-Path $repo 'install.ps1'
$teamUrl = 'https://github.com/NobleFactor/noblefactor-ops.git'

# Windows PowerShell 5.1 has no $IsWindows, and under Set-StrictMode a variable that does not exist is an error, so
# the edition is read first: Desktop is 5.1, which runs on Windows alone.
$onWindows = ($PSVersionTable.PSEdition -ne 'Core') -or $IsWindows
$writName = if ($onWindows) { 'writ.exe' } else { 'writ' }

# This process's own executable, so a child session is the same edition as this one.
$shell = [System.Diagnostics.Process]::GetCurrentProcess().MainModule.FileName

$scratch = Join-Path ([System.IO.Path]::GetTempPath()) "devlore-install-test-$([System.Guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Path $scratch -Force | Out-Null

$script:failures = 0

###########
# Helper functions
###########

function Write-Pass {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Description
    )

    Write-Information -InformationAction Continue "PASS $Description"
}

function Write-Fail {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Description,

        [string]
        $Detail
    )

    $script:failures++
    Write-Information -InformationAction Continue "FAIL $Description"
    if ($Detail) {
        Write-Information -InformationAction Continue '----'
        Write-Information -InformationAction Continue $Detail
        Write-Information -InformationAction Continue '----'
    }
}

# Test-Expectation: PASS when the condition holds, else FAIL with the detail shown.
function Test-Expectation {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Description,

        [Parameter(Mandatory)]
        [bool]
        $Condition,

        [string]
        $Detail
    )

    if ($Condition) {
        Write-Pass -Description $Description
    } else {
        Write-Fail -Description $Description -Detail $Detail
    }
}

# Use-Account: points this process's XDG homes and temporary directory at a scratch account, which the installer
# and the writ it runs inherit. Returns the account's prefix.
function Use-Account {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [string]
        $Name
    )

    $account = Join-Path $scratch $Name
    $tmp = Join-Path $account 'tmp'
    New-Item -ItemType Directory -Path $tmp -Force | Out-Null
    $env:XDG_CONFIG_HOME = Join-Path $account 'config'
    $env:XDG_DATA_HOME = Join-Path $account 'data'
    $env:XDG_STATE_HOME = Join-Path $account 'state'
    $env:XDG_CACHE_HOME = Join-Path $account 'cache'
    $env:TMP = $tmp
    $env:TEMP = $tmp
    $env:TMPDIR = $tmp
    return (Join-Path $account 'prefix')
}

# Get-LayerRoot: the root writ reports for a layer, by the writ given.
function Get-LayerRoot {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [string]
        $Writ,

        [Parameter(Mandatory)]
        [string]
        $Layer
    )

    if (-not (Test-Path -LiteralPath $Writ)) {
        return ''
    }
    # writ leaves root out of an unregistered layer, and under strict mode reading a missing property throws, so the
    # property is looked up rather than read: a layer that failed to register is a FAIL with its detail, not a crash.
    $registrations = ((& $Writ repo list) -join "`n") | ConvertFrom-Json
    foreach ($registration in $registrations) {
        $root = $registration.PSObject.Properties['root']
        if ($registration.layer -eq $Layer -and $root) {
            return [string]$root.Value
        }
    }
    return ''
}

# Get-Registration: every registration writ reports, as one string to compare runs by.
function Get-Registration {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [string]
        $Writ
    )

    if (-not (Test-Path -LiteralPath $Writ)) {
        return ''
    }
    return ((& $Writ repo list) -join "`n")
}

# Invoke-Child: runs this edition of PowerShell as a separate process with the arguments given, its output streams
# redirected by the operating system into files, so no stream becomes an error record in this session. Returns the
# exit code and everything the child printed.
function Invoke-Child {
    [CmdletBinding()]
    [OutputType([pscustomobject])]
    param(
        [Parameter(Mandatory)]
        [string]
        $Name,

        [Parameter(Mandatory)]
        [string[]]
        $ArgumentList
    )

    $out = Join-Path $scratch "$Name.out"
    $err = Join-Path $scratch "$Name.err"
    $process = Start-Process -FilePath $shell -ArgumentList $ArgumentList -Wait -PassThru -NoNewWindow `
        -RedirectStandardOutput $out -RedirectStandardError $err
    return [pscustomobject]@{
        ExitCode = $process.ExitCode
        Output   = "$(Get-Content -Raw -LiteralPath $out)$(Get-Content -Raw -LiteralPath $err)"
    }
}

# ConvertTo-EncodedCommand: a command for -EncodedCommand, which takes it as base64 UTF-16 and needs no quoting.
function ConvertTo-EncodedCommand {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [string]
        $Command
    )

    return [System.Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($Command))
}

###########
# Main
###########

Write-Information -InformationAction Continue "PowerShell $($PSVersionTable.PSVersion) ($($PSVersionTable.PSEdition))"

$block = [scriptblock]::Create((Get-Content -Raw -LiteralPath $installer))
$savedVersion = $env:DEVLORE_VERSION

try {
    # --- The script-block form: a path layer and a URL layer, run twice; the second changes nothing ---

    $prefix = Use-Account -Name 'layers'
    $writ = Join-Path (Join-Path $prefix 'bin') $writName
    $runs = @()
    foreach ($run in 1, 2) {
        try {
            & $block -Prefix $prefix -Personal $repo -Team $teamUrl
            $runs += Get-Registration -Writ $writ
            Write-Pass -Description "script-block form: run $run completes"
        } catch {
            Write-Fail -Description "script-block form: run $run completes" -Detail $_.Exception.Message
        }
    }
    $personalRoot = Get-LayerRoot -Writ $writ -Layer 'personal'
    Test-Expectation -Description 'script-block form: personal is the checkout' -Condition ($personalRoot -eq $repo) `
        -Detail "personal: $personalRoot"
    $teamRoot = Get-LayerRoot -Writ $writ -Layer 'team'
    $expectedTeam = Join-Path (Join-Path (Join-Path (Join-Path $env:XDG_DATA_HOME 'devlore') 'writ') 'repos') 'noblefactor-ops'
    Test-Expectation -Description 'script-block form: team is cloned into the account' `
        -Condition ($teamRoot -eq $expectedTeam) -Detail "team: $teamRoot; expected: $expectedTeam"
    Test-Expectation -Description 'script-block form: the second run leaves the registrations as the first did' `
        -Condition ($runs.Count -eq 2 -and $runs[0] -and $runs[0] -eq $runs[1]) -Detail ($runs -join "`n----`n")

    # --- -Help prints the usage and returns ---

    $null = Use-Account -Name 'help'
    try {
        $help = (& $block -Help 6>&1 | Out-String)
        Test-Expectation -Description '-Help: prints the usage and returns' `
            -Condition ($help -match 'Usage: install\.ps1') -Detail $help
    } catch {
        Write-Fail -Description '-Help: prints the usage and returns' -Detail $_.Exception.Message
    }

    # --- A failure throws, and the session goes on (#965): an exit would end this script here ---

    $prefix = Use-Account -Name 'failure'
    $env:DEVLORE_VERSION = 'v0.0.0-no-such-tag'
    $message = ''
    try {
        & $block -Prefix $prefix
    } catch {
        $message = $_.Exception.Message
    } finally {
        $env:DEVLORE_VERSION = $savedVersion
    }
    Test-Expectation -Description 'a forced failure: throws its message, and the next statement runs' `
        -Condition ($message -match 'GitHub API error') -Detail "message: $message"

    # --- The caller's streams redirected: the run still finishes ---

    $prefix = Use-Account -Name 'redirected'
    $log = Join-Path $scratch 'redirected.log'
    try {
        & $block -Prefix $prefix -Personal $repo *>&1 | Tee-Object -FilePath $log | Out-Null
        Write-Pass -Description 'streams redirected (*>&1 | Tee-Object): the run finishes'
    } catch {
        $logged = if (Test-Path -LiteralPath $log) { Get-Content -Raw -LiteralPath $log } else { '' }
        Write-Fail -Description 'streams redirected (*>&1 | Tee-Object): the run finishes' `
            -Detail "$($_.Exception.Message)`n$logged"
    }

    # --- Run as a file: a failure exits 1, -Help exits 0 ---

    $prefix = Use-Account -Name 'file'
    $env:DEVLORE_VERSION = 'v0.0.0-no-such-tag'
    try {
        $failed = Invoke-Child -Name 'file-failure' -ArgumentList @(
            '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', "`"$installer`"", '-Prefix', "`"$prefix`"")
    } finally {
        $env:DEVLORE_VERSION = $savedVersion
    }
    Test-Expectation -Description 'as a file: a failure exits 1' -Condition ($failed.ExitCode -eq 1) `
        -Detail "exit $($failed.ExitCode)`n$($failed.Output)"
    $helped = Invoke-Child -Name 'file-help' -ArgumentList @(
        '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', "`"$installer`"", '-Help')
    Test-Expectation -Description 'as a file: -Help exits 0' -Condition ($helped.ExitCode -eq 0) `
        -Detail "exit $($helped.ExitCode)`n$($helped.Output)"

    # --- irm | iex with $env:DEVLORE_BASE, in a child session: it installs into ~/.local, so CI alone runs it ---

    if ($env:GITHUB_ACTIONS -eq 'true') {
        $null = Use-Account -Name 'iex'
        $command = @"
`$env:DEVLORE_BASE = '$repo'
Get-Content -Raw -LiteralPath '$installer' | Invoke-Expression
'after: session alive'
"@
        $iex = Invoke-Child -Name 'iex' -ArgumentList @(
            '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-EncodedCommand', (ConvertTo-EncodedCommand -Command $command))
        $iexWrit = Join-Path (Join-Path (Join-Path $HOME '.local') 'bin') $writName
        $baseRoot = Get-LayerRoot -Writ $iexWrit -Layer 'base'
        Test-Expectation -Description 'irm | iex with $env:DEVLORE_BASE: base registered, the session goes on' `
            -Condition ($baseRoot -eq $repo -and $iex.Output -match 'after: session alive') `
            -Detail "base: $baseRoot; exit $($iex.ExitCode)`n$($iex.Output)"
        Test-Expectation -Description 'irm | iex with $env:DEVLORE_BASE: team and personal skipped, last' `
            -Condition (([regex]::Matches($iex.Output, '(?m)^skipped: ')).Count -eq 2) -Detail $iex.Output
    } else {
        Write-Information -InformationAction Continue 'SKIP irm | iex: it installs into ~/.local, so it runs in CI alone'
    }
} finally {
    $env:DEVLORE_VERSION = $savedVersion

    # The layer links writ made go first, each unlinked on its own. They point at this checkout and at clones, which
    # must never be recursed into, and Windows PowerShell 5.1's Remove-Item -Recurse fails with a terminating error on
    # a directory symbolic link. A layer never registered is an empty directory (#840), which Delete() also removes. A
    # cleanup that fails is a warning, not a failed run, and it stops before Remove-Item can reach a link left in place.
    try {
        foreach ($account in Get-ChildItem -LiteralPath $scratch -Directory -Force) {
            $layers = Join-Path (Join-Path (Join-Path (Join-Path $account.FullName 'data') 'devlore') 'writ') 'layers'
            if (Test-Path -LiteralPath $layers) {
                foreach ($link in Get-ChildItem -LiteralPath $layers -Force) {
                    $link.Delete()
                }
            }
        }
        Remove-Item -LiteralPath $scratch -Recurse -Force
    } catch {
        Write-Warning "cleanup: $($_.Exception.Message)"
    }
}

if ($script:failures -gt 0) {
    Write-Information -InformationAction Continue "$($script:failures) check(s) failed"
    exit 1
}
Write-Information -InformationAction Continue 'every check passed'
