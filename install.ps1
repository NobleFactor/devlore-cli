#!/usr/bin/env pwsh
# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

<#
.SYNOPSIS
    Installs lore, star and writ, and registers the layers given.

.DESCRIPTION
    Installs lore, star and writ into -Prefix (default ~/.local), then registers each layer given with
    writ repo set, base first. A layer not given is skipped and named at the end, unless writ already has it
    registered. Never asks. Running the same command again is safe: it is also how to recover from a failure.

    Nothing is installed that is not verified. The archive is checked against the release's checksums file
    before anything is extracted, and refused when it cannot be: when the release has no checksums file, when
    the file has no line for the archive, or when the two disagree. Both files are downloaded from the
    release's public links, the checksums file first.

    Runs on Windows, under Windows PowerShell 5.1 and PowerShell 7; on Linux and macOS, install.sh installs. The
    installer is served by the DevLore site's develop environment, from which devlore is released today.

    It speaks through PowerShell's streams, as a cmdlet does: its results on the output stream, a warning on the
    warning stream, a failure as an error record. What it is doing goes to the verbose stream: give -Verbose, or set
    $VerbosePreference = 'Continue' before the irm | iex form, to see it. A failure leaves the session open; run as a
    file, the script exits 1.

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

using namespace System.Linq

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

# $ErrorActionPreference is set inside Install-DevLore, never here: under `irm | iex` this script runs in the user's own
# scope, and setting it here would change their session's (#1044, D4).

# Where the DevLore site serves this script. The site releases from develop, so this is its develop environment.
$InstallersUrl = "https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net"

# `return`, not `exit`: under `irm | iex` and the script-block form the script is not a file, and `exit` would end
# the user's PowerShell session (#965). Run as a file, returning still exits 0.
if ($Help) {
    Write-Output -InputObject @(
        "Usage: install.ps1 [-Prefix <dir>] [-Base <loc>] [-Team <loc>] [-Personal <loc>]",
        '',
        "Installs lore, star and writ into <prefix> (default ~/.local), then registers each layer given",
        "with writ repo set, base first. A layer not given is skipped and named at the end, unless writ",
        "already has it registered. Never asks.",
        "The archive is verified against the release's checksums file first, and refused if it cannot be.",
        '',
        "  -Prefix <dir>     installation prefix (default: ~/.local)",
        "  -Base <loc>       the base layer: a working-tree root or a repository URL (or `$env:DEVLORE_BASE)",
        "  -Team <loc>       the team layer (or `$env:DEVLORE_TEAM)",
        "  -Personal <loc>   the personal layer (or `$env:DEVLORE_PERSONAL)",
        "  -Verbose          show what the installer is doing as it does it",
        '',
        "Environment: DEVLORE_VERSION (a release tag), DEVLORE_TOOLS (all, writ, lore or star), GH_TOKEN (optional).",
        '',
        "Served by the DevLore site's develop environment, from which devlore is released today:",
        "  & ([scriptblock]::Create((irm $InstallersUrl/install.ps1))) -Base <loc> -Team <loc> -Personal <loc>"
    )
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
# Sent as "Authorization: Bearer", the form GitHub's documentation uses and the one every token accepts ("Authenticating
# to the REST API": a JSON web token must use Bearer), and the form self upgrade sends.
$AuthToken = $env:GH_TOKEN

# -------------------------------------------------------------------
# Helpers
# -------------------------------------------------------------------

# Each stream has one use, as the PowerShell style guide's section 7 says (#1044). What the installer is doing goes to
# Write-Verbose; a warning to Write-Warning; a result, the usage, the summary and the next steps, to the output stream,
# as text, since a person reads it; a failure is an error record. Nothing goes to the information stream or the host,
# and no stream is turned on by force: the caller decides what is shown.
#
# Write-Host went to none of them. It cannot be captured or redirected, so a user whose install failed could not pipe
# the run to a file or paste a log -- which for an installer people run under `irm | iex`, and in CI, is exactly when a
# log matters.
#
# The color is not replaced. $PSStyle is PowerShell 7.2 and later, and this script must run on Windows PowerShell 5.1,
# which is what a fresh Windows machine has (#948). The streams carry the distinction the color used to, and each labels
# its own output.

# A refusal: an error record of the installer's own, with an id and a category, thrown inside Install-DevLore's caught
# work, so its catch reports it as it reports any other error. A thrown string reaches the caller as "Exception:", its
# message standing in for an id (the style guide's section 7). New-Object, not ::new: Windows PowerShell 5.1 runs this.
function ConvertTo-InstallerError {
    [CmdletBinding()]
    [OutputType([System.Management.Automation.ErrorRecord])]
    param(
        [Parameter(Mandatory)]
        [string]
        $Message,

        [Parameter(Mandatory)]
        [string]
        $Id,

        [Parameter(Mandatory)]
        [System.Management.Automation.ErrorCategory]
        $Category
    )

    $exception = New-Object -TypeName System.InvalidOperationException -ArgumentList $Message

    return New-Object -TypeName System.Management.Automation.ErrorRecord -ArgumentList $exception, $Id, $Category, $null
}

# Detect OS. install.ps1 installs on Windows, and install.sh on Linux and macOS; neither serves the other's (#1032).
#
# Windows PowerShell 5.1 has no $IsWindows, and under Set-StrictMode a variable that does not exist is an error, so the
# edition is read first: Desktop is 5.1, which runs on Windows alone. $IsWindows is consulted only on Core, where it
# exists.
function Get-OSName {
    [CmdletBinding()]
    [OutputType([string])]
    param()

    if ($PSVersionTable.PSEdition -ne 'Core' -or $IsWindows) {
        return "windows"
    }

    throw (ConvertTo-InstallerError -Id RequiresWindows -Category InvalidOperation `
        -Message "This script requires Windows.")
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
        default {
            throw (ConvertTo-InstallerError -Id UnsupportedArchitecture -Category NotImplemented `
                -Message "Unsupported architecture: $arch")
        }
    }
}

