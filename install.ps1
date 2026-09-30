#!/usr/bin/env pwsh
# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

<#
.SYNOPSIS
    Installs lore, star and writ, and registers the layers given.

.DESCRIPTION
    Installs lore, star and writ into -Prefix (default ~/.local), then registers each layer given with
    writ repo set, base first. A layer not given is skipped and named at the end. Never asks. Running the
    same command again is safe: it is also how to recover from a failure.

    Runs on Windows PowerShell 5.1 and PowerShell 7, on Windows, macOS and Linux. The installer is served
    by the DevLore site's develop environment, from which devlore is released today.

    Environment: $env:DEVLORE_VERSION picks a release tag (default: the newest release, pre-releases
    included); $env:DEVLORE_TOOLS picks all, writ, lore or star (default: all); $env:GH_TOKEN, optional,
    is sent to GitHub's API, which lifts its limit of 60 anonymous requests an hour.

.PARAMETER Base
    The base layer: a working-tree root or a repository URL. Or set $env:DEVLORE_BASE; the parameter wins.

.PARAMETER Team
    The team layer. Or set $env:DEVLORE_TEAM.

.PARAMETER Personal
    The personal layer. Or set $env:DEVLORE_PERSONAL.

.PARAMETER Prefix
    The installation prefix. Default: ~/.local, on every platform.

.PARAMETER Help
    Show the usage and return.

.EXAMPLE
    & ([scriptblock]::Create((irm https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net/install.ps1))) `
        -Base <loc> -Team <loc> -Personal <loc>

    The preferred form: its parameters bind by name, and it leaves nothing behind in the session.

.EXAMPLE
    $env:DEVLORE_BASE = '<loc>'; irm https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net/install.ps1 | iex
#>

#Requires -Version 5.1

[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSReviewUnusedParameter', '',
    Justification = 'Prefix is read by Main below -- it defaults there to ~/.local, becomes the bin
    directory, and is passed to `self install`. PowerShell resolves a script-scope parameter inside a
    function in the same script dynamically, and the analyzer cannot see across that boundary. Deleting
    it would silently drop a documented flag.')]
[CmdletBinding()]
param(
    [string]$Base,
    [string]$Team,
    [string]$Personal,
    [string]$Prefix,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Where the DevLore site serves this script. The site releases from develop, so this is its develop environment.
$InstallersUrl = "https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net"

# `return`, not `exit`: under `irm | iex` and the script-block form the script is not a file, and `exit` would end
# the user's PowerShell session (#965). Run as a file, returning still exits 0.
if ($Help) {
    Write-Information -InformationAction Continue "Usage: install.ps1 [-Prefix <dir>] [-Base <loc>] [-Team <loc>] [-Personal <loc>]"
    Write-Information -InformationAction Continue ''
    Write-Information -InformationAction Continue "Installs lore, star and writ into <prefix> (default ~/.local), then registers each layer given"
    Write-Information -InformationAction Continue "with writ repo set, base first. A layer not given is skipped and named at the end. Never asks."
    Write-Information -InformationAction Continue ''
    Write-Information -InformationAction Continue "  -Prefix <dir>     installation prefix (default: ~/.local)"
    Write-Information -InformationAction Continue "  -Base <loc>       the base layer: a working-tree root or a repository URL (or `$env:DEVLORE_BASE)"
    Write-Information -InformationAction Continue "  -Team <loc>       the team layer (or `$env:DEVLORE_TEAM)"
    Write-Information -InformationAction Continue "  -Personal <loc>   the personal layer (or `$env:DEVLORE_PERSONAL)"
    Write-Information -InformationAction Continue ''
    Write-Information -InformationAction Continue "Environment: DEVLORE_VERSION (a release tag), DEVLORE_TOOLS (all, writ, lore or star), GH_TOKEN (optional)."
    Write-Information -InformationAction Continue ''
    Write-Information -InformationAction Continue "Served by the DevLore site's develop environment, from which devlore is released today:"
    Write-Information -InformationAction Continue "  & ([scriptblock]::Create((irm $InstallersUrl/install.ps1))) -Base <loc> -Team <loc> -Personal <loc>"
    return
}

# -------------------------------------------------------------------
# Configuration
# -------------------------------------------------------------------

$GitHubRepo = "NobleFactor/devlore-cli"
$GitHubApi = "https://api.github.com/repos/$GitHubRepo"

