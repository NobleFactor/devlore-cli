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

    Nothing is installed that is not verified. The archive is checked against the release's checksums file
    before anything is extracted, and refused when it cannot be: when the release has no checksums file, when
    the file has no line for the archive, or when the two disagree. Both files are downloaded from the
    release's public links, the checksums file first.

    Runs on Windows PowerShell 5.1 and PowerShell 7, on Windows, macOS and Linux. The installer is served
    by the DevLore site's develop environment, from which devlore is released today.

    Environment: $env:DEVLORE_VERSION picks a release tag (default: the newest release, pre-releases
    included); $env:DEVLORE_TOOLS picks all, writ, lore or star (default: all); $env:GH_TOKEN, optional,
    is sent to GitHub's API alone, which lifts its limit of 60 anonymous requests an hour.

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
    Write-Information -InformationAction Continue `
        "The archive is verified against the release's checksums file first, and refused if it cannot be."
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
# A release's files, each by its public link, <tag>/<name>: its browser_download_url, which self upgrade downloads too.
$GitHubDownload = "https://github.com/$GitHubRepo/releases/download"

$Version = if ($env:DEVLORE_VERSION) { $env:DEVLORE_VERSION } else { "latest" }
$Tools = if ($env:DEVLORE_TOOLS) { $env:DEVLORE_TOOLS } else { "all" }

# Each layer's variable is its default; a parameter wins over it (#950).
$Base = if ($Base) { $Base } elseif ($env:DEVLORE_BASE) { $env:DEVLORE_BASE } else { '' }
$Team = if ($Team) { $Team } elseif ($env:DEVLORE_TEAM) { $env:DEVLORE_TEAM } else { '' }
$Personal = if ($Personal) { $Personal } elseif ($env:DEVLORE_PERSONAL) { $env:DEVLORE_PERSONAL } else { '' }

# GitHub authentication (optional). The repository is public; a token only lifts the API's anonymous rate limit, and is
# sent to the API alone, never to a download link.
# Per https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api
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
        $description = [System.Runtime.InteropServices.RuntimeInformation]::OSDescription
        Write-Fatal "Unsupported operating system: $description"
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

# The HTTP status GitHub answered a failed request with, or 0 when it never answered. Both editions attach the response
# to the exception: an HttpWebResponse on Windows PowerShell 5.1, an HttpResponseMessage on PowerShell 7, each with its
# StatusCode. Every property is looked up rather than read: they differ by edition and by failure, and under
# Set-StrictMode reading one that does not exist is itself an error.
function Get-HttpStatus {
    [CmdletBinding()]
    [OutputType([int])]
    param(
        [Parameter(Mandatory)]
        [System.Management.Automation.ErrorRecord]
        $ErrorRecord
    )

    $response = $ErrorRecord.Exception.PSObject.Properties['Response']

    if (-not $response -or -not $response.Value) {
        return 0
    }

    $statusCode = $response.Value.PSObject.Properties['StatusCode']

    if (-not $statusCode -or -not $statusCode.Value) {
        return 0
    }

    return [int]$statusCode.Value
}

# What GitHub answered a request that failed, for the error that reports it: "GitHub answered HTTP <status>: <message>",
# as install.sh words it.
#
# GitHub explains a refusal in the body it sends with it: its API in JSON, under "message" ("Not Found", "API rate limit
# exceeded for ..."), its download host in plain text ("Not Found"). As install.sh reads it, the message is the text's
# first line when the body is plain text, and otherwise the JSON's "message". The web cmdlets of both editions keep the
# body in the error record's ErrorDetails, and the content type in the response they attach: an HttpWebResponse's
# ContentType on Windows PowerShell 5.1, the content's headers of an HttpResponseMessage on PowerShell 7. That message
# is what the user is shown (#1002): the cmdlet's own, "Response status code does not indicate success" on PowerShell 7
# and "The remote server returned an error" on Windows PowerShell 5.1, names only the status. A request that never
# reached GitHub has no answer, so its error is reported as PowerShell words it.
function Get-GitHubAnswer {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [System.Management.Automation.ErrorRecord]
        $ErrorRecord
    )

    $status = Get-HttpStatus -ErrorRecord $ErrorRecord

    if (-not $status) {
        return $ErrorRecord.Exception.Message
    }

    $contentType = ''
    $received = $ErrorRecord.Exception.Response
    $typeProperty = $received.PSObject.Properties['ContentType']
    $contentProperty = $received.PSObject.Properties['Content']

    if ($typeProperty) {
        $contentType = [string]$typeProperty.Value
    }
    elseif ($contentProperty -and $contentProperty.Value) {
        $contentType = [string]$contentProperty.Value.Headers.ContentType
    }

    $body = ''

    if ($ErrorRecord.ErrorDetails -and $ErrorRecord.ErrorDetails.Message) {
        $body = $ErrorRecord.ErrorDetails.Message
    }

    $message = ''

    if ($contentType.StartsWith('text/plain', [System.StringComparison]::Ordinal)) {
        $message = ($body -split "`n", 2)[0] -replace '\r$', ''
    }
    elseif ($body) {
        try {
            $json = $body | ConvertFrom-Json
        }
        catch {
            $json = $null
        }

        if ($json) {
            $field = $json.PSObject.Properties['message']

            if ($field -and $field.Value) {
                $message = [string]$field.Value
            }
        }
    }

    $answer = "GitHub answered HTTP $status"

    if ($message) {
        $answer += ": $message"
    }

    return $answer
}

