# SPDX-License-Identifier: MIT
# Copyright (c) 2025-2026 Noble Factor. All rights reserved.
#
# DevLore CLI Installer (PowerShell)
# Usage: irm https://devlore.noblefactor.com/install.ps1 | iex
#        .\install.ps1 -Prefix "C:\devlore"
#
# For private repo (requires GitHub token):
#   $env:GH_TOKEN = (gh auth token); irm https://devlore.noblefactor.com/install.ps1 | iex
#
# Parameters:
#   -Prefix <dir>        - Installation prefix (default: ~/.local on Unix, ~/AppData/Local/DevLore on Windows)
#                          Binaries go to <prefix>/bin
#
# Environment variables:
#   GH_TOKEN             - GitHub token for private repo access
#                          Use: $env:GH_TOKEN = (gh auth token) for OAuth token
#   DEVLORE_VERSION      - Version to install (default: latest)
#                          "latest" installs the most recent release (including prereleases)
#                          Set explicitly (e.g., "v1.0.0") for a specific version
#   DEVLORE_TOOLS        - Tools to install: "all", "writ", "lore" (default: all)
#
# Documentation references:
#   - GitHub Releases API: https://docs.github.com/en/rest/releases/releases
#   - GitHub Release Assets API: https://docs.github.com/en/rest/releases/assets

[CmdletBinding()]
param(
    [string]$Prefix,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($Help) {
    Write-Host "Usage: install.ps1 [-Prefix <dir>]"
    Write-Host "  -Prefix <dir>  Installation prefix (default: ~/.local or ~/AppData/Local/DevLore)"
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
function Get-OSName {
    if ($IsWindows -or [System.Environment]::OSVersion.Platform -eq 'Win32NT') {
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
function Invoke-ApiGet {
    param([string]$Url)
    $headers = Get-ApiHeaders
    Invoke-RestMethod -Uri $Url -Headers $headers -ErrorAction Stop
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
    Invoke-WebRequest -Uri $url -Headers $headers -OutFile $Destination -ErrorAction Stop
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

    # Check for auth token (required for private repo)
    if (-not $AuthToken) {
        Write-Warn "No GH_TOKEN set. This will fail for private repositories."
        Write-Warn "Set GH_TOKEN with a token that has 'Contents' read permission."
    }

    # Detect platform
    $os = Get-OSName
    $arch = Get-ArchName
    Write-Info "Detected platform: $os/$arch"

    # Resolve default prefix based on OS
    if (-not $Prefix) {
        if ($os -eq "windows") {
            $Prefix = Join-Path $env:LOCALAPPDATA "DevLore"
        } else {
            $Prefix = Join-Path $HOME ".local"
        }
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
        Write-Info "Extracting..."
        if ($ext -eq "zip") {
            Expand-Archive -Path $archivePath -DestinationPath $tmpDir -Force
        } else {
            # tar.gz — PowerShell 7+ on macOS/Linux has tar available
            tar -xzf $archivePath -C $tmpDir
        }

        # Create install directory
        if (-not (Test-Path $installDir)) {
            New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        }

        # Install binaries
        $installed = @()
        $binExt = if ($os -eq "windows") { ".exe" } else { "" }

        if ($Tools -eq "all" -or $Tools -eq "writ") {
            $writBin = "writ$binExt"
            $writPath = Join-Path $tmpDir $writBin
            if (Test-Path $writPath) {
                Copy-Item $writPath (Join-Path $installDir $writBin) -Force
                if ($os -ne "windows") { chmod +x (Join-Path $installDir $writBin) }
                $installed += "writ"
            }
        }

        if ($Tools -eq "all" -or $Tools -eq "lore") {
            $loreBin = "lore$binExt"
            $lorePath = Join-Path $tmpDir $loreBin
            if (Test-Path $lorePath) {
                Copy-Item $lorePath (Join-Path $installDir $loreBin) -Force
                if ($os -ne "windows") { chmod +x (Join-Path $installDir $loreBin) }
                $installed += "lore"
            }
        }

        if ($installed.Count -eq 0) {
            Write-Fatal "No binaries found in archive"
        }

        # Run self-install for each tool to install man pages and completions
        foreach ($tool in $installed) {
            Write-Info "Running $tool self-install..."
            $toolPath = Join-Path $installDir "$tool$binExt"
            try {
                & $toolPath self-install --prefix="$Prefix" --unattended
            } catch {
                Write-Warn "$tool self-install failed"
            }
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
