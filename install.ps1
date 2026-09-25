# SPDX-License-Identifier: MIT
# Copyright (c) 2025-2026 Noble Factor. All rights reserved.
#
# DevLore CLI Installer (PowerShell)
# Usage: irm https://devlore.noblefactor.com/install.ps1 | iex
#        .\install.ps1 -Prefix "C:\devlore"
#
# Runs on Windows PowerShell 5.1 -- what a fresh Windows machine has -- and on PowerShell 7, on Windows,
# macOS and Linux, one code path. It installs nothing but lore, star and writ (#798, ruled 2026-09-24).
# The irm | iex form needs no execution-policy change; a Windows client's default policy refuses a
# downloaded .ps1 run as a file, so the second form wants `-ExecutionPolicy Bypass` on the pwsh or
# powershell command line.
#
# For private repo (requires GitHub token):
#   $env:GH_TOKEN = (gh auth token); irm https://devlore.noblefactor.com/install.ps1 | iex
#
# Parameters:
#   -Prefix <dir>        - Installation prefix (default: ~/.local, on every platform)
#                          Binaries go to <prefix>/bin
#
# Environment variables:
#   GH_TOKEN             - GitHub token for private repo access
#                          Use: $env:GH_TOKEN = (gh auth token) for OAuth token
#   DEVLORE_VERSION      - Version to install (default: latest)
#                          "latest" installs the most recent release (including prereleases)
#                          Set explicitly (e.g., "v1.0.0") for a specific version
#   DEVLORE_TOOLS        - Tools to install: "all", or one product: "writ", "lore", "star" (default: all)
#
# Documentation references:
#   - GitHub Releases API: https://docs.github.com/en/rest/releases/releases
#   - GitHub Release Assets API: https://docs.github.com/en/rest/releases/assets

#Requires -Version 5.1