# The value of header Name in the response GitHub refused a request with, or '' when it sent none. Windows PowerShell 5.1
# attaches an HttpWebResponse, whose Headers is a WebHeaderCollection; PowerShell 7 an HttpResponseMessage, whose
# Headers gives a header's values through TryGetValues. Each property is looked up rather than read, as in
# Get-HttpStatus.
function Get-ResponseHeader {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [System.Management.Automation.ErrorRecord]
        $ErrorRecord,

        [Parameter(Mandatory)]
        [string]
        $Name
    )

    $response = $ErrorRecord.Exception.PSObject.Properties['Response']

    if (-not $response -or -not $response.Value) {
        return ''
    }

    $headers = $response.Value.PSObject.Properties['Headers']

    if (-not $headers -or $null -eq $headers.Value) {
        return ''
    }

    $collection = $headers.Value

    if ($collection -is [System.Net.WebHeaderCollection]) {
        $value = $collection[$Name]

        if ($value) {
            return [string]$value
        }

        return ''
    }

    $values = $null

    if ($collection.TryGetValues($Name, [ref]$values)) {
        return [string](@($values)[0])
    }

    return ''
}

# What to tell the user when GitHub's API has refused a request for its rate limit, or '' when it refused for another
# reason, as install.sh's rate_limit_message says it. GitHub refuses a spent limit with 403 or 429,
# x-ratelimit-remaining 0, and x-ratelimit-reset, the time the limit resets in seconds since the epoch, which is said as
# this machine's clock shows it and in minutes, rounded up. GH_TOKEN raises the limit, which is said when it is unset.
# Per https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api
function Get-RateLimitMessage {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [System.Management.Automation.ErrorRecord]
        $ErrorRecord
    )

    $status = Get-HttpStatus -ErrorRecord $ErrorRecord

    if (($status -ne 403 -and $status -ne 429) -or
        (Get-ResponseHeader -ErrorRecord $ErrorRecord -Name 'x-ratelimit-remaining') -cne '0') {
        return ''
    }

    $reset = Get-ResponseHeader -ErrorRecord $ErrorRecord -Name 'x-ratelimit-reset'

    if ($reset -cnotmatch '^[0-9]+$') {
        return ''
    }

    # From the epoch by hand: DateTimeOffset.FromUnixTimeSeconds is .NET Framework 4.6, and Windows PowerShell 5.1 runs
    # on 4.5.2 too. HH:mm in the invariant culture, whose time separator is ':', whatever the user's culture.
    $epoch = New-Object -TypeName System.DateTime -ArgumentList 1970, 1, 1, 0, 0, 0, ([System.DateTimeKind]::Utc)
    $clock = $epoch.AddSeconds([double]$reset).ToLocalTime().ToString('HH:mm',
        [System.Globalization.CultureInfo]::InvariantCulture)
    $now = [long][System.Math]::Floor(([System.DateTime]::UtcNow - $epoch).TotalSeconds)
    $minutes = [long][System.Math]::Floor(([long]$reset - $now + 59) / 60)

    if ($minutes -lt 1) {
        $minutes = 1
    }

    $unit = 'minutes'

    if ($minutes -eq 1) {
        $unit = 'minute'
    }

    # GitHub counts the limit per token when one is sent, and per address when none is.
    $whose = 'this address'

    if ($AuthToken) {
        $whose = 'your token'
    }

    $message = "GitHub's API limit for $whose is used up. It resets at $clock (in $minutes $unit); run the " +
        "installer again after that."

    if (-not $AuthToken) {
        $message += ' Setting GH_TOKEN raises the limit.'
    }

    return $message
}