$Version = if ($env:DEVLORE_VERSION) { $env:DEVLORE_VERSION } else { "latest" }
$Tools = if ($env:DEVLORE_TOOLS) { $env:DEVLORE_TOOLS } else { "all" }

# Each layer's variable is its default; a parameter wins over it (#950).
$Base = if ($Base) { $Base } elseif ($env:DEVLORE_BASE) { $env:DEVLORE_BASE } else { '' }
$Team = if ($Team) { $Team } elseif ($env:DEVLORE_TEAM) { $env:DEVLORE_TEAM } else { '' }
$Personal = if ($Personal) { $Personal } elseif ($env:DEVLORE_PERSONAL) { $env:DEVLORE_PERSONAL } else { '' }

# GitHub authentication (optional). The repository is public; a token only lifts the API's anonymous rate limit.
# Per https://docs.github.com/en/rest/releases/assets
# Note: Use "token" not "Bearer" for OAuth tokens from gh auth
$AuthToken = $env:GH_TOKEN

# -------------------------------------------------------------------
# Helpers
# -------------------------------------------------------------------

# Each level goes to the stream PowerShell already has for it: information, warning, error.
#
# Write-Host went to none of them. It cannot be captured or redirected, so a user whose install failed
# could not pipe the run to a file or paste a log -- which for an installer people run under
# `irm | iex`, and in CI, is exactly when a log matters. Separate streams also let a caller silence
# warnings and keep progress. A failure ends the script with a terminating error, which a process's stderr
# captures (`pwsh -File install.ps1 2> err.txt`); an in-session `2>` on the one-liner does not (#965).
#
# `-InformationAction Continue` is on every Write-Information call, so progress appears whatever
# $InformationPreference the caller's session carries. Write-Warning is visible by default.
#
# The color is not replaced. $PSStyle is PowerShell 7.2 and later, and this script must run on Windows
# PowerShell 5.1, which is what a fresh Windows machine has (#948). ANSI escapes are not reliable there
# either. The streams carry the distinction the color used to, and Write-Warning and the error record label
# their own output, so `info:` and `success:` are the only prefixes left.

function Write-Info {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Message
    )

    Write-Information -InformationAction Continue "info: $Message"
}

function Write-Success {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Message
    )

    Write-Information -InformationAction Continue "success: $Message"
}

function Write-Warn {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Message
    )

    Write-Warning $Message
}

function Write-Fatal {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Message
    )

    # A terminating error, not `exit`. Under `irm | iex` and the script-block form the script is not a file, so
    # `exit` would end the user's PowerShell session and take the message with it (#965). The error ends the
    # script, runs Main's finally, and leaves the session and the message on screen. Run as a file, an uncaught
    # terminating error still exits 1. It terminates because the script pins $ErrorActionPreference = 'Stop'.
    throw $Message
}

# Detect OS
#
# Windows PowerShell 5.1 has no $IsWindows, $IsMacOS or $IsLinux, and under Set-StrictMode a variable that
# does not exist is an error, so the edition is read first: Desktop is 5.1, which runs on Windows alone.
# The automatic variables are consulted only on Core, where they exist.
function Get-OSName {
    [CmdletBinding()]
    [OutputType([string])]
    param()

    if ($PSVersionTable.PSEdition -ne 'Core') {
        return "windows"
    }
    if ($IsWindows) {
        return "windows"
    } elseif ($IsMacOS) {
        return "darwin"
    } elseif ($IsLinux) {
        return "linux"
    } else {
        Write-Fatal "Unsupported operating system"
    }
}

# Detect architecture. The releases publish amd64 and arm64 only.
function Get-ArchName {
    [CmdletBinding()]
    [OutputType([string])]
    param()

    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($arch) {
        'X64'   { return "amd64" }
        'Arm64' { return "arm64" }
        default { Write-Fatal "Unsupported architecture: $arch" }
    }
}

# Build common headers for GitHub API requests, authenticated when GH_TOKEN is set
function Get-ApiHeader {
    [CmdletBinding()]
    [OutputType([hashtable])]
    param([string]$Accept = "application/vnd.github+json")
    $headers = @{ Accept = $Accept }
    if ($AuthToken) {
        $headers["Authorization"] = "token $AuthToken"
    }
    return $headers
}