[CmdletBinding()]
param(
    [string]$Prefix,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($Help) {
    Write-Host "Usage: install.ps1 [-Prefix <dir>]"
    Write-Host "  -Prefix <dir>  Installation prefix (default: ~/.local)"
    exit 0
}

# -------------------------------------------------------------------
# Configuration
# -------------------------------------------------------------------

$GitHubRepo = "NobleFactor/devlore-cli"
$GitHubApi = "https://api.github.com/repos/$GitHubRepo"

$Version = if ($env:DEVLORE_VERSION) { $env:DEVLORE_VERSION } else { "latest" }
$Tools = if ($env:DEVLORE_TOOLS) { $env:DEVLORE_TOOLS } else { "all" }

# GitHub authentication (required for private repo)
# Per https://docs.github.com/en/rest/releases/assets - requires "Contents" read permission
# Note: Use "token" not "Bearer" for OAuth tokens from gh auth
$AuthToken = $env:GH_TOKEN

# -------------------------------------------------------------------
# Helpers
# -------------------------------------------------------------------

function Write-Info { param([string]$Message) Write-Host "info: $Message" -ForegroundColor Blue }
function Write-Success { param([string]$Message) Write-Host "success: $Message" -ForegroundColor Green }
function Write-Warn { param([string]$Message) Write-Host "warning: $Message" -ForegroundColor Yellow }
function Write-Fatal {
    param([string]$Message)
    Write-Host "error: $Message" -ForegroundColor Red
    exit 1
}

# Detect OS
#
# Windows PowerShell 5.1 has no $IsWindows, $IsMacOS or $IsLinux, and under Set-StrictMode a variable that
# does not exist is an error, so the edition is read first: Desktop is 5.1, which runs on Windows alone.
# The automatic variables are consulted only on Core, where they exist.
function Get-OSName {
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

# Detect architecture
function Get-ArchName {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($arch) {
        'X64'   { return "amd64" }
        'Arm64' { return "arm64" }
        'Arm'   { return "armv7" }
        default { Write-Fatal "Unsupported architecture: $arch" }
    }
}

# Build common headers for GitHub API requests
function Get-ApiHeaders {
    param([string]$Accept = "application/vnd.github+json")
    $headers = @{ Accept = $Accept }
    if ($AuthToken) {
        $headers["Authorization"] = "token $AuthToken"
    }
    return $headers
}

# Make authenticated API request
# Per https://docs.github.com/en/rest/releases/releases
#
# -UseBasicParsing on every web call: without it Windows PowerShell 5.1 parses responses with the Internet
# Explorer engine, which hangs on a machine that has never run IE's first-launch dialog. PowerShell 7
# accepts the switch and ignores it.
function Invoke-ApiGet {
    param([string]$Url)
    $headers = Get-ApiHeaders
    Invoke-RestMethod -Uri $Url -Headers $headers -UseBasicParsing -ErrorAction Stop
}

# Get latest release version from GitHub API
# Per https://docs.github.com/en/rest/releases/releases#list-releases
# Uses /releases?per_page=1 to get the most recent release (including prereleases)
# Note: /releases/latest excludes prereleases, so we use the list endpoint instead
function Get-LatestVersion {
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
    param([string]$Tag)
    $url = "$GitHubApi/releases/tags/$Tag"
    Invoke-ApiGet -Url $url
}

# Download release asset by ID
# Per https://docs.github.com/en/rest/releases/assets#get-a-release-asset
# Must use Accept: application/octet-stream to get binary content
function Save-ReleaseAsset {
    param([string]$AssetId, [string]$Destination)
    $url = "$GitHubApi/releases/assets/$AssetId"
    $headers = Get-ApiHeaders -Accept "application/octet-stream"
    Invoke-WebRequest -Uri $url -Headers $headers -OutFile $Destination -UseBasicParsing -ErrorAction Stop
}

# Verify checksum
function Test-Checksum {
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
    Write-Info "DevLore CLI Installer"
    Write-Host ""

    # GitHub speaks TLS 1.2 and later. Windows PowerShell 5.1 takes its protocols from the .NET Framework's
    # default, which on an older machine stops at 1.0, so every request fails before it starts. Adding 1.2
    # is a no-op wherever the default already includes it, PowerShell 7 included.
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor [System.Net.SecurityProtocolType]::Tls12

    # Check for auth token (required for private repo)
    if (-not $AuthToken) {
        Write-Warn "No GH_TOKEN set. This will fail for private repositories."
        Write-Warn "Set GH_TOKEN with a token that has 'Contents' read permission."
    }

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
            Write-Fatal ("Could not determine latest version. Check GH_TOKEN has correct permissions.`n" +
                "For private repos, token needs 'Contents' read permission.")
        }
    }
    Write-Info "Version: $Version"

    # Get release info
    Write-Info "Fetching release info..."
    try {
        $release = Get-ReleaseByTag -Tag $Version
    } catch {
        Write-Fatal "GitHub API error: $($_.Exception.Message)`nCheck GH_TOKEN has Contents read permission."
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
        # its extensions from.
        Write-Info "Extracting..."
        $pkg = Join-Path $tmpDir "pkg"
        $pkgBin = Join-Path $pkg "bin"
        New-Item -ItemType Directory -Path $pkgBin -Force | Out-Null
        if ($ext -eq "zip") {
            Expand-Archive -Path $archivePath -DestinationPath $pkg -Force
        } else {
            # tar.gz — PowerShell 7+ on macOS/Linux has tar available
            tar -xzf $archivePath -C $pkg
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
        # installed, so it is fatal. A native command's exit code is checked, because try/catch never sees it.
        $installed = @()

        foreach ($file in Get-ChildItem -LiteralPath $pkg -File) {
            $product = [System.IO.Path]::GetFileNameWithoutExtension($file.Name)
            if ($Tools -ne "all" -and $Tools -ne $product) {
                continue
            }

            $toolPath = Join-Path $pkgBin $file.Name
            Move-Item -LiteralPath $file.FullName -Destination $toolPath -Force
            if ($os -ne "windows") { chmod +x $toolPath }

            Write-Info "Installing $product..."
            Push-Location $pkg
            try {
                & $toolPath self install $Prefix --unattended
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

        Write-Host ""
        Write-Success "Installed: $($installed -join ', ')"
        Write-Success "Location: $installDir"
        Write-Host ""

        # Check if install dir is in PATH
        $pathDirs = $env:PATH -split [System.IO.Path]::PathSeparator
        if ($installDir -notin $pathDirs) {
            Write-Warn "$installDir is not in your PATH"
            Write-Host ""
            if ($os -eq "windows") {
                Write-Host "Add it to your PATH (run as Administrator):"
                Write-Host ""
                Write-Host "  [Environment]::SetEnvironmentVariable('Path',"
                Write-Host "    `"$installDir;`" + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')"
                Write-Host ""
                Write-Host "Or add to your PowerShell profile (`$PROFILE):"
                Write-Host ""
                Write-Host "  `$env:PATH = `"$installDir;`$env:PATH`""
                Write-Host ""
            } else {
                Write-Host "Add it to your shell profile:"
                Write-Host ""
                Write-Host "  # For PowerShell (`$PROFILE)"
                Write-Host "  `$env:PATH = `"$installDir`:`$env:PATH`""
                Write-Host ""
            }
        }

        # Verify installation
        if ($installDir -in $pathDirs) {
            Write-Host "Verify installation:"
            foreach ($tool in $installed) {
                Write-Host "  $tool --version"
            }
        }

        Write-Host ""
        Write-Info "Next steps:"
        Write-Host "  Adopt files:      writ adopt --project <name> <file>..."
        Write-Host "  Migrate existing: writ migrate <directory>"
        Write-Host ""
        Write-Info "Documentation: https://devlore.noblefactor.com"

    } finally {
        # Clean up temp directory
        Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Main