# Make an API request, authenticated when GH_TOKEN is set, and return GitHub's answer. A refusal is an error saying
# "Could not <What>", with GitHub's answer, followed, when the refusal is for GitHub's rate limit, by when to run the
# installer again, as install.sh's api_get says it.
# Per https://docs.github.com/en/rest/releases/releases
#
# -UseBasicParsing on every web call: without it Windows PowerShell 5.1 parses responses with the Internet
# Explorer engine, which hangs on a machine that has never run IE's first-launch dialog. PowerShell 7
# accepts the switch and ignores it.
function Invoke-ApiGet {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Url,

        [Parameter(Mandatory)]
        [string]
        $What
    )

    $headers = Get-ApiHeader

    try {
        Invoke-RestMethod -Uri $Url -Headers $headers -UseBasicParsing -ErrorAction Stop
    }
    catch {
        $answer = Get-GitHubAnswer -ErrorRecord $_
        $limit = Get-RateLimitMessage -ErrorRecord $_

        if ($limit) {
            $answer += "`n$limit"
        }

        Write-Fatal "Could not ${What}: $answer"
    }
}

# Get latest release version from GitHub API: the newest release's tag, or $null when the repository has none
# Per https://docs.github.com/en/rest/releases/releases#list-releases
# Uses /releases?per_page=1 to get the most recent release (including prereleases)
# Note: /releases/latest excludes prereleases, so we use the list endpoint instead
function Get-LatestVersion {
    [CmdletBinding()]
    param()

    $releases = Invoke-ApiGet -Url "$GitHubApi/releases?per_page=1" -What "list the releases of $GitHubRepo"

    if (-not $releases -or $releases.Count -eq 0) {
        return $null
    }

    return $releases[0].tag_name
}

# Require release Tag, which is an error, with GitHub's message, when GitHub has no such release. Its JSON is not read:
# a download link answers 404 alike for a release that doesn't exist and for a file it doesn't have, and this is how a
# release that doesn't exist is told apart, and reported as that.
# Per https://docs.github.com/en/rest/releases/releases#get-a-release-by-tag-name
function Assert-Release {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Tag
    )

    $null = Invoke-ApiGet -Url "$GitHubApi/releases/tags/$Tag" -What "fetch release $Tag of $GitHubRepo"
}

# The name of the file GitHub served, from the Content-Disposition of the response a download returned, or '' when it
# names none. A header's name is matched in any case; PowerShell 7 gives a header's values as an array, Windows
# PowerShell 5.1 as one string.
function Get-ServedName {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [object]
        $Response
    )

    $name = ''

    foreach ($header in $Response.Headers.GetEnumerator()) {
        if ($header.Key -ne 'Content-Disposition') {
            continue
        }

        foreach ($value in @($header.Value)) {
            if ($value -cmatch 'filename="?([^";]+)') {
                $name = $Matches[1]
            }
        }
    }

    return $name
}