# Build common headers for GitHub API requests, authenticated when GH_TOKEN is set
function Get-ApiHeader {
    [CmdletBinding()]
    [OutputType([hashtable])]
    param([string]$Accept = "application/vnd.github+json")
    $headers = @{ Accept = $Accept }
    if ($AuthToken) {
        $headers["Authorization"] = "Bearer $AuthToken"
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

# The value of header Name in the response GitHub refused a request with, or '' when it sent none. Windows PowerShell
# 5.1 attaches an HttpWebResponse, whose Headers is a WebHeaderCollection; PowerShell 7 an HttpResponseMessage, whose
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

        throw (ConvertTo-InstallerError -Id GitHubRefused -Category ConnectionError `
            -Message "Could not ${What}: $answer")
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

        throw (ConvertTo-InstallerError -Id DownloadRefused -Category ConnectionError `
            -Message "Could not download $Name from release ${Tag}: $(Get-GitHubAnswer -ErrorRecord $failure)")
    }

    $served = Get-ServedName -Response $response

    if ($served -and -not [string]::Equals($served, $Name, [System.StringComparison]::Ordinal)) {
        throw (ConvertTo-InstallerError -Id ServedOtherFile -Category SecurityError `
            -Message "Could not download $Name from release ${Tag}: GitHub served $served, a file by another name")
    }
}

# Run a program outside PowerShell's streams, its stderr narration going to the verbose stream.
#
# PowerShell wraps each line a program writes to stderr in an error record whenever it carries the stream, which it
# does whenever a caller has redirected the install (`*>&1 | Tee-Object`, `2>&1`); Windows PowerShell 5.1 then applies
# this script's 'Stop' to the record, so the first line of lore's, star's or writ's narration ended the install (found
# by the installers' CI, #950). So no program this script runs has its streams carried by PowerShell (#1029): the
# operating system redirects them, .NET's Process class reads them, and each line of stderr goes to Write-Verbose as the
# program prints it, beside this script's own narration. stdout is returned as text, a string a line. Its standard input
# is redirected and closed, so a program that reads it ends rather than waits on a console nobody is typing into (the
# style guide's section 7).
# $LASTEXITCODE is the program's exit status, which every caller checks. The program runs in this session's working
# directory, so a relative path it is given resolves where it was typed.
function Invoke-NativeCommand {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [string]
        $FilePath,

        [Parameter(Mandatory)]
        [string[]]
        $ArgumentList
    )

    $program = Get-Command -Name $FilePath -CommandType Application -ErrorAction Stop | Select-Object -First 1
    $utf8 = New-Object -TypeName System.Text.UTF8Encoding -ArgumentList $false

    $startInfo = New-Object -TypeName System.Diagnostics.ProcessStartInfo
    $startInfo.FileName = $program.Path
    $startInfo.WorkingDirectory = (Get-Location -PSProvider FileSystem).ProviderPath
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardInput = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $startInfo.StandardOutputEncoding = $utf8
    $startInfo.StandardErrorEncoding = $utf8

    # PowerShell 7's .NET passes each argument as it is; Windows PowerShell 5.1's takes one command line, quoted as
    # Windows splits it.
    if ($startInfo.PSObject.Properties['ArgumentList']) {
        foreach ($argument in $ArgumentList) {
            $startInfo.ArgumentList.Add($argument)
        }
    }
    else {
        $startInfo.Arguments = ($ArgumentList | ForEach-Object { ConvertTo-CommandLineArgument -Argument $_ }) -join ' '
    }

    # No byte-order mark reaches the program (#1104; ruled 2026-10-20, "We do not store boms. Ever!"). Windows
    # PowerShell 5.1's .NET Framework builds a redirected standard input's writer from the console's input encoding and
    # writes that encoding's preamble the moment the process starts, so under a UTF-8 console the program reads EF BB BF
    # before the end of its input; its ProcessStartInfo has no StandardInputEncoding to choose another. So for that
    # moment the input encoding is UTF-8 without a preamble, and then the caller's again. The code page is 65001
    # throughout: section 7's rule that a script never changes the console's code page holds. Without a console,
    # reading the encoding fails, and nothing is set.
    $callerInputEncoding = $null

    try {
        $callerInputEncoding = [Console]::InputEncoding
    }
    catch {
        $callerInputEncoding = $null
    }

    $setInputEncoding = $null -ne $callerInputEncoding -and $callerInputEncoding.CodePage -eq 65001 -and
        $callerInputEncoding.GetPreamble().Length -gt 0

    try {
        if ($setInputEncoding) {
            [Console]::InputEncoding = New-Object -TypeName System.Text.UTF8Encoding -ArgumentList $false
        }

        $process = [System.Diagnostics.Process]::Start($startInfo)
    }
    finally {
        if ($setInputEncoding) {
            [Console]::InputEncoding = $callerInputEncoding
        }
    }

    try {
        $process.StandardInput.Close()

        # stdout is drained while stderr is read, so a program that fills one pipe never waits on the other.
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $line = $process.StandardError.ReadLine()

        while ($null -ne $line) {
            Write-Verbose -Message $line
            $line = $process.StandardError.ReadLine()
        }

        $process.WaitForExit()
        Set-Variable -Name LASTEXITCODE -Value $process.ExitCode -Scope Global
        $text = $stdout.GetAwaiter().GetResult()
    }
    finally {
        $process.Dispose()
    }

    if ($text) {
        $text.TrimEnd("`r", "`n") -split "`r?`n"
    }
}

# Quote one argument for a Windows command line, as Windows splits one back into arguments: an argument with no space,
# tab or quote stands as it is; any other is quoted, its quotes escaped, and the backslashes before a quote, or before
# the closing quote, doubled.
function ConvertTo-CommandLineArgument {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [AllowEmptyString()]
        [string]
        $Argument
    )

    if ($Argument -and $Argument -notmatch '[\s"]') {
        return $Argument
    }

    $escaped = $Argument -replace '(\\*)"', '$1$1\"'
    $escaped = $escaped -replace '(\\+)$', '$1$1'

    return "`"$escaped`""
}

# Verify checksum
function Test-Checksum {
    [CmdletBinding()]
    param([string]$File, [string]$Expected)
    $actual = (Get-FileHash -Path $File -Algorithm SHA256).Hash.ToLower()
    if ($actual -ne $Expected.ToLower()) {
        throw (ConvertTo-InstallerError -Id ChecksumMismatch -Category SecurityError `
            -Message "Checksum verification failed!`nExpected: $Expected`nActual:   $actual")
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

# Get-CommandOnPersistentPath gets a program as Get-Command does, from the PATH a new terminal searches.
#
# A new terminal searches the machine and user PATH stored in the registry, not this session's. The folders are the
# machine PATH's entries, then the user PATH's, each as written. An empty entry goes, as Windows skips it; a repeat
# stays, since searching a folder again changes no answer. A name with an extension is searched for as written; a name
# without one, with each extension of the machine's PATHEXT in turn, in each folder before the next. A folder is
# searched with Get-ChildItem -LiteralPath, since -Path reads [...] in its name as a wildcard. As Get-Command does, it
# writes a non-terminating error for each name it finds nothing for.
#
# Parameters:
#   - `Name`: Each program's name, with or without its extension; taken from the pipeline too.
#   - `All`: Get every copy, in the order a new terminal finds them, not the first alone.
#
# Returns:
#   - System.Management.Automation.ApplicationInfo: Get-Command's description of each copy found.
function Get-CommandOnPersistentPath {
    [CmdletBinding()]
    [OutputType([System.Management.Automation.ApplicationInfo])]
    param(
        [Parameter(Mandatory, ValueFromPipeline)]
        [String[]]
        $Name,

        [Switch]
        $All
    )

    begin {

        $folders =
            ([Environment]::GetEnvironmentVariable('Path', [EnvironmentVariableTarget]::Machine) -split ';') +
            ([Environment]::GetEnvironmentVariable('Path', [EnvironmentVariableTarget]::User) -split ';') |
                Where-Object { $_ }

        # Each of PATHEXT's extensions, keyed without case, to its place in PATHEXT: a folder's files are taken in that
        # order. ToDictionary throws on a repeated extension.

        $pathExt = [Environment]::GetEnvironmentVariable('PATHEXT', [EnvironmentVariableTarget]::Machine)
        $ordered = [String[]] ($pathExt -split ';' | Where-Object { $_ })

        $pathExtensions = [Enumerable]::ToDictionary(
            [Enumerable]::Range(0, $ordered.Length),
            [Func[Int32, String]] { param($i) $ordered[$i] },
            [System.StringComparer]::OrdinalIgnoreCase)
    }

    process {

        foreach ($commandName in $Name) {

            if ([System.IO.Path]::HasExtension($commandName)) {
                $filter = $commandName
                $extensions = New-Object `
                    -TypeName 'System.Collections.Generic.Dictionary[String, Int32]' `
                    -ArgumentList ([System.StringComparer]::OrdinalIgnoreCase)
                $extensions[[System.IO.Path]::GetExtension($commandName)] = 0
            }
            else {
                $filter = "$commandName.*"
                $extensions = $pathExtensions
            }

            $found = $false

            foreach ($folder in $folders) {

                # A folder's files are taken in PATHEXT's order; a file whose extension PATHEXT lacks, writ.exe.config
                # for writ.*, is not a program.

                $files = Get-ChildItem -LiteralPath $folder -Filter $filter -File -ErrorAction Ignore |
                    Where-Object { $extensions.ContainsKey($_.Extension) } |
                    Sort-Object -Property { $extensions[$_.Extension] }

                foreach ($file in $files) {

                    Get-Command -Name $file.FullName -CommandType Application -ErrorAction Ignore
                    $found = $true

                    if (-not $All) {
                        break
                    }
                }

                if ($found -and -not $All) {
                    break
                }
            }

            if (-not $found) {

                $exception = New-Object `
                    -TypeName System.Management.Automation.CommandNotFoundException `
                    -ArgumentList "The term '$commandName' is not recognized as a program on the persistent PATH."

                $record = New-Object `
                    -TypeName System.Management.Automation.ErrorRecord `
                    -ArgumentList (
                        $exception,
                        'CommandNotFoundOnPersistentPath',
                        [System.Management.Automation.ErrorCategory]::ObjectNotFound,
                        $commandName)

                $PSCmdlet.WriteError($record)
            }
        }
    }
}

# Get-FileIdentity gets the identity of the file a path names: its volume's serial number and its file ID.
#
# Two paths name the same file exactly when their identities are equal, whatever links lie between: a hard link shares
# its file's ID, and opening a symbolic link or a path through a junction opens its target. The identity comes from
# GetFileInformationByHandleEx's FileIdInfo, whose 128-bit ID holds on ReFS as well as NTFS.
#
# Parameters:
#   - `LiteralPath`: The file's path, taken as written.
#
# Returns:
#   - String: The volume serial number and the file ID, in hexadecimal, as `<volume>:<file ID>`.
function Get-FileIdentity {
    [CmdletBinding()]
    [OutputType([String])]
    param(
        [Parameter(Mandatory)]
        [String]
        $LiteralPath
    )

    if (-not ('DevLore.FileIdentity' -as [Type])) {
        Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.IO;
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;

namespace DevLore
{
    public static class FileIdentity
    {
        [StructLayout(LayoutKind.Sequential)]
        private struct FileIdInfo
        {
            public ulong VolumeSerialNumber;
            [MarshalAs(UnmanagedType.ByValArray, SizeConst = 16)]
            public byte[] FileId;
        }

        // FILE_INFO_BY_HANDLE_CLASS.FileIdInfo.
        private const int FileIdInfoClass = 18;

        [DllImport("kernel32.dll", SetLastError = true)]
        private static extern bool GetFileInformationByHandleEx(
            SafeFileHandle file, int infoClass, out FileIdInfo info, uint size);

        public static string Of(string path)
        {
            using (var stream = new FileStream(path, FileMode.Open, FileAccess.Read,
                FileShare.ReadWrite | FileShare.Delete))
            {
                FileIdInfo info;

                if (!GetFileInformationByHandleEx(stream.SafeFileHandle, FileIdInfoClass, out info,
                    (uint) Marshal.SizeOf(typeof(FileIdInfo))))
                {
                    throw new Win32Exception(Marshal.GetLastWin32Error());
                }

                return info.VolumeSerialNumber.ToString("x16") + ":" +
                    BitConverter.ToString(info.FileId).Replace("-", "");
            }
        }
    }
}
'@
    }

    [DevLore.FileIdentity]::Of($LiteralPath)
}

# The installer's work, as the PowerShell style guide's section 7 has a script do it (#1044). It notes its caller's
# error action, sets Stop for its own work, and runs that work inside try and catch. The catch puts the caller's error
# action back and reports the error with $PSCmdlet.WriteError, so a caller who continues on error continues, one who
# silences errors hears nothing though $Error keeps the record, and one who stops on error stops, their own try catching
# it.
#
# The catch is this function's and not the script's (D1): under `irm | iex` the script has no $PSCmdlet, and a function
# has its own in every form. So an error names Install-DevLore, where section 7's example names the script. The error
# preference is this function's too (D4): set at script scope, it would change the user's session under `irm | iex`.
function Install-DevLore {
    [CmdletBinding()]
    param()

    $callerErrorAction = $ErrorActionPreference
    $ErrorActionPreference = 'Stop'

    try {
        Write-Verbose -Message "DevLore CLI Installer"

        # A layer is registered by the writ this run installs, so a run that leaves writ out cannot register one.
        # Refused before anything is downloaded (#950).
        if (($Base -or $Team -or $Personal) -and $Tools -ne "all" -and $Tools -ne "writ") {
            throw (ConvertTo-InstallerError -Id LayersNeedWrit -Category InvalidArgument `
                -Message "-Base, -Team and -Personal register layers with writ, which DEVLORE_TOOLS=$Tools does not install")
        }

        # GitHub speaks TLS 1.2 and later. Windows PowerShell 5.1 takes its protocols from the .NET Framework's
        # default, which on an older machine stops at 1.0, so every request fails before it starts. Adding 1.2
        # is a no-op wherever the default already includes it, PowerShell 7 included.
        [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor [System.Net.SecurityProtocolType]::Tls12

        # Detect platform
        $os = Get-OSName
        $arch = Get-ArchName
        Write-Verbose -Message "Detected platform: $os/$arch"

        # Default prefix: ~/.local on every platform. XDG conventions hold on Windows too (ruled 2026-09-22, #903), and
        # star's extension loader looks under ~/.local/share, so a prefix anywhere else installs extensions nothing
        # loads.
        if (-not $Prefix) {
            $Prefix = Join-Path $HOME ".local"
        }
        $installDir = Join-Path $Prefix "bin"

        # Resolve version. $releaseFound says whether GitHub has shown that release $Version exists, by listing it as
        # the newest or by serving one of its files. A release DEVLORE_VERSION names is not looked up before its files
        # are downloaded: GitHub's API is asked for it only when a link answers 404 before then (#1008).
        $releaseFound = $false

        if ($Version -eq "latest") {
            Write-Verbose -Message "Fetching latest version..."
            $Version = Get-LatestVersion

            if (-not $Version) {
                throw (ConvertTo-InstallerError -Id NoRelease -Category ObjectNotFound `
                    -Message "Could not determine the latest release of ${GitHubRepo}: GitHub lists none")
            }

            $releaseFound = $true
        }

        Write-Verbose -Message "Version: $Version"

        # Build asset names
        $archiveName = "devlore-cli_${Version}_${os}_${arch}.zip"
        $checksumsName = "devlore-cli_${Version}_checksums.txt"

        # Create temp directory
        $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "devlore-install-$([System.Guid]::NewGuid().ToString('N'))"
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

        try {
            # Download the checksums file, then the archive, each by its public link (#1008). An archive the release
            # gives no way to verify is not installed (#1002): as self upgrade does, the archive is downloaded only when
            # the checksums file has its line.
            Write-Verbose -Message "Downloading $checksumsName..."
            $checksumsPath = Join-Path $tmpDir $checksumsName
            Save-ReleaseFile -Name $checksumsName -Tag $Version -Destination $checksumsPath -ReleaseFound:$releaseFound
            $releaseFound = $true
            $expectedChecksum = Get-ListedChecksum -Path $checksumsPath -Name $archiveName

            if (-not $expectedChecksum) {
                throw (ConvertTo-InstallerError -Id NoChecksum -Category SecurityError `
                    -Message "$checksumsName has no line for $archiveName, so the archive cannot be verified")
            }

            Write-Verbose -Message "Downloading $archiveName..."
            $archivePath = Join-Path $tmpDir $archiveName
            Save-ReleaseFile -Name $archiveName -Tag $Version -Destination $archivePath -ReleaseFound:$releaseFound

            # Verify the archive before anything is extracted
            Write-Verbose -Message "Verifying checksum..."
            Test-Checksum -File $archivePath -Expected $expectedChecksum
            Write-Verbose -Message "Checksum verified"

            # Extract archive
            #
            # The archive holds the products at its root and star's extensions under share/ (#903). The products move
            # to pkg/bin so that each one's `self install` finds pkg/share at <exeDir>/../share, the path star copies
            # its extensions from.
            Write-Verbose -Message "Extracting..."
            $pkg = Join-Path $tmpDir "pkg"
            $pkgBin = Join-Path $pkg "bin"
            New-Item -ItemType Directory -Path $pkgBin -Force | Out-Null
            Expand-Archive -Path $archivePath -DestinationPath $pkg -Force

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

                Write-Verbose -Message "Installing $product..."
                Push-Location $pkg
                try {
                    $null = Invoke-NativeCommand -FilePath $toolPath -ArgumentList @('self', 'install', $Prefix, '--unattended')
                    if ($LASTEXITCODE -ne 0) {
                        throw (ConvertTo-InstallerError -Id SelfInstallFailed -Category NotInstalled `
                            -Message "$product self install failed")
                    }
                } finally {
                    Pop-Location
                }
                $installed += $product
            }

            if ($installed.Count -eq 0) {
                throw (ConvertTo-InstallerError -Id NoPrograms -Category ObjectNotFound `
                    -Message "No binaries found in archive for DEVLORE_TOOLS=$Tools")
            }

            # Register the layers given, base first, with the writ just installed (#950). The call runs in the user's
            # working directory, so a relative location resolves where it was typed. writ's output is its own, and so
            # are its errors: a failure ends the run, and running the same command again is the recovery. --unattended
            # is writ's contract for a run nobody is there to answer.
            $writPath = Join-Path $installDir "writ.exe"
            $layers = [ordered]@{ base = $Base; team = $Team; personal = $Personal }
            $registered = @()
            $held = @()
            $skipped = @()

            foreach ($layer in $layers.Keys) {
                $location = $layers[$layer]
                if (-not $location) {
                    # A layer not given is skipped unless the writ just installed already has it registered (#1029). A
                    # writ this run didn't install, or that can't answer, leaves it skipped. --filter, not a --jq
                    # select: Windows PowerShell 5.1 strips the double quotes inside a native command's arguments.
                    $root = ''
                    if (Test-Path -LiteralPath $writPath -PathType Leaf) {
                        try {
                            $root = (Invoke-NativeCommand -FilePath $writPath -ArgumentList @('repo', 'list',
                                    '--filter', "layer=$layer", '--filter', 'state=registered', '--jq', '.[].root',
                                    '--output', 'value') | Out-String).Trim()
                            if ($LASTEXITCODE -ne 0) {
                                $root = ''
                            }
                        } catch {
                            $root = ''
                        }
                    }
                    if ($root) {
                        $held += $layer
                    } else {
                        $skipped += $layer
                    }
                    continue
                }

                Write-Verbose -Message "Registering ${layer}: $location"
                $null = Invoke-NativeCommand -FilePath $writPath -ArgumentList @('repo', 'set', $layer, $location, '--unattended')
                if ($LASTEXITCODE -ne 0) {
                    throw (ConvertTo-InstallerError -Id LayerNotRegistered -Category NotInstalled `
                        -Message "writ repo set $layer exited $LASTEXITCODE")
                }
                $registered += $layer
            }

            # The results: the summary, then the PATH advice and the next steps, on the output stream; its last
            # lines are the layers skipped.
            Write-Output -InputObject ''
            Write-Output -InputObject "Installed: $($installed -join ' ')"
            Write-Output -InputObject "Location: $installDir"
            if ($registered.Count -gt 0) {
                Write-Output -InputObject "Registered: $($registered -join ' ')"
            }
            if ($held.Count -gt 0) {
                Write-Output -InputObject "Already registered: $($held -join ' ')"
            }
            Write-Output -InputObject ''

            # Whether a new terminal would run what was installed (#1050). A new terminal searches the PATH stored in
            # the registry, which this session's need not match, so the copies on it are asked for, in the order it
            # finds them, and each is compared with the installed file by identity, not by name: a link reaches the
            # same file by another path. writ is asked about, or the first program installed when writ was not.
            $program = $installed[0]

            if ($installed -contains 'writ') {
                $program = 'writ'
            }

            $programPath = Join-Path -Path $installDir -ChildPath "$program.exe"
            $programIdentity = Get-FileIdentity -LiteralPath $programPath
            $copies = @(Get-CommandOnPersistentPath -Name $program -All -ErrorAction Ignore)
            $oursAt = -1

            for ($i = 0; $i -lt $copies.Count -and $oursAt -lt 0; $i++) {

                # A copy that cannot be read is not the installed file, which this run just read.
                try {
                    if ((Get-FileIdentity -LiteralPath $copies[$i].Source) -eq $programIdentity) {
                        $oursAt = $i
                    }
                }
                catch {
                    Write-Verbose -Message "Could not read $($copies[$i].Source): $($_.Exception.Message)"
                }
            }

            if ($oursAt -eq 0) {
                Write-Output -InputObject "Verify installation:"

                foreach ($tool in $installed) {
                    Write-Output -InputObject "  $tool --version"
                }
            }
            else {
                $userAdvice = $true

                if ($copies.Count -eq 0) {
                    Write-Warning -Message "$installDir is not on your PATH"
                }
                else {
                    $first = $copies[0].Source

                    if ($oursAt -gt 0) {
                        Write-Warning -Message "$first comes before $programPath on your PATH, so $program runs it"
                    }
                    else {
                        Write-Warning -Message "$installDir is not on your PATH, and $first is, so $program runs it"
                    }

                    # A copy in a machine PATH folder comes before every user PATH folder, so no change to the user
                    # PATH can put the install directory ahead of it.
                    $firstFolder = (Split-Path -Path $first -Parent).TrimEnd('\')
                    $machinePath = [Environment]::GetEnvironmentVariable('Path', [EnvironmentVariableTarget]::Machine)
                    $machineFolders = $machinePath -split ';' |
                        Where-Object { $_ } |
                        ForEach-Object { $_.TrimEnd('\') }

                    if ($machineFolders -contains $firstFolder) {
                        $userAdvice = $false
                        Write-Output -InputObject ''
                        Write-Output -InputObject `
                            "$first is on the machine PATH, which a new terminal searches before yours."
                        Write-Output -InputObject `
                            "Remove it, or put $installDir first in the machine PATH, as an administrator."
                        Write-Output -InputObject ''
                    }
                }

                if ($userAdvice) {
                    Write-Output -InputObject ''
                    Write-Output -InputObject "Add it to your PATH:"
                    Write-Output -InputObject ''
                    Write-Output -InputObject "  [Environment]::SetEnvironmentVariable('Path',"
                    Write-Output -InputObject `
                        "    `"$installDir;`" + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')"
                    Write-Output -InputObject ''
                    Write-Output -InputObject "then open a new terminal: this one keeps the PATH it started with."
                    Write-Output -InputObject ''
                    Write-Output -InputObject "Or, for PowerShell alone, add to your profile (`$PROFILE):"
                    Write-Output -InputObject ''
                    Write-Output -InputObject "  `$env:PATH = `"$installDir;`$env:PATH`""
                    Write-Output -InputObject ''
                }
            }

            Write-Output -InputObject ''
            Write-Output -InputObject "Documentation: https://github.com/NobleFactor/devlore-cli#readme"
            Write-Output -InputObject ''
            Write-Output -InputObject "Next steps:"
            if ($registered.Count -gt 0) {
                Write-Output -InputObject "  writ deploy"
            }
            foreach ($layer in $skipped) {
                Write-Output -InputObject "skipped: $layer; to register it later:"
                Write-Output -InputObject "  writ repo set $layer <working-tree-root>|<repository-url>"
            }

        } finally {
            # Clean up temp directory
            Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
    catch {
        $caught = $_
        $ErrorActionPreference = $callerErrorAction
        $PSCmdlet.WriteError($caught)
    }
}

# After a failure the script returns, so the session it runs in goes on: under `irm | iex` and the script-block form,
# `exit` would end the user's PowerShell session (#965; ruled 2026-10-19, "return, per #965"). Run as a file there is no
# session to end, and the script exits 1, as section 7 says, so the process that ran it knows (D2). $? is false after a
# reported error whatever the caller's error action; under Stop the error never gets here.
Install-DevLore

if (-not $? -and $MyInvocation.MyCommand.CommandType -eq 'ExternalScript') {
    exit 1
}