# Make an API request
# Per https://docs.github.com/en/rest/releases/releases
#
# -UseBasicParsing on every web call: without it Windows PowerShell 5.1 parses responses with the Internet
# Explorer engine, which hangs on a machine that has never run IE's first-launch dialog. PowerShell 7
# accepts the switch and ignores it.
function Invoke-ApiGet {
    [CmdletBinding()]
    param([string]$Url)
    $headers = Get-ApiHeader
    Invoke-RestMethod -Uri $Url -Headers $headers -UseBasicParsing -ErrorAction Stop
}

# Get latest release version from GitHub API
# Per https://docs.github.com/en/rest/releases/releases#list-releases
# Uses /releases?per_page=1 to get the most recent release (including prereleases)
# Note: /releases/latest excludes prereleases, so we use the list endpoint instead
function Get-LatestVersion {
    [CmdletBinding()]
    param()

    $url = "$GitHubApi/releases?per_page=1"
    $releases = Invoke-ApiGet -Url $url
    if (-not $releases -or $releases.Count -eq 0) {
        return $null
    }
    return $releases[0].tag_name
}

# Get release by tag
# Per https://docs.github.com/en/rest/releases/releases#get-a-release-by-tag-name
function Get-ReleaseByTag {
    [CmdletBinding()]
    param([string]$Tag)
    $url = "$GitHubApi/releases/tags/$Tag"
    Invoke-ApiGet -Url $url
}

# Download release asset by ID
# Per https://docs.github.com/en/rest/releases/assets#get-a-release-asset
# Must use Accept: application/octet-stream to get binary content
function Save-ReleaseAsset {
    [CmdletBinding()]
    param([string]$AssetId, [string]$Destination)
    $url = "$GitHubApi/releases/assets/$AssetId"
    $headers = Get-ApiHeader -Accept "application/octet-stream"
    Invoke-WebRequest -Uri $url -Headers $headers -OutFile $Destination -UseBasicParsing -ErrorAction Stop
}

# Run a native command with $ErrorActionPreference at 'Continue' for its duration.
#
# Windows PowerShell 5.1 turns each line a native command writes to stderr into an error record whenever a caller has
# redirected the error stream (`*>&1 | Tee-Object`, `2>&1`), and this script's 'Stop' makes the first such line fatal.
# lore, star and writ narrate on stderr, so a user who logged the install lost it at the first progress line (found by
# the installers' CI, #950). PowerShell 7.2 and later leave native stderr alone. The exit code decides: every caller
# checks $LASTEXITCODE after this returns.
function Invoke-NativeCommand {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $FilePath,

        [Parameter(Mandatory)]
        [string[]]
        $ArgumentList
    )

    $ErrorActionPreference = 'Continue'
    & $FilePath @ArgumentList
}

# Verify checksum
function Test-Checksum {
    [CmdletBinding()]
    param([string]$File, [string]$Expected)
    $actual = (Get-FileHash -Path $File -Algorithm SHA256).Hash.ToLower()
    if ($actual -ne $Expected.ToLower()) {
        Write-Fatal "Checksum verification failed!`nExpected: $Expected`nActual:   $actual"
    }
}

# -------------------------------------------------------------------
# Main
# -------------------------------------------------------------------