# Download release Tag's file Name, by its public link, to Destination, whose name is the file's. The link is public, so
# GH_TOKEN is not sent; it redirects to where GitHub keeps the file, and the cmdlet follows it. A refusal is an error
# naming the file and the release, with GitHub's status and message, which its download host sends as plain text ("Not
# Found"), unless it is a 404 before the release is found (-ReleaseFound) and GitHub's API has no such release, which
# Assert-Release reports. That host takes a name in any case, and names the file it served in its Content-Disposition:
# a file by another name is not this one, and is refused (#1002). -PassThru returns the response, for that header, as
# well as saving the file.
# Per https://docs.github.com/en/repositories/releasing-projects-on-github/linking-to-releases
function Save-ReleaseFile {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Name,

        [Parameter(Mandatory)]
        [string]
        $Tag,

        [Parameter(Mandatory)]
        [string]
        $Destination,

        [switch]
        $ReleaseFound
    )

    try {
        $response = Invoke-WebRequest -Uri "$GitHubDownload/$Tag/$Name" -OutFile $Destination -PassThru `
            -UseBasicParsing -ErrorAction Stop
    }
    catch {
        $failure = $_

        if ((Get-HttpStatus -ErrorRecord $failure) -eq 404 -and -not $ReleaseFound) {
            Assert-Release -Tag $Tag
        }

        Write-Fatal "Could not download $Name from release ${Tag}: $(Get-GitHubAnswer -ErrorRecord $failure)"
    }

    $served = Get-ServedName -Response $response

    if ($served -and -not [string]::Equals($served, $Name, [System.StringComparison]::Ordinal)) {
        Write-Fatal "Could not download $Name from release ${Tag}: GitHub served $served, a file by another name"
    }
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

# The SHA-256 a checksums file lists for one file, found by exact name, or $null when no line names it.
#
# A line is 64 hex characters, a space, then a space or '*', then the name, exactly and with case; a trailing carriage
# return is ignored, and the first line naming the file wins. That is self upgrade's rule (listedChecksum, in
# cmd/internal/cli/selfupgrade_archive.go), so the installers, the guide and self upgrade read one format. A pattern
# would not do: in -match a '.' matches any character, nothing is anchored, and case is ignored (#1002).
#
# Exactly means character for character, so the name is compared ordinally. -ceq compares by culture, which ignores
# characters such as a soft hyphen or a zero-width space, and so takes a line naming another file. The file is read as
# bytes and decoded as UTF-8, as self upgrade reads it: ReadAllText would drop a byte-order mark, and decode UTF-16,
# taking a line that self upgrade and sha256sum refuse.
function Get-ListedChecksum {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [string]
        $Path,

        [Parameter(Mandatory)]
        [string]
        $Name
    )

    $text = [System.Text.Encoding]::UTF8.GetString([System.IO.File]::ReadAllBytes($Path))

    foreach ($line in ($text -split "`n")) {
        $fields = $line.TrimEnd("`r") -split ' ', 2

        if ($fields.Count -ne 2 -or $fields[0] -cnotmatch '^[0-9a-fA-F]{64}$') {
            continue
        }

        $rest = $fields[1]

        if ($rest.Length -gt 1 -and ($rest[0] -ceq ' ' -or $rest[0] -ceq '*') -and
            [string]::Equals($rest.Substring(1), $Name, [System.StringComparison]::Ordinal)) {
            return $fields[0].ToLowerInvariant()
        }
    }

    return $null
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

    # Resolve version. $releaseFound says whether GitHub has shown that release $Version exists, by listing it as the
    # newest or by serving one of its files. A release DEVLORE_VERSION names is not looked up before its files are
    # downloaded: GitHub's API is asked for it only when a link answers 404 before then (#1008).
    $releaseFound = $false

    if ($Version -eq "latest") {
        Write-Info "Fetching latest version..."
        $Version = Get-LatestVersion

        if (-not $Version) {
            Write-Fatal "Could not determine the latest release of ${GitHubRepo}: GitHub lists none"
        }

        $releaseFound = $true
    }

    Write-Info "Version: $Version"

    # Determine archive extension
    $ext = if ($os -eq "windows") { "zip" } else { "tar.gz" }

    # Build asset names
    $archiveName = "devlore-cli_${Version}_${os}_${arch}.${ext}"
    $checksumsName = "devlore-cli_${Version}_checksums.txt"

    # Create temp directory
    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "devlore-install-$([System.Guid]::NewGuid().ToString('N'))"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        # Download the checksums file, then the archive, each by its public link (#1008). An archive the release gives
        # no way to verify is not installed (#1002): as self upgrade does, the archive is downloaded only when the
        # checksums file has its line.
        Write-Info "Downloading $checksumsName..."
        $checksumsPath = Join-Path $tmpDir $checksumsName
        Save-ReleaseFile -Name $checksumsName -Tag $Version -Destination $checksumsPath -ReleaseFound:$releaseFound
        $releaseFound = $true
        $expectedChecksum = Get-ListedChecksum -Path $checksumsPath -Name $archiveName

        if (-not $expectedChecksum) {
            Write-Fatal "$checksumsName has no line for $archiveName, so the archive cannot be verified"
        }

        Write-Info "Downloading $archiveName..."
        $archivePath = Join-Path $tmpDir $archiveName
        Save-ReleaseFile -Name $archiveName -Tag $Version -Destination $archivePath -ReleaseFound:$releaseFound

        # Verify the archive before anything is extracted
        Write-Info "Verifying checksum..."
        Test-Checksum -File $archivePath -Expected $expectedChecksum
        Write-Success "Checksum verified"

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
                    Write-Fatal "$product self install failed"
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
        Write-Success "Installed: $($installed -join ' ')"
        Write-Success "Location: $installDir"
        if ($registered.Count -gt 0) {
            Write-Success "Registered: $($registered -join ' ')"
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