function Main {
    [CmdletBinding()]
    param()

    Write-Info "DevLore CLI Installer"
    Write-Information -InformationAction Continue ''

    # A layer is registered by the writ this run installs, so a run that leaves writ out cannot register one.
    # Refused before anything is downloaded (#950).
    if (($Base -or $Team -or $Personal) -and $Tools -ne "all" -and $Tools -ne "writ") {
        Write-Fatal "-Base, -Team and -Personal register layers with writ, which DEVLORE_TOOLS=$Tools does not install"
    }

    # GitHub speaks TLS 1.2 and later. Windows PowerShell 5.1 takes its protocols from the .NET Framework's
    # default, which on an older machine stops at 1.0, so every request fails before it starts. Adding 1.2
    # is a no-op wherever the default already includes it, PowerShell 7 included.
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor [System.Net.SecurityProtocolType]::Tls12

    # Detect platform
    $os = Get-OSName
    $arch = Get-ArchName
    Write-Info "Detected platform: $os/$arch"

    # Default prefix: ~/.local on every platform. XDG conventions hold on Windows too (ruled 2026-09-22, #903), and
    # star's extension loader looks under ~/.local/share, so a prefix anywhere else installs extensions nothing loads.
    if (-not $Prefix) {
        $Prefix = Join-Path $HOME ".local"
    }
    $installDir = Join-Path $Prefix "bin"

    # Resolve version
    if ($Version -eq "latest") {
        Write-Info "Fetching latest version..."
        $Version = Get-LatestVersion
        if (-not $Version) {
            Write-Fatal "Could not determine the latest release of $GitHubRepo"
        }
    }
    Write-Info "Version: $Version"

    # Get release info
    Write-Info "Fetching release info..."
    try {
        $release = Get-ReleaseByTag -Tag $Version
    } catch {
        Write-Fatal "GitHub API error: $($_.Exception.Message)"
    }

    # Determine archive extension
    $ext = if ($os -eq "windows") { "zip" } else { "tar.gz" }

    # Build asset names
    $archiveName = "devlore-cli_${Version}_${os}_${arch}.${ext}"
    $checksumsName = "devlore-cli_${Version}_checksums.txt"

    # Find assets by name
    $archiveAsset = $release.assets | Where-Object { $_.name -eq $archiveName }
    if (-not $archiveAsset) {
        Write-Fatal "Asset $archiveName not found in release $Version"
    }
    $checksumsAsset = $release.assets | Where-Object { $_.name -eq $checksumsName }

    # Create temp directory
    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "devlore-install-$([System.Guid]::NewGuid().ToString('N'))"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        # Download archive via GitHub API
        Write-Info "Downloading $archiveName..."
        $archivePath = Join-Path $tmpDir $archiveName
        Save-ReleaseAsset -AssetId $archiveAsset.id -Destination $archivePath

        # Download and verify checksum
        if ($checksumsAsset) {
            Write-Info "Verifying checksum..."
            $checksumsPath = Join-Path $tmpDir "checksums.txt"
            Save-ReleaseAsset -AssetId $checksumsAsset.id -Destination $checksumsPath

            $checksumLine = Get-Content $checksumsPath | Where-Object { $_ -match $archiveName }
            if ($checksumLine) {
                $expectedChecksum = ($checksumLine -split '\s+')[0]
                Test-Checksum -File $archivePath -Expected $expectedChecksum
                Write-Success "Checksum verified"
            } else {
                Write-Warn "Checksum not found for $archiveName, skipping verification"
            }
        } else {
            Write-Warn "Checksums file not found, skipping verification"
        }

        # Extract archive
        #
        # The archive holds the products at its root and star's extensions under share/ (#903). The products move
        # to pkg/bin so that each one's `self install` finds pkg/share at <exeDir>/../share, the path star copies
        # its extensions from. A native command's exit code is checked, because try/catch never sees it.
        Write-Info "Extracting..."
        $pkg = Join-Path $tmpDir "pkg"
        $pkgBin = Join-Path $pkg "bin"
        New-Item -ItemType Directory -Path $pkgBin -Force | Out-Null
        if ($ext -eq "zip") {
            Expand-Archive -Path $archivePath -DestinationPath $pkg -Force
        } else {
            # tar.gz -- PowerShell 7+ on macOS/Linux has tar available
            Invoke-NativeCommand -FilePath tar -ArgumentList @('--extract', '--gzip', '--file', $archivePath, '--directory', $pkg)
            if ($LASTEXITCODE -ne 0) {
                Write-Fatal "tar exited $LASTEXITCODE extracting $archiveName"
            }
        }

        # Create install directory
        if (-not (Test-Path $installDir)) {
            New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        }

        # Install binaries
        #
        # Every file at the archive root is a product, so this list is the archive's and not a second copy of the
        # Makefile's. Each product installs itself: `self install <prefix>` copies the binary to <prefix>/bin and
        # adds its man pages, completions and, for star, its extensions. A failure means that product is not
        # installed, so it is fatal.
        $installed = @()

        foreach ($file in Get-ChildItem -LiteralPath $pkg -File) {
            $product = [System.IO.Path]::GetFileNameWithoutExtension($file.Name)
            if ($Tools -ne "all" -and $Tools -ne $product) {
                continue
            }

            $toolPath = Join-Path $pkgBin $file.Name
            Move-Item -LiteralPath $file.FullName -Destination $toolPath -Force
            if ($os -ne "windows") {
                Invoke-NativeCommand -FilePath chmod -ArgumentList @('+x', $toolPath)
                if ($LASTEXITCODE -ne 0) {
                    Write-Fatal "chmod exited $LASTEXITCODE on $product"
                }
            }

            Write-Info "Installing $product..."
            Push-Location $pkg
            try {
                Invoke-NativeCommand -FilePath $toolPath -ArgumentList @('self', 'install', $Prefix, '--unattended')
                if ($LASTEXITCODE -ne 0) {
                    Write-Fatal "$product self install failed with exit code $LASTEXITCODE"
                }
            } finally {
                Pop-Location
            }
            $installed += $product
        }

        if ($installed.Count -eq 0) {
            Write-Fatal "No binaries found in archive for DEVLORE_TOOLS=$Tools"
        }

        # Register the layers given, base first, with the writ just installed (#950). The call runs in the user's
        # working directory, so a relative location resolves where it was typed. writ's output is its own, and so are
        # its errors: a failure ends the run, and running the same command again is the recovery. --unattended is
        # writ's contract for a run nobody is there to answer.
        $writPath = Join-Path $installDir $(if ($os -eq "windows") { "writ.exe" } else { "writ" })
        $layers = [ordered]@{ base = $Base; team = $Team; personal = $Personal }
        $registered = @()
        $skipped = @()

        foreach ($layer in $layers.Keys) {
            $location = $layers[$layer]
            if (-not $location) {
                $skipped += $layer
                continue
            }

            Write-Info "Registering ${layer}: $location"
            Invoke-NativeCommand -FilePath $writPath -ArgumentList @('repo', 'set', $layer, $location, '--unattended')
            if ($LASTEXITCODE -ne 0) {
                Write-Fatal "writ repo set $layer exited $LASTEXITCODE"
            }
            $registered += $layer
        }

        # The summary comes last, after writ's output, and its last lines are the layers skipped.
        Write-Information -InformationAction Continue ''
        Write-Success "Installed: $($installed -join ', ')"
        Write-Success "Location: $installDir"
        if ($registered.Count -gt 0) {
            Write-Success "Registered: $($registered -join ', ')"
        }
        Write-Information -InformationAction Continue ''

        # Check if install dir is in PATH
        $pathDirs = $env:PATH -split [System.IO.Path]::PathSeparator
        if ($installDir -notin $pathDirs) {
            Write-Warn "$installDir is not in your PATH"
            Write-Information -InformationAction Continue ''
            if ($os -eq "windows") {
                Write-Information -InformationAction Continue "Add it to your PATH (run as Administrator):"
                Write-Information -InformationAction Continue ''
                Write-Information -InformationAction Continue "  [Environment]::SetEnvironmentVariable('Path',"
                Write-Information -InformationAction Continue "    `"$installDir;`" + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')"
                Write-Information -InformationAction Continue ''
                Write-Information -InformationAction Continue "Or add to your PowerShell profile (`$PROFILE):"
                Write-Information -InformationAction Continue ''
                Write-Information -InformationAction Continue "  `$env:PATH = `"$installDir;`$env:PATH`""
                Write-Information -InformationAction Continue ''
            } else {
                Write-Information -InformationAction Continue "Add it to your shell profile:"
                Write-Information -InformationAction Continue ''
                Write-Information -InformationAction Continue "  # For PowerShell (`$PROFILE)"
                Write-Information -InformationAction Continue "  `$env:PATH = `"$installDir`:`$env:PATH`""
                Write-Information -InformationAction Continue ''
            }
        }

        # Verify installation
        if ($installDir -in $pathDirs) {
            Write-Information -InformationAction Continue "Verify installation:"
            foreach ($tool in $installed) {
                Write-Information -InformationAction Continue "  $tool --version"
            }
        }

        Write-Information -InformationAction Continue ''
        Write-Info "Documentation: https://github.com/NobleFactor/devlore-cli#readme"
        Write-Information -InformationAction Continue ''
        Write-Info "Next steps:"
        if ($registered.Count -gt 0) {
            Write-Information -InformationAction Continue "  writ deploy"
        }
        foreach ($layer in $skipped) {
            Write-Information -InformationAction Continue "skipped: $layer; to register it later:"
            Write-Information -InformationAction Continue "  writ repo set $layer <working-tree-root>|<repository-url>"
        }

    } finally {
        # Clean up temp directory
        Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Main
