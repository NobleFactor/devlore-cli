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

    Every install runs against a faux channel (#1031): a fixture release instead of GitHub, whose archive for this
    platform is this checkout's lore, star and writ, so the installer under test installs the programs under test. The
    cases of #1002's one list, which install.sh's test runs too, run against it, each check named by its number in the
    plan (docs/plans/fix/1002-installers-install-an-archive.md). Functions named
    Invoke-WebRequest and Invoke-RestMethod, defined where the installer runs, stand in for GitHub and answer from
    files made here, as install.sh's test's fake curl does. Invoke-WebRequest answers the release's public links,
    https://github.com/NobleFactor/devlore-cli/releases/download/<tag>/<name>, as GitHub's download host does: it takes
    the name in any case, names the file it served in its Content-Disposition, and answers a name it has no file for
    with 404 and "Not Found" in plain text. Invoke-RestMethod answers GitHub's API, for the newest release and for a
    release by its tag, and anything else with GitHub's 404 and its JSON. A function takes precedence over a cmdlet of
    the same name, so the installer is tested as it ships, with no hook that changes where it downloads from. A request
    GitHub refuses is answered as this edition's cmdlet answers it, with GitHub's status, content type and body in the
    error record. A refusal is checked for its error record, the installer's own, by its id and for install.sh's
    sentence, exactly, from the same tags, and every case for each request the installer made of GitHub, in order, and
    for those it sent GH_TOKEN with. The installer runs with -Verbose, every stream captured, so its narration is read
    from the verbose stream and its results from the output stream (#1044). A refusal runs in this session, which goes
    on; run as a file, a failure exits 1, which 'as a file: a failure exits 1' checks.

    Three checks run against GitHub itself, so that what the stand-ins model is what this edition's cmdlets do: a
    release GitHub does not have, the name GitHub's download host says it served, and a file a release lacks.

    Writes PASS or FAIL for each check to the output stream, with what the installer printed under each failure, and
    exits 1 if any check failed. What it is doing goes to the verbose stream; a failure of its own is an error record
    and exits 1, as the PowerShell style guide's section 7 says (#1044). Run by .github/workflows/installers.yaml on
    Windows, under both editions (#950, #965).

    Environment: $env:GH_TOKEN, optional, is passed to the installer, which sends it to GitHub's API.
    $env:DEVLORE_TEST_DIST is a directory holding what `make dist DEVLORE_VERSION=v0.0.0-test.1002` made: this
    platform's archive. On Windows it is required, since Windows builds no archive; elsewhere, without it, the suite
    runs that for this platform itself, which needs Go and GNU make 3.82 or later.

.EXAMPLE
    powershell.exe -NoProfile -File scripts/Test-InstallScript.ps1

.EXAMPLE
    pwsh -NoProfile -File scripts/Test-InstallScript.ps1
#>

#Requires -Version 5.1

[CmdletBinding()]
param()

Set-StrictMode -Version Latest

# The caller's error action, put back by the main block's catch before it reports an error of the suite's own (the
# style guide's section 7).
$callerErrorAction = $ErrorActionPreference
$ErrorActionPreference = 'Stop'

# The checkout this script belongs to, whose install.ps1 is under test, and which also serves as a personal layer:
# a working-tree root.
$repo = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$installer = Join-Path $repo 'install.ps1'
$teamUrl = 'https://github.com/NobleFactor/noblefactor-ops.git'

# Windows PowerShell 5.1 has no $IsWindows, and under Set-StrictMode a variable that does not exist is an error, so
# the edition is read first: Desktop is 5.1, which runs on Windows alone.
$onWindows = ($PSVersionTable.PSEdition -ne 'Core') -or $IsWindows
$writName = 'writ'

if ($onWindows) {
    $writName = 'writ.exe'
}

# This process's own executable, so a child session is the same edition as this one.
$shell = [System.Diagnostics.Process]::GetCurrentProcess().MainModule.FileName

$scratch = Join-Path ([System.IO.Path]::GetTempPath()) "devlore-install-test-$([System.Guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Path $scratch -Force | Out-Null

# The fixture release the checksum cases run against (#1002): its tag, and the names the installer looks for on this
# platform, worked out as the installer works them out. The tag is install.sh's test's, so where the two installers
# word a refusal alike, the two tests check the same sentence.
$fixtureTag = 'v0.0.0-test.1002'
$fixtureApi = 'https://api.github.com/repos/NobleFactor/devlore-cli'
$fixtureDownload = 'https://github.com/NobleFactor/devlore-cli/releases/download'
$fixtureDir = Join-Path $scratch 'fixture'
$fixtureOs = 'linux'
$fixtureExt = 'tar.gz'

if ($onWindows) {
    $fixtureOs = 'windows'
    $fixtureExt = 'zip'
}
elseif ($IsMacOS) {
    $fixtureOs = 'darwin'
}

$fixtureArch = "$([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture)".ToLowerInvariant()

if ($fixtureArch -eq 'x64') {
    $fixtureArch = 'amd64'
}

$fixtureArchiveName = "devlore-cli_${fixtureTag}_${fixtureOs}_${fixtureArch}.$fixtureExt"
$fixtureChecksumsName = "devlore-cli_${fixtureTag}_checksums.txt"

# The requests a case may make of GitHub: each file by its public link, and of GitHub's API, the newest release and the
# release by its tag.
$fixtureChecksumsLink = "$fixtureDownload/$fixtureTag/$fixtureChecksumsName"
$fixtureArchiveLink = "$fixtureDownload/$fixtureTag/$fixtureArchiveName"
$fixtureNewest = "$fixtureApi/releases?per_page=1"
$fixtureByTag = "$fixtureApi/releases/tags/$fixtureTag"

# Every archive a release publishes, so that a release without this platform's still has the others.
$fixturePlatforms = @(
    'darwin_amd64.tar.gz', 'darwin_arm64.tar.gz', 'linux_amd64.tar.gz', 'linux_arm64.tar.gz', 'windows_amd64.zip',
    'windows_arm64.zip'
)

# GitHub's message when an address's anonymous requests are over its limit, as GitHub words it.
$fixtureRateLimit = 'API rate limit exceeded for 203.0.113.7. (But here''s the good news: Authenticated requests ' +
    'get a higher rate limit. Check out the documentation for more details.)'

# A tag no release has, for the release DEVLORE_VERSION names when GitHub does not have it: install.sh's test's too.
$fixtureMissingTag = 'v0.0.0-test.no-such-release'

# The response a refused request carries on Windows PowerShell 5.1, where the web cmdlets throw a WebException holding
# an HttpWebResponse. An HttpWebResponse has no public constructor, so the stand-ins hold this instead: a WebResponse
# carrying the status, the content type and the headers under the names HttpWebResponse gives them, StatusCode,
# ContentType and Headers. C# 5, which Windows PowerShell's compiler takes.
$fixtureResponseSource = @'
public class DevLoreFixtureResponse : System.Net.WebResponse
{
    private readonly System.Net.HttpStatusCode statusCode;
    private readonly string contentType;
    private readonly System.Net.WebHeaderCollection headers;

    public DevLoreFixtureResponse(System.Net.HttpStatusCode statusCode, string contentType,
        System.Net.WebHeaderCollection headers)
    {
        this.statusCode = statusCode;
        this.contentType = contentType;
        this.headers = headers;
    }

    public System.Net.HttpStatusCode StatusCode
    {
        get { return this.statusCode; }
    }

    public override string ContentType
    {
        get { return this.contentType; }
        set { }
    }

    public override System.Net.WebHeaderCollection Headers
    {
        get { return this.headers; }
    }
}
'@

$script:failures = 0

# Every fixture run, by its account and in order, with what it left in the account's temporary directory (#1051).
$script:fixtureRuns = New-Object -TypeName 'System.Collections.Generic.List[Object]'

###########
# Helper functions
###########

# Write-Pass writes a passed check's line, "PASS <description>".
#
# Parameters:
#   - `Description`: What the check checked.
#
# Returns:
#   - String: The line.
function Write-Pass {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Description
    )

    Write-Output -InputObject "PASS $Description"
}

# Write-Fail writes a failed check's line, "FAIL <description>", and its detail, and counts the failure.
#
# Parameters:
#   - `Description`: What the check checked.
#   - `Detail`: What the check saw, written between two lines of dashes when given.
#
# Returns:
#   - String: The line, then the detail's lines.
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
    Write-Output -InputObject "FAIL $Description"

    if ($Detail) {
        Write-Output -InputObject '----'
        Write-Output -InputObject $Detail
        Write-Output -InputObject '----'
    }
}

# ConvertTo-SuiteError makes an error record of the suite's own, for a refusal.
#
# The record carries an id and a category, so the main block's catch reports it with the suite's name and its id, not as
# a thrown string (the style guide's section 7).
#
# Parameters:
#   - `Message`: What was refused, and why.
#   - `Id`: The record's error id.
#   - `Category`: The record's error category.
#
# Returns:
#   - System.Management.Automation.ErrorRecord: The record, whose exception is an InvalidOperationException.
function ConvertTo-SuiteError {
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

# Test-Expectation writes a check's result: PASS when its condition holds, and FAIL with its detail when not.
#
# Parameters:
#   - `Description`: What the check checks.
#   - `Condition`: Whether the check holds.
#   - `Detail`: What to show when it does not.
#
# Returns:
#   - String: The check's lines, as Write-Pass and Write-Fail write them.
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
    }
    else {
        Write-Fail -Description $Description -Detail $Detail
    }
}

# Use-Account points this process's XDG homes and temporary directory at a scratch account.
#
# The installer, and the writ it runs, inherit them.
#
# Parameters:
#   - `Name`: The account's directory name under the scratch directory.
#
# Returns:
#   - String: The account's prefix, where an install into it goes.
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

# Get-LayerRoot gets the root a writ reports for a layer, or '' when it reports none.
#
# Parameters:
#   - `Writ`: The writ to ask.
#   - `Layer`: The layer's name.
#
# Returns:
#   - String: The layer's root, or ''.
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

# Get-Registration gets every registration a writ reports, as one string to compare runs by.
#
# Parameters:
#   - `Writ`: The writ to ask.
#
# Returns:
#   - String: `writ repo list`'s output, or '' when the writ is not there.
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

# Get-Leftover gets the download directories the installer left in an account's temporary directory.
#
# Use-Account points TMP, TEMP and TMPDIR there, so it is where install.ps1 makes its download directory,
# devlore-install-<guid>, which it must remove whichever way it ends (#1051). Nothing else there is counted: under
# Windows PowerShell 5.1, Add-Type's compiler leaves an empty directory with a random name in TEMP, its own and not the
# installer's, and PowerShell 7 leaves none (measured on danoble-wd11-3, 2026-10-21).
#
# Parameters:
#   - `Account`: The account's name.
#
# Returns:
#   - String: The name of each download directory left.
function Get-Leftover {
    [CmdletBinding()]
    [OutputType([String])]
    param(
        [Parameter(Mandatory)]
        [String]
        $Account
    )

    $tmp = Join-Path (Join-Path $scratch $Account) 'tmp'
    Get-ChildItem -LiteralPath $tmp -Filter 'devlore-install-*' -Force -ErrorAction Ignore | ForEach-Object { $_.Name }
}

# Invoke-Child runs this edition of PowerShell as a separate process with the arguments given.
#
# Its output streams are redirected by the operating system into files, so no stream becomes an error record in this
# session, and its standard input is read from -InputFile when one is given.
#
# Parameters:
#   - `Name`: The run's name, which names its files under the scratch directory.
#   - `ArgumentList`: The arguments PowerShell is given.
#   - `InputFile`: A file to read standard input from.
#
# Returns:
#   - PSCustomObject: `ExitCode`, and `Output`, everything the child printed.
function Invoke-Child {
    [CmdletBinding()]
    [OutputType([pscustomobject])]
    param(
        [Parameter(Mandatory)]
        [string]
        $Name,

        [Parameter(Mandatory)]
        [string[]]
        $ArgumentList,

        [string]
        $InputFile
    )

    $out = Join-Path $scratch "$Name.out"
    $err = Join-Path $scratch "$Name.err"
    $redirect = @{ RedirectStandardOutput = $out; RedirectStandardError = $err }

    if ($InputFile) {
        $redirect['RedirectStandardInput'] = $InputFile
    }

    $process = Start-Process -FilePath $shell -ArgumentList $ArgumentList -Wait -PassThru -NoNewWindow @redirect
    return [pscustomobject]@{
        ExitCode = $process.ExitCode
        Output   = "$(Get-Content -Raw -LiteralPath $out)$(Get-Content -Raw -LiteralPath $err)"
    }
}

# ConvertTo-EncodedCommand encodes a command for -EncodedCommand, which takes it as base64 UTF-16 and needs no quoting.
#
# Parameters:
#   - `Command`: The command.
#
# Returns:
#   - String: The command, encoded.
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

# Initialize-FixtureArchive gets the faux channel's archive for this platform (#1031), building it when it must.
#
# The archive holds this checkout's lore, star and writ, as `make dist` builds and packs them for a release, named for
# the fixtures' tag, so the installer under test installs the programs under test. In CI, one job builds them for every
# platform, off Windows, and $env:DEVLORE_TEST_DIST names them. Without it, off Windows, this suite builds this
# platform's; on Windows it refuses, since Windows builds nothing and needs neither tar nor zip.
#
# Returns:
#   - String: The archive's path.
function Initialize-FixtureArchive {
    [CmdletBinding()]
    [OutputType([string])]
    param()

    $dist = $env:DEVLORE_TEST_DIST

    if (-not $dist) {
        if ($onWindows) {
            throw (ConvertTo-SuiteError -Id NoTestDist -Category ObjectNotFound `
                -Message ('set $env:DEVLORE_TEST_DIST to a directory holding what `make dist DEVLORE_VERSION=' +
                    "$fixtureTag`` made, off Windows: Windows builds no archive"))
        }

        $platform = "$fixtureOs/$fixtureArch"
        Write-Verbose -Message "Building the faux channel: make dist PLATFORM=$platform DEVLORE_VERSION=$fixtureTag"
        $log = Join-Path $scratch 'dist.log'

        # Continue, not Stop: PowerShell makes each line a native command writes to stderr an error record when it
        # carries the stream, as it does here. The exit status is what says whether it built.
        $ErrorActionPreference = 'Continue'
        & make -C $repo dist "PLATFORM=$platform" "DEVLORE_VERSION=$fixtureTag" *> $log

        if ($LASTEXITCODE -ne 0) {
            throw (ConvertTo-SuiteError -Id DistFailed -Category NotInstalled `
                -Message "make dist exited ${LASTEXITCODE}:`n$(Get-Content -Raw -LiteralPath $log)")
        }

        $dist = Join-Path $repo 'dist'
    }

    $archive = Join-Path $dist $fixtureArchiveName

    if (-not (Test-Path -LiteralPath $archive -PathType Leaf)) {
        throw (ConvertTo-SuiteError -Id NoFixtureArchive -Category ObjectNotFound `
            -Message ("$dist has no ${fixtureArchiveName}: it holds no make dist DEVLORE_VERSION=$fixtureTag for " +
                'this platform'))
    }

    return $archive
}

# Get-GitHubRefusal makes the error record this edition's web cmdlets throw when GitHub refuses a request.
#
# The record carries the status, the content type, the headers and the body where the cmdlets carry them. GitHub's API
# refuses with its JSON, the message under "message" and its documentation_url; GitHub's download host refuses with the
# message alone, in plain text. PowerShell 7 throws an HttpResponseException holding the response, with the body in
# ErrorDetails, JSON re-indented after a newline. Windows PowerShell 5.1 throws a WebException holding the response,
# with the body as sent; the response here is a DevLoreFixtureResponse, which carries the status and the content type as
# an HttpWebResponse does. So the installer reads GitHub's status and message from these stand-ins on both editions, and
# the same sentence is checked on both.
#
# Parameters:
#   - `Status`: The HTTP status GitHub refuses with.
#   - `Message`: GitHub's message.
#   - `Documentation`: The API's documentation_url; without it, the refusal is the download host's, in plain text.
#   - `Header`: Any more headers GitHub sends, as its API sends x-ratelimit-* with a rate limit (default: none).
#   - `Command`: The cmdlet that throws it, which the record names.
#
# Returns:
#   - System.Management.Automation.ErrorRecord: The record.
function Get-GitHubRefusal {
    [CmdletBinding()]
    [OutputType([System.Management.Automation.ErrorRecord])]
    param(
        [Parameter(Mandatory)]
        [System.Net.HttpStatusCode]
        $Status,

        [Parameter(Mandatory)]
        [string]
        $Message,

        [string]
        $Documentation,

        [System.Collections.IDictionary]
        $Header = @{},

        [Parameter(Mandatory)]
        [string]
        $Command
    )

    $mediaType = 'text/plain'
    $body = $Message

    if ($Documentation) {
        $mediaType = 'application/json'
        $body = [ordered]@{ message = $Message; documentation_url = $Documentation; status = "$([int]$Status)" } |
            ConvertTo-Json
    }

    $details = $body

    if ($PSVersionTable.PSEdition -eq 'Core') {
        $response = New-Object -TypeName System.Net.Http.HttpResponseMessage -ArgumentList $Status
        $response.Content = New-Object -TypeName System.Net.Http.StringContent -ArgumentList @(
            $body, [System.Text.Encoding]::UTF8, $mediaType)

        foreach ($name in $Header.Keys) {
            $null = $response.Headers.TryAddWithoutValidation($name, [string]$Header[$name])
        }

        $exception = New-Object -TypeName Microsoft.PowerShell.Commands.HttpResponseException -ArgumentList @(
            "Response status code does not indicate success: $([int]$Status) ($($response.ReasonPhrase)).", $response)

        if ($Documentation) {
            $details = "`n$body"
        }
    }
    else {
        if (-not ('DevLoreFixtureResponse' -as [type])) {
            Add-Type -TypeDefinition $fixtureResponseSource
        }

        $reason = "$Status" -creplace '(?<=[a-z])(?=[A-Z])', ' '
        $headers = New-Object -TypeName System.Net.WebHeaderCollection

        foreach ($name in $Header.Keys) {
            $headers[$name] = [string]$Header[$name]
        }

        $response = New-Object -TypeName DevLoreFixtureResponse -ArgumentList @(
            $Status, "$mediaType; charset=utf-8", $headers)
        $exception = New-Object -TypeName System.Net.WebException -ArgumentList @(
            "The remote server returned an error: ($([int]$Status)) $reason.", $null,
            [System.Net.WebExceptionStatus]::ProtocolError, $response)
    }

    $record = New-Object -TypeName System.Management.Automation.ErrorRecord -ArgumentList @(
        $exception, "WebCmdletWebResponseException,Microsoft.PowerShell.Commands.$Command",
        [System.Management.Automation.ErrorCategory]::InvalidOperation, $null)
    $record.ErrorDetails = New-Object -TypeName System.Management.Automation.ErrorDetails -ArgumentList $details
    return $record
}

# Get-ClockTime gets a time as this machine's clock shows it, HH:mm, whatever the culture.
#
# Parameters:
#   - `Seconds`: The time, in seconds since the epoch.
#
# Returns:
#   - String: The time, HH:mm.
function Get-ClockTime {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [long]
        $Seconds
    )

    $local = [System.DateTimeOffset]::FromUnixTimeSeconds($Seconds).ToLocalTime()
    return $local.ToString('HH:mm', [System.Globalization.CultureInfo]::InvariantCulture)
}

# Get-LookAlikeName gets a name a pattern would take for the one given: an X in place of its last dot (#1002).
#
# Parameters:
#   - `Name`: The name.
#
# Returns:
#   - String: The look-alike.
function Get-LookAlikeName {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [string]
        $Name
    )

    $dot = $Name.LastIndexOf('.')
    return $Name.Substring(0, $dot) + 'X' + $Name.Substring($dot + 1)
}

# Invoke-FixtureInstall runs the installer in this session against the fixture release.
#
# The stand-ins for GitHub are functions defined here, so they shadow the cmdlets for this run and no other: a command
# is found by walking out from the scope that calls it, and the installer runs in a scope inside this one. Each answers
# only the request the installer makes, as it makes it, and anything else as GitHub answers an unknown address. The
# variables they read are named fixture*: a variable is found the same way, so one the installer also names would be the
# installer's.
#
# Parameters:
#   - `Name`: The scratch account's name.
#   - `Line`: The lines of a checksums file for the release to carry; without them, it carries none.
#   - `Newline`: What ends each of those lines (default: "`n").
#   - `ArchiveAs`: The name this platform's archive goes under (default: its own name).
#   - `ChecksumsAs`: The name the checksums file goes under (default: its own name).
#   - `Encoding`: The checksums file's encoding (default: UTF-8 with no byte-order mark).
#   - `Before`: Files the release carries before every other, each by name and file.
#   - `NoArchive`: The release carries no archive for this platform.
#   - `Refuse`: Files GitHub lists but answers with 404.
#   - `Lookup`: How the installer finds the release: Tag, the fixture's tag in DEVLORE_VERSION; NoSuchTag, a tag GitHub
#     has no release for; Latest, NoRelease or RateLimited, no DEVLORE_VERSION, and GitHub answers the newest-release
#     lookup with the fixture release, with no release, or with its rate limit, which resets 23 minutes after it is
#     asked (default: Tag).
#   - `Token`: GH_TOKEN for the run; without it, GH_TOKEN is unset.
#   - `Arguments`: The installer's parameters, by name (default: none).
#   - `Quiet`: The installer runs without -Verbose.
#
# Returns:
#   - PSCustomObject: `Message` and `ErrorId`, the installer's error record's, empty when it finished; `Requests`, every
#     address it asked GitHub for, in order, and `Tokens`, those it sent GH_TOKEN with; `Output`, everything it printed;
#     `Errors`, its error records; `Verbose`, what it wrote to the verbose stream; `Information`, how many records it
#     wrote to the information stream; `Prefix`, the scratch account's prefix; and `Reset`, the reset of the last rate
#     limit GitHub answered with, in seconds since the epoch, or 0.
function Invoke-FixtureInstall {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidOverwritingBuiltInCmdlets', '',
        Justification = 'Shadowing Invoke-RestMethod and Invoke-WebRequest is the point: the functions stand in for
        GitHub while the installer runs, and live only in this function''s scope (#1002).')]
    [CmdletBinding()]
    [OutputType([pscustomobject])]
    param(
        [Parameter(Mandatory)]
        [string]
        $Name,

        [string[]]
        $Line,

        [string]
        $Newline = "`n",

        [string]
        $ArchiveAs = $fixtureArchiveName,

        [string]
        $ChecksumsAs = $fixtureChecksumsName,

        [System.Text.Encoding]
        $Encoding = (New-Object -TypeName System.Text.UTF8Encoding -ArgumentList $false),

        [System.Collections.Specialized.OrderedDictionary]
        $Before,

        [switch]
        $NoArchive,

        [string[]]
        $Refuse,

        [ValidateSet('Tag', 'NoSuchTag', 'Latest', 'NoRelease', 'RateLimited')]
        [string]
        $Lookup = 'Tag',

        [string]
        $Token,

        [hashtable]
        $Arguments = @{},

        [switch]
        $Quiet
    )

    $prefix = Use-Account -Name $Name
    $files = [ordered]@{}

    if ($Before) {
        foreach ($assetName in $Before.Keys) {
            $files[$assetName] = $Before[$assetName]
        }
    }

    if ($Line) {
        $checksums = Join-Path (Join-Path $fixtureDir $Name) $fixtureChecksumsName
        New-Item -ItemType Directory -Path (Split-Path -Parent $checksums) -Force | Out-Null
        [System.IO.File]::WriteAllText($checksums, (($Line -join $Newline) + $Newline), $Encoding)
        $files[$ChecksumsAs] = $checksums
    }

    foreach ($platform in $fixturePlatforms) {
        $assetName = "devlore-cli_${fixtureTag}_$platform"

        if ($assetName -cne $fixtureArchiveName) {
            $files[$assetName] = $fixtureOther
        }
        elseif (-not $NoArchive) {
            $files[$ArchiveAs] = $fixtureArchive
        }
    }

    # The release as Invoke-RestMethod returns GitHub's JSON, which the installer asks for only to tell a release GitHub
    # does not have from a file it lacks; the files its links serve, by name, a refused one listed but not served; and
    # every address the installer asks for, in order, and those it sends GH_TOKEN with.
    $fixtureAssets = @()
    $fixtureServed = [ordered]@{}
    $fixtureRequests = New-Object -TypeName 'System.Collections.Generic.List[string]'
    $fixtureTokens = New-Object -TypeName 'System.Collections.Generic.List[string]'
    $fixtureResets = New-Object -TypeName 'System.Collections.Generic.List[long]'

    foreach ($assetName in $files.Keys) {
        $fixtureAssets += [pscustomobject]@{
            id                   = $fixtureAssets.Count + 1
            name                 = $assetName
            browser_download_url = "$fixtureDownload/$fixtureTag/$assetName"
        }

        if ($Refuse -cnotcontains $assetName) {
            $fixtureServed[$assetName] = $files[$assetName]
        }
    }

    $fixtureRelease = [pscustomobject]@{ tag_name = $fixtureTag; assets = $fixtureAssets }
    $fixtureLookup = $Lookup

    # Invoke-RestMethod answers the installer's requests of GitHub's API, as GitHub would.
    #
    # It answers two: the newest release, and a release by its tag, which only the fixture's tag names.
    #
    # Parameters:
    #   - `Uri`: The address asked for.
    #   - `Headers`: The request's headers.
    #   - `UseBasicParsing`: Taken, as the cmdlet takes it, and ignored.
    #
    # Returns:
    #   - PSCustomObject: The release, or an array of releases for the list, as Invoke-RestMethod returns GitHub's JSON.
    function Invoke-RestMethod {
        [CmdletBinding()]
        [OutputType([pscustomobject], [object[]])]
        param(
            [string]
            $Uri,

            [System.Collections.IDictionary]
            $Headers,

            [switch]
            $UseBasicParsing
        )

        $fixtureRequests.Add($Uri)

        if ($Headers -and $Headers.Contains('Authorization')) {
            $fixtureTokens.Add($Uri)
        }

        if (-not $UseBasicParsing -or -not $Headers -or $Headers['Accept'] -cne 'application/vnd.github+json') {
            throw (ConvertTo-SuiteError -Id StandInMisused -Category InvalidArgument `
                -Message ("the stand-in for GitHub takes the installer's request, -UseBasicParsing and JSON " +
                    "accepted: $Uri"))
        }

        if ($Uri -ceq $fixtureByTag) {
            return $fixtureRelease
        }

        if ($Uri -cne $fixtureNewest) {
            $refusal = Get-GitHubRefusal -Status NotFound -Message 'Not Found' `
                -Documentation 'https://docs.github.com/rest/releases/releases#get-a-release-by-tag-name' `
                -Command 'InvokeRestMethodCommand'
            $PSCmdlet.ThrowTerminatingError($refusal)
        }

        if ($fixtureLookup -ceq 'RateLimited') {
            $reset = [System.DateTimeOffset]::UtcNow.ToUnixTimeSeconds() + 23 * 60
            $fixtureResets.Add($reset)
            $limit = [ordered]@{
                'x-ratelimit-limit'     = 60
                'x-ratelimit-remaining' = 0
                'x-ratelimit-reset'     = $reset
                'x-ratelimit-used'      = 60
                'x-ratelimit-resource'  = 'core'
            }
            $refusal = Get-GitHubRefusal -Status Forbidden -Message $fixtureRateLimit `
                -Documentation 'https://docs.github.com/rest/overview/rate-limits-for-the-rest-api' -Header $limit `
                -Command 'InvokeRestMethodCommand'
            $PSCmdlet.ThrowTerminatingError($refusal)
        }

        $releases = @()

        if ($fixtureLookup -cne 'NoRelease') {
            $releases = @($fixtureRelease)
        }

        # GitHub answers with a JSON array. PowerShell 7 writes its elements; Windows PowerShell 5.1 writes the array
        # as one object.
        if ($PSVersionTable.PSEdition -eq 'Core') {
            return $releases
        }

        return , $releases
    }

    # Invoke-WebRequest answers the release's public links, as GitHub's download host answers them.
    #
    # It saves the file whose name matches in any case, named in the Content-Disposition of the response this edition's
    # cmdlet returns with -PassThru, or refuses with 404 and "Not Found" in plain text.
    #
    # Parameters:
    #   - `Uri`: The link asked for.
    #   - `Headers`: The request's headers.
    #   - `OutFile`: Where to save the file.
    #   - `PassThru`: Return the response, as well as saving the file.
    #   - `UseBasicParsing`: Taken, as the cmdlet takes it, and ignored.
    #
    # Returns:
    #   - PSCustomObject: The response, with -PassThru.
    function Invoke-WebRequest {
        [CmdletBinding()]
        [OutputType([pscustomobject])]
        param(
            [string]
            $Uri,

            [System.Collections.IDictionary]
            $Headers,

            [string]
            $OutFile,

            [switch]
            $PassThru,

            [switch]
            $UseBasicParsing
        )

        $fixtureRequests.Add($Uri)

        if ($Headers -and $Headers.Contains('Authorization')) {
            $fixtureTokens.Add($Uri)
        }

        if (-not $UseBasicParsing -or -not $PassThru -or -not $OutFile) {
            throw (ConvertTo-SuiteError -Id StandInMisused -Category InvalidArgument `
                -Message ("the stand-in for GitHub takes the installer's request, -UseBasicParsing, -OutFile and " +
                    "-PassThru: $Uri"))
        }

        $link = "$fixtureDownload/$fixtureTag/"

        if ($Uri.StartsWith($link, [System.StringComparison]::Ordinal)) {
            $asked = $Uri.Substring($link.Length)

            foreach ($served in $fixtureServed.Keys) {
                if (-not [string]::Equals($served, $asked, [System.StringComparison]::OrdinalIgnoreCase)) {
                    continue
                }

                Copy-Item -LiteralPath $fixtureServed[$served] -Destination $OutFile
                $disposition = "attachment; filename=$served"

                # PowerShell 7's response gives a header's values as an array; Windows PowerShell 5.1's as one string.
                if ($PSVersionTable.PSEdition -eq 'Core') {
                    $answer = New-Object -TypeName ('System.Collections.Generic.Dictionary[string,' +
                        'System.Collections.Generic.IEnumerable[string]]')
                    $answer['Content-Disposition'] = [string[]]@($disposition)
                }
                else {
                    $answer = New-Object -TypeName 'System.Collections.Generic.Dictionary[string,string]'
                    $answer['Content-Disposition'] = $disposition
                }

                return [pscustomobject]@{ StatusCode = 200; Headers = $answer }
            }
        }

        $refusal = Get-GitHubRefusal -Status NotFound -Message 'Not Found' -Command 'InvokeWebRequestCommand'
        $PSCmdlet.ThrowTerminatingError($refusal)
    }

    # Every stream redirected, as a caller that captures the install redirects them: each line is kept as text, and an
    # error record among them is counted, since a program's stderr carried by PowerShell arrives as one (#1029,
    # Requirement 7). The installer reports a failure with an error record of its own, named for Install-DevLore, under
    # its caller's error action (#1044): this caller continues on error, so the record arrives here among the rest, and
    # the run is judged by it. Its narration arrives as verbose records, and nothing may arrive on the information
    # stream.
    $output = New-Object -TypeName 'System.Collections.Generic.List[string]'
    $errorRecords = New-Object -TypeName 'System.Collections.Generic.List[string]'
    $verbose = New-Object -TypeName 'System.Collections.Generic.List[string]'

    # What the run reported, in one table the ForEach-Object block below writes into: the installer's error record's
    # message and id, and how many information records it wrote.
    $reported = @{ Message = ''; ErrorId = ''; Information = 0 }
    $verboseSwitch = @{ Verbose = -not $Quiet }
    $env:DEVLORE_VERSION = $fixtureTag

    if ($Lookup -ceq 'NoSuchTag') {
        $env:DEVLORE_VERSION = $fixtureMissingTag
    }
    elseif ($Lookup -cne 'Tag') {
        $env:DEVLORE_VERSION = ''
    }

    $env:GH_TOKEN = $Token

    $ErrorActionPreference = 'Continue'

    try {
        & $block -Prefix $prefix @verboseSwitch @Arguments *>&1 | ForEach-Object {
            if ($_ -is [System.Management.Automation.ErrorRecord]) {
                $errorRecords.Add("$_")

                if ($_.FullyQualifiedErrorId.EndsWith(',Install-DevLore', [System.StringComparison]::Ordinal)) {
                    $reported.Message = $_.Exception.Message
                    $reported.ErrorId = $_.FullyQualifiedErrorId
                }
            }
            elseif ($_ -is [System.Management.Automation.VerboseRecord]) {
                $verbose.Add($_.Message)
            }
            elseif ($_ -is [System.Management.Automation.InformationRecord]) {
                $reported.Information++
            }

            $output.Add("$_")
        }
    }
    catch {
        # Nothing the installer does should end this run: it reports its own failures. A terminating error here is the
        # suite's to show.
        $reported.Message = $_.Exception.Message
        $reported.ErrorId = "terminating: $($_.FullyQualifiedErrorId)"
    }
    finally {
        $env:DEVLORE_VERSION = $savedVersion
        $env:GH_TOKEN = $savedToken
    }

    $reset = 0

    if ($fixtureResets.Count -gt 0) {
        $reset = $fixtureResets[$fixtureResets.Count - 1]
    }

    $script:fixtureRuns.Add([pscustomobject]@{ Account = $Name; Leftovers = @(Get-Leftover -Account $Name) })

    return [pscustomobject]@{
        Message     = $reported.Message
        ErrorId     = $reported.ErrorId
        Requests    = $fixtureRequests.ToArray()
        Tokens      = $fixtureTokens.ToArray()
        Output      = $output -join "`n"
        Errors      = $errorRecords.ToArray()
        Verbose     = $verbose.ToArray()
        Information = $reported.Information
        Prefix      = $prefix
        Reset       = $reset
    }
}

# Test-Refusal checks a fixture run that must be refused, with two checks.
#
# "<case>: <expected>" passes when the installer reported an error record of its own whose id is -Id, with exactly
# -Message, and stopped before it extracted or installed anything. "<case>: asks GitHub for ..." is Test-Request's, of
# the addresses in -Requests.
#
# Parameters:
#   - `Case`: The case's name.
#   - `Expected`: What the case expects, for the first check's name.
#   - `Result`: The run, as Invoke-FixtureInstall returns it.
#   - `Message`: The refusal's message, exactly.
#   - `Id`: The refusal's error id.
#   - `Requests`: The addresses the run must ask GitHub for, in order.
#
# Returns:
#   - String: The checks' lines.
function Test-Refusal {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Case,

        [Parameter(Mandatory)]
        [string]
        $Expected,

        [Parameter(Mandatory)]
        [pscustomobject]
        $Result,

        [Parameter(Mandatory)]
        [string]
        $Message,

        [Parameter(Mandatory)]
        [string]
        $Id,

        [Parameter(Mandatory)]
        [string[]]
        $Requests
    )

    $expectedId = "$Id,Install-DevLore"
    $extracted = $Result.Output.Contains('Extracting') -or (Test-Path -LiteralPath (Join-Path $Result.Prefix 'bin'))
    Test-Expectation -Description "${Case}: $Expected" `
        -Condition ($Result.Message -ceq $Message -and $Result.ErrorId -ceq $expectedId -and -not $extracted) `
        -Detail ("expected: $Message`nmessage:  $($Result.Message)`nid: $($Result.ErrorId), expected $expectedId; " +
            "extracted or installed: $extracted`n$($Result.Output)")
    Test-Request -Case $Case -Result $Result -Requests $Requests
}

# Test-Verified checks a fixture run that must install, with two checks.
#
# "<case>: <expected>" passes when the run finished, said on the verbose stream that it verified the checksum, and
# installed the writ it verified, whose version is the release's (#1031). "<case>: asks GitHub for ..." is
# Test-Request's.
#
# Parameters:
#   - `Case`: The case's name.
#   - `Expected`: What the case expects, for the first check's name.
#   - `Result`: The run, as Invoke-FixtureInstall returns it.
#   - `Requests`: The addresses the run must ask GitHub for, in order (default: the checksums file's link, then the
#     archive's).
#
# Returns:
#   - String: The checks' lines.
function Test-Verified {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Case,

        [Parameter(Mandatory)]
        [string]
        $Expected,

        [Parameter(Mandatory)]
        [pscustomobject]
        $Result,

        [string[]]
        $Requests = @($fixtureChecksumsLink, $fixtureArchiveLink)
    )

    $writ = Join-Path (Join-Path $Result.Prefix 'bin') $writName
    $version = ''

    if (Test-Path -LiteralPath $writ) {
        $version = (& $writ --version) -join "`n"
    }

    Test-Expectation -Description "${Case}: $Expected" `
        -Condition (-not $Result.Message -and $Result.Verbose -ccontains 'Checksum verified' -and
            $version.Contains($fixtureTag)) `
        -Detail "message: $($Result.Message)`nwrit --version: $version`n$($Result.Output)"
    Test-Request -Case $Case -Result $Result -Requests $Requests
}

# Get-RequestName gets what an address the installer asked GitHub for is, in words, for the name of a check.
#
# Parameters:
#   - `Url`: The address.
#
# Returns:
#   - String: Its name.
function Get-RequestName {
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)]
        [string]
        $Url
    )

    $byTag = "$fixtureApi/releases/tags/"
    $link = "$fixtureDownload/"

    if ($Url -ceq $fixtureNewest) {
        return "the API's newest release"
    }

    if ($Url.StartsWith($byTag, [System.StringComparison]::Ordinal)) {
        return "the API's release $($Url.Substring($byTag.Length))"
    }

    if (-not $Url.StartsWith($link, [System.StringComparison]::Ordinal)) {
        return $Url
    }

    $tag, $file = $Url.Substring($link.Length) -split '/', 2
    $what = "$file"

    if ($file -ceq "devlore-cli_${tag}_checksums.txt") {
        $what = 'the checksums file'
    }
    elseif ($file -ceq $fixtureArchiveName) {
        $what = 'the archive'
    }

    if ($tag -cne $fixtureTag) {
        $what += " of $tag"
    }

    return $what
}

# Test-Request checks that a fixture run asked GitHub for exactly the addresses given, in that order.
#
# Each file is asked for by its public link, the checksums file before the archive, and the archive only once the
# checksums file has its line (Requirement 1); GitHub's API only for the newest release's tag, or for the release when a
# link answered 404 before the release was found (#1008). These are the requests install.sh makes in the same case. The
# check names each by what it is, so its name says what the installer may ask for.
#
# Parameters:
#   - `Case`: The case's name.
#   - `Result`: The run, as Invoke-FixtureInstall returns it.
#   - `Requests`: The addresses, in order.
#
# Returns:
#   - String: The check's lines.
function Test-Request {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]
        $Case,

        [Parameter(Mandatory)]
        [pscustomobject]
        $Result,

        [Parameter(Mandatory)]
        [string[]]
        $Requests
    )

    $what = @(
        foreach ($url in $Requests) {
            Get-RequestName -Url $url
        }
    )
    $phrase = $what -join ', then '

    if ($Requests -cnotcontains $fixtureArchiveLink) {
        $phrase += ', not the archive'
    }

    Test-Expectation -Description "${Case}: asks GitHub for $phrase" `
        -Condition (($Result.Requests -join "`n") -ceq ($Requests -join "`n")) `
        -Detail "asked:    $($Result.Requests -join ', ')`nexpected: $($Requests -join ', ')"
}

# Test-GitHubItself checks two of the installer's readers of GitHub's answers against GitHub itself.
#
# So what the stand-ins model is what this edition's cmdlets give the installer. The newest release's checksums file,
# asked for in upper case, comes back under its own name, which Get-ServedName must read; and a file the release lacks
# is refused with 404 and "Not Found" in plain text, which Get-GitHubAnswer must report. The installer's functions are
# defined here from its source, and Install-DevLore is not run: the installer has no hook, and run whole it never asks
# for a name it does not build.
#
# Returns:
#   - String: The checks' lines.
function Test-GitHubItself {
    [CmdletBinding()]
    param()

    $parsed = [System.Management.Automation.Language.Parser]::ParseFile($installer, [ref]$null, [ref]$null)
    $isFunction = { $args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] }

    foreach ($definition in $parsed.FindAll($isFunction, $true)) {
        . ([scriptblock]::Create($definition.Extent.Text))
    }

    $headers = @{ Accept = 'application/vnd.github+json' }

    if ($env:GH_TOKEN) {
        $headers['Authorization'] = "Bearer $env:GH_TOKEN"
    }

    try {
        $releases = Invoke-RestMethod -Uri $fixtureNewest -Headers $headers -UseBasicParsing
        $tag = [string]$releases[0].tag_name
    }
    catch {
        Write-Fail -Description 'GitHub itself: the newest release, to check the installer against' `
            -Detail $_.Exception.Message
        return
    }

    $checksums = "devlore-cli_${tag}_checksums.txt"
    $saved = Join-Path $scratch 'github-itself'
    New-Item -ItemType Directory -Path $saved -Force | Out-Null
    $served = ''

    try {
        $response = Invoke-WebRequest -Uri "$fixtureDownload/$tag/DEVLORE-CLI_${tag}_CHECKSUMS.TXT" `
            -OutFile (Join-Path $saved $checksums) -PassThru -UseBasicParsing
        $served = Get-ServedName -Response $response
    }
    catch {
        $served = "error: $($_.Exception.Message)"
    }

    Test-Expectation -Condition ($served -ceq $checksums) -Detail "read: $served`nexpected: $checksums" `
        -Description "GitHub itself: $tag's checksums file, asked for in upper case, is read as served by its own name"

    $answer = ''

    try {
        $null = Invoke-WebRequest -Uri "$fixtureDownload/$tag/devlore-cli_${tag}_no-such-file.txt" `
            -OutFile (Join-Path $saved 'no-such-file.txt') -PassThru -UseBasicParsing
        $answer = 'downloaded'
    }
    catch {
        try {
            $answer = Get-GitHubAnswer -ErrorRecord $_
        }
        catch {
            $answer = "error: $($_.Exception.Message)"
        }
    }

    Test-Expectation -Description "GitHub itself: a file $tag lacks is refused with GitHub's status and text" `
        -Condition ($answer -ceq 'GitHub answered HTTP 404: Not Found') -Detail "read: $answer"
}

###########
# Main
###########

Write-Output -InputObject "PowerShell $($PSVersionTable.PSVersion) ($($PSVersionTable.PSEdition))"

$block = [scriptblock]::Create((Get-Content -Raw -LiteralPath $installer))
$savedVersion = $env:DEVLORE_VERSION
$savedToken = $env:GH_TOKEN

# The main block runs inside try and catch, as the style guide's section 7 has a script do: the catch puts the caller's
# error action back, reports an error of the suite's own with $PSCmdlet.WriteError, and exits 1. The finally cleans up.
try {
    # --- Off Windows, the one check: install.ps1 installs on Windows alone, and refuses anywhere else (#1032) ---
    #
    # The installer reports the refusal with its own error record, under its caller's error action (#1044): continued
    # here, so it arrives among the streams, by its id.

    if (-not $onWindows) {
        $prefix = Use-Account -Name 'not-windows'
        $records = @(& $block -Prefix $prefix -ErrorAction Continue 2>&1 |
                Where-Object { $_ -is [System.Management.Automation.ErrorRecord] })
        $refusal = @($records | Where-Object { $_.FullyQualifiedErrorId -ceq 'RequiresWindows,Install-DevLore' })
        Test-Expectation `
            -Description 'off Windows: refused with the record RequiresWindows, "This script requires Windows."' `
            -Condition ($refusal.Count -eq 1 -and
                $refusal[0].Exception.Message -ceq 'This script requires Windows.' -and
                -not (Test-Path -LiteralPath $prefix)) `
            -Detail "records:`n$(($records | ForEach-Object { "$($_.FullyQualifiedErrorId): $_" }) -join "`n")"

        if ($script:failures -gt 0) {
            Write-Output -InputObject "$($script:failures) check(s) failed"
            exit 1
        }

        Write-Output -InputObject 'every check passed (off Windows, the refusal is all there is to check)'
        exit 0
    }

    # --- The faux channel (#1031): its release carries this checkout's programs, and every install below is from it ---

    New-Item -ItemType Directory -Path $fixtureDir -Force | Out-Null
    $fixtureArchive = Initialize-FixtureArchive
    $fixtureOther = Join-Path $fixtureDir 'not-the-archive'
    Set-Content -LiteralPath $fixtureOther -Value "not $fixtureArchiveName"
    $fixtureHash = (Get-FileHash -LiteralPath $fixtureArchive -Algorithm SHA256).Hash.ToLowerInvariant()
    $listed = "$fixtureHash  $fixtureArchiveName"

    # --- The script-block form: a path layer and a URL layer, run twice; the second changes nothing ---

    $layerArguments = @{ Personal = $repo; Team = $teamUrl }
    $writ = Join-Path (Join-Path (Join-Path (Join-Path $scratch 'layers') 'prefix') 'bin') $writName
    $runs = @()

    foreach ($run in 1, 2) {
        $result = Invoke-FixtureInstall -Name 'layers' -Line @($listed) -Arguments $layerArguments
        Test-Expectation -Description "script-block form: run $run completes" -Condition (-not $result.Message) `
            -Detail "message: $($result.Message)`n$($result.Output)"
        Test-Expectation `
            -Description "script-block form: run ${run}, every stream redirected, carries no error record" `
            -Condition ($result.Errors.Count -eq 0) `
            -Detail "error records:`n$($result.Errors -join "`n")`n----`n$($result.Output)"
        Test-Expectation -Description "script-block form: run ${run} writes nothing to the information stream" `
            -Condition ($result.Information -eq 0) -Detail "information records: $($result.Information)"
        $runs += Get-Registration -Writ $writ
    }

    $personalRoot = Get-LayerRoot -Writ $writ -Layer 'personal'
    Test-Expectation -Description 'script-block form: personal is the checkout' -Condition ($personalRoot -eq $repo) `
        -Detail "personal: $personalRoot"
    $teamRoot = Get-LayerRoot -Writ $writ -Layer 'team'
    $expectedTeam = Join-Path (Join-Path (Join-Path (Join-Path $env:XDG_DATA_HOME 'devlore') 'writ') 'repos') `
        'noblefactor-ops'
    Test-Expectation -Description 'script-block form: team is cloned into the account' `
        -Condition ($teamRoot -eq $expectedTeam) -Detail "team: $teamRoot; expected: $expectedTeam"
    Test-Expectation -Description 'script-block form: the second run leaves the registrations as the first did' `
        -Condition ($runs.Count -eq 2 -and $runs[0] -and $runs[0] -eq $runs[1]) -Detail ($runs -join "`n----`n")

    # --- No flags, where writ already has team and personal: they are named as registered, and only base is skipped ---

    $result = Invoke-FixtureInstall -Name 'layers' -Line @($listed)
    Test-Expectation -Description 'script-block form, no flags: names the layers writ already has' `
        -Condition (-not $result.Message -and $result.Output.Contains('Already registered: team personal')) `
        -Detail "message: $($result.Message)`n$($result.Output)"
    $skippedLines = @([regex]::Matches($result.Output, '(?m)^skipped: .*$') | ForEach-Object { $_.Value.TrimEnd() })
    Test-Expectation -Description 'script-block form, no flags: skips base alone' `
        -Condition ($skippedLines.Count -eq 1 -and $skippedLines[0] -eq 'skipped: base; to register it later:') `
        -Detail $result.Output

    # --- The PATH advice (#1050): a fixture install is on no PATH, so the summary says how to add it, needing no
    # administrator, and to open a new terminal ---

    Test-Expectation -Description 'script-block form, no flags: the PATH advice asks for no administrator' `
        -Condition (-not $result.Output.Contains('Administrator')) -Detail $result.Output
    Test-Expectation -Description 'script-block form, no flags: the PATH advice says to open a new terminal' `
        -Condition ($result.Output.Contains('then open a new terminal: this one keeps the PATH it started with.')) `
        -Detail $result.Output

    # --- Every layer registered (#1050): base too, a working tree made here with git init, all a path layer needs.
    # Then no flags names all three as registered, skips none, and ends with the documentation line: no next step ---

    $baseRoot = Join-Path $scratch 'base-layer'
    New-Item -ItemType Directory -Path $baseRoot -Force | Out-Null
    & git init --quiet $baseRoot
    Test-Expectation -Description 'every layer: git init makes the base layer' -Condition ($LASTEXITCODE -eq 0) `
        -Detail "git init exited $LASTEXITCODE"

    $result = Invoke-FixtureInstall -Name 'layers' -Line @($listed) -Arguments @{ Base = $baseRoot }
    Test-Expectation -Description 'every layer: base given, registers it, and the next step is writ deploy' `
        -Condition (-not $result.Message -and $result.Output.Contains('Registered: base') -and
            $result.Output.Contains("Next steps:`n  writ deploy")) `
        -Detail "message: $($result.Message)`n$($result.Output)"

    $result = Invoke-FixtureInstall -Name 'layers' -Line @($listed)
    $summary = @($result.Output -split "`n" | Where-Object { $_.Trim() -and $result.Verbose -notcontains $_ })
    Test-Expectation -Description 'every layer, no flags: names all three as already registered' `
        -Condition (-not $result.Message -and $result.Output.Contains('Already registered: base team personal')) `
        -Detail "message: $($result.Message)`n$($result.Output)"
    Test-Expectation -Description 'every layer, no flags: skips none and gives no next step' `
        -Condition (-not ($result.Output -match '(?m)^skipped: ') -and -not $result.Output.Contains('Next steps:')) `
        -Detail $result.Output
    Test-Expectation -Description 'every layer, no flags: ends with the documentation line' `
        -Condition ($summary.Count -gt 0 -and
            $summary[-1].Trim() -eq 'Documentation: https://github.com/NobleFactor/devlore-cli#readme') `
        -Detail $result.Output

    # --- -Help gives the usage on the output stream and returns ---

    $null = Use-Account -Name 'help'

    try {
        $help = @(& $block -Help 6>&1)
        $helpInformation = @($help | Where-Object { $_ -is [System.Management.Automation.InformationRecord] })
        $helpText = ($help | Where-Object { $_ -isnot [System.Management.Automation.InformationRecord] }) -join "`n"
        Test-Expectation -Description '-Help: gives the usage on the output stream and returns' `
            -Condition ($helpText -match 'Usage: install\.ps1' -and $helpInformation.Count -eq 0) `
            -Detail "information records: $($helpInformation.Count)`n$helpText"
    }
    catch {
        Write-Fail -Description '-Help: gives the usage on the output stream and returns' -Detail $_.Exception.Message
    }

    # --- A failure is an error record, and the session goes on (#965, #1044): an exit would end this script here ---
    #
    # A caller who continues on error gets the record and goes on; one who stops on error catches it in their own try.

    $prefix = Use-Account -Name 'failure'
    $env:DEVLORE_VERSION = $fixtureMissingTag
    $message = ''
    $records = @()
    $caught = ''

    try {
        $records = @(& $block -Prefix $prefix -ErrorAction Continue 2>&1 |
                Where-Object { $_ -is [System.Management.Automation.ErrorRecord] })

        try {
            & $block -Prefix $prefix -ErrorAction Stop
        }
        catch {
            $caught = $_.FullyQualifiedErrorId
        }
    }
    finally {
        $env:DEVLORE_VERSION = $savedVersion
    }

    $refusal = @($records | Where-Object { $_.FullyQualifiedErrorId -ceq 'GitHubRefused,Install-DevLore' })

    if ($refusal.Count -eq 1) {
        $message = $refusal[0].Exception.Message
    }

    Test-Expectation `
        -Description 'a forced failure: reported as the record GitHubRefused, and the next statement runs' `
        -Condition ($refusal.Count -eq 1 -and $message.Contains(
            "Could not fetch release $fixtureMissingTag of NobleFactor/devlore-cli")) `
        -Detail "records:`n$(($records | ForEach-Object { "$($_.FullyQualifiedErrorId): $_" }) -join "`n")"
    Test-Expectation `
        -Description 'a forced failure, its caller stopping on error: the caller''s try catches the record' `
        -Condition ($caught -ceq 'GitHubRefused,Install-DevLore') -Detail "caught: $caught"

    # GitHub itself, not a stand-in: the one refusal whose error record is the edition's own, on every runner, so the
    # sentence the stand-ins draw from the installer in case 16 is the one GitHub draws from it (#1002).
    Test-Expectation -Description 'a release GitHub does not have: refused with GitHub''s status and message' `
        -Condition ($message -ceq ("Could not fetch release $fixtureMissingTag of NobleFactor/devlore-cli: " +
            'GitHub answered HTTP 404: Not Found')) -Detail "message: $message"

    # The other two answers the stand-ins model, from GitHub itself: the name its download host served, and a file a
    # release lacks.
    Test-GitHubItself

    # --- The one list of cases (#1002), which install.sh's test runs too, each check named by the plan's number ---

    $upper = $fixtureArchiveName.ToUpperInvariant()
    $neighbor = "$('0' * 64)  devlore-cli_${fixtureTag}_plan9_amd64.tar.gz"
    $unlisted = Join-Path $fixtureDir 'unlisted.txt'
    Set-Content -LiteralPath $unlisted -Value $neighbor

    # The sentences the cases check, each install.sh's in the same case, and the requests each case makes, each the
    # requests install.sh makes in it.
    $notFound = 'GitHub answered HTTP 404: Not Found'
    $noChecksums = "Could not download $fixtureChecksumsName from release ${fixtureTag}: $notFound"
    $noLine = "$fixtureChecksumsName has no line for $fixtureArchiveName, so the archive cannot be verified"
    $noArchive = "Could not download $fixtureArchiveName from release ${fixtureTag}: $notFound"
    $checksumsOnly = @($fixtureChecksumsLink)
    $checksumsThenRelease = @($fixtureChecksumsLink, $fixtureByTag)
    $both = @($fixtureChecksumsLink, $fixtureArchiveLink)

    $result = Invoke-FixtureInstall -Name 'case-1'
    Test-Refusal -Case 'case 1: no checksums file in the release' -Expected 'refused, naming the file and the tag' `
        -Result $result -Message $noChecksums -Requests $checksumsThenRelease -Id DownloadRefused

    $result = Invoke-FixtureInstall -Name 'case-2' -Line $neighbor
    Test-Refusal -Case 'case 2: no line for the archive' -Result $result -Message $noLine -Requests $checksumsOnly `
        -Expected 'refused, naming the archive and the checksums file' -Id NoChecksum

    $result = Invoke-FixtureInstall -Name 'case-3' -Line $neighbor, "$fixtureHash  ${fixtureArchiveName}.sig"
    Test-Refusal -Case 'case 3: a line only for <archive>.sig' -Expected 'refused, as 2' `
        -Result $result -Message $noLine -Requests $checksumsOnly -Id NoChecksum

    $pattern = Get-LookAlikeName -Name $fixtureArchiveName
    $result = Invoke-FixtureInstall -Name 'case-4' -Line $neighbor, "$fixtureHash  $pattern"
    Test-Refusal -Case 'case 4: a line that matches only as a pattern, a dot replaced' -Expected 'refused, as 2' `
        -Result $result -Message $noLine -Requests $checksumsOnly -Id NoChecksum

    $result = Invoke-FixtureInstall -Name 'case-5' -Line $neighbor, "$fixtureHash  $upper"
    Test-Refusal -Case 'case 5: a line naming the archive in another case' -Expected 'refused, as 2' `
        -Result $result -Message $noLine -Requests $checksumsOnly -Id NoChecksum

    $mismatch = 'ab' * 32
    $result = Invoke-FixtureInstall -Name 'case-6' -Line $neighbor, "$mismatch  $fixtureArchiveName"
    Test-Refusal -Case 'case 6: a mismatch' -Expected 'refused, naming both hashes' -Result $result `
        -Message "Checksum verification failed!`nExpected: $mismatch`nActual:   $fixtureHash" -Requests $both `
        -Id ChecksumMismatch

    $result = Invoke-FixtureInstall -Name 'case-7' -Line $neighbor, $listed -Refuse $fixtureChecksumsName
    Test-Refusal -Case 'case 7: the checksums file''s download refused (404)' -Result $result `
        -Expected 'refused, with GitHub''s message and the file''s name' -Requests $checksumsThenRelease `
        -Message $noChecksums -Id DownloadRefused

    $result = Invoke-FixtureInstall -Name 'case-8' -Line $neighbor, $listed -Refuse $fixtureArchiveName
    Test-Refusal -Case 'case 8: the archive''s download refused (404)' -Result $result `
        -Expected 'refused, with GitHub''s message and the archive''s name' -Requests $both -Message $noArchive `
        -Id DownloadRefused

    # The checksums file's link has shown the release exists, so GitHub's API is asked nothing.
    $result = Invoke-FixtureInstall -Name 'case-9' -Line $neighbor, $listed -NoArchive
    Test-Refusal -Case 'case 9: no archive for this platform in the release' -Result $result -Message $noArchive `
        -Expected 'refused, naming the archive and the tag' -Requests $both -Id DownloadRefused

    $result = Invoke-FixtureInstall -Name 'case-10' -Line $neighbor, $listed -ArchiveAs $upper
    Test-Refusal -Case 'case 10: the archive published under a name in another case' -Expected 'refused, as 9' `
        -Result $result -Requests $both -Message ("Could not download $fixtureArchiveName from release " +
            "${fixtureTag}: GitHub served $upper, a file by another name") -Id ServedOtherFile

    # Each look-alike is another file, listed first: the checksums file's lacks the archive's line, and the archive's
    # holds other bytes, so taking either is a refusal, and asking for either is a FAIL of the request check.
    $before = [ordered]@{ (Get-LookAlikeName -Name $fixtureChecksumsName) = $unlisted }
    $result = Invoke-FixtureInstall -Name 'case-11-checksums' -Line $neighbor, $listed -Before $before
    Test-Verified -Case 'case 11: a look-alike of the checksums file, a dot replaced, listed first' `
        -Expected 'the real one taken, installs' -Result $result

    $before = [ordered]@{ (Get-LookAlikeName -Name $fixtureArchiveName) = $fixtureOther }
    $result = Invoke-FixtureInstall -Name 'case-11-archive' -Line $neighbor, $listed -Before $before
    Test-Verified -Case 'case 11: a look-alike of the archive, a dot replaced, listed first' `
        -Expected 'the real one taken, installs' -Result $result

    $result = Invoke-FixtureInstall -Name 'case-12-none' -Line $neighbor, $listed -Lookup NoRelease
    Test-Refusal -Case 'case 12: no release, on the latest-release lookup' -Expected 'refused, saying so' `
        -Result $result -Requests $fixtureNewest `
        -Message 'Could not determine the latest release of NobleFactor/devlore-cli: GitHub lists none' -Id NoRelease

    # The rate limit says when to run again (Requirement 3b): the reset as a clock time and in minutes, and, only when
    # GH_TOKEN is unset, that setting it raises the limit.
    $rateLimited = "Could not list the releases of NobleFactor/devlore-cli: GitHub answered HTTP 403: $fixtureRateLimit"
    $result = Invoke-FixtureInstall -Name 'case-12-rate-limit' -Line $neighbor, $listed -Lookup RateLimited
    $clock = Get-ClockTime -Seconds $result.Reset
    Test-Refusal -Case 'case 12: a rate limit, on the latest-release lookup' -Result $result `
        -Expected 'refused, saying so, with GitHub''s message, when to run again, and that GH_TOKEN raises the limit' `
        -Requests $fixtureNewest `
        -Message ("$rateLimited`nGitHub's API limit for this address is used up. It resets at $clock (in 23 " +
            "minutes); run the installer again after that. Setting GH_TOKEN raises the limit.") `
        -Id GitHubRefused

    $result = Invoke-FixtureInstall -Name 'case-12-rate-limit-token' -Line $neighbor, $listed -Lookup RateLimited `
        -Token 'fixture-token'
    $clock = Get-ClockTime -Seconds $result.Reset
    Test-Refusal -Case 'case 12: a rate limit, with GH_TOKEN set' -Result $result `
        -Expected 'refused, saying so, with GitHub''s message and when to run again, and nothing of GH_TOKEN' `
        -Requests $fixtureNewest -Message ("$rateLimited`nGitHub's API limit for your token is used up. It resets at " +
            "$clock (in 23 minutes); run the installer again after that.") -Id GitHubRefused

    # A pinned DEVLORE_VERSION that names a release: GitHub's API is asked nothing.
    $result = Invoke-FixtureInstall -Name 'case-13-plain' -Line $neighbor, $listed
    Test-Verified -Case 'case 13: plain lines' -Expected '"Checksum verified", installs' -Result $result

    $result = Invoke-FixtureInstall -Name 'case-13-crlf' -Line $neighbor, $listed -Newline "`r`n"
    Test-Verified -Case 'case 13: CRLF lines' -Expected '"Checksum verified", installs' -Result $result

    $result = Invoke-FixtureInstall -Name 'case-13-binary' -Line $neighbor, "$fixtureHash *$fixtureArchiveName"
    Test-Verified -Case 'case 13: <hash> *<name> lines' -Expected '"Checksum verified", installs' -Result $result

    $result = Invoke-FixtureInstall -Name 'case-14' -Line $neighbor, $listed `
        -ChecksumsAs $fixtureChecksumsName.ToUpperInvariant()
    Test-Refusal -Case 'case 14: the checksums file published under a name in another case' -Expected 'refused, as 1' `
        -Result $result -Requests $checksumsOnly -Message ("Could not download $fixtureChecksumsName from release " +
            "${fixtureTag}: GitHub served $($fixtureChecksumsName.ToUpperInvariant()), a file by another name") `
        -Id ServedOtherFile

    # A byte-order mark is part of the first line, as self upgrade and sha256sum read it: its hash is not 64 hex
    # characters, so the archive's line behind it is no line.
    $result = Invoke-FixtureInstall -Name 'case-15' -Line $listed, $neighbor `
        -Encoding (New-Object -TypeName System.Text.UTF8Encoding -ArgumentList $true)
    Test-Refusal -Case 'case 15: the archive''s line behind a byte-order mark' -Expected 'refused, as 2' `
        -Result $result -Message $noLine -Requests $checksumsOnly -Id NoChecksum

    # GitHub's API is asked for the release only after its checksums file's link answers 404.
    $result = Invoke-FixtureInstall -Name 'case-16' -Line $neighbor, $listed -Lookup NoSuchTag
    Test-Refusal -Case 'case 16: a pinned DEVLORE_VERSION that names no release' -Expected 'refused, naming the tag' `
        -Result $result -Message "Could not fetch release $fixtureMissingTag of NobleFactor/devlore-cli: $notFound" `
        -Requests "$fixtureDownload/$fixtureMissingTag/devlore-cli_${fixtureMissingTag}_checksums.txt",
        "$fixtureApi/releases/tags/$fixtureMissingTag" -Id GitHubRefused

    # GH_TOKEN goes to GitHub's API alone, which it lets past the anonymous limit, and never to a download link.
    $result = Invoke-FixtureInstall -Name 'token' -Line $neighbor, $listed -Lookup Latest -Token 'fixture-token'
    Test-Verified -Case 'the newest release, with GH_TOKEN set' -Expected '"Checksum verified", installs' `
        -Result $result -Requests $fixtureNewest, $fixtureChecksumsLink, $fixtureArchiveLink
    Test-Expectation -Description 'the newest release, with GH_TOKEN set: sends it to GitHub''s API alone' `
        -Condition (($result.Tokens -join "`n") -ceq $fixtureNewest) -Detail "sent with: $($result.Tokens -join ', ')"

    # --- A plain run, without -Verbose: the results, and no narration (#1044) ---

    $result = Invoke-FixtureInstall -Name 'quiet' -Line $neighbor, $listed -Quiet
    Test-Expectation -Description 'a plain run: installs, its results on the output stream, no narration' `
        -Condition (-not $result.Message -and $result.Output.Contains('Installed: ') -and
            $result.Verbose.Count -eq 0 -and $result.Information -eq 0) `
        -Detail ("message: $($result.Message); verbose records: $($result.Verbose.Count); information records: " +
            "$($result.Information)`n$($result.Output)")

    # --- A program the installer runs reads nothing of the installer's standard input (#1044) ---
    #
    # In a child session whose standard input is a file, the installer's own Invoke-NativeCommand runs this
    # PowerShell reading its standard input to the end. Closed, as section 7 has it, that input is empty; inherited, it
    # would be the file's. The child's console input encoding is UTF-8 with a preamble first, which Windows PowerShell
    # 5.1 writes to a redirected input as a byte-order mark (#1104), so the check finds one on any console, a runner's
    # or an OEM one; and afterwards it is that encoding again, as the caller had it.

    $stdinFile = Join-Path $scratch 'stdin.txt'
    Set-Content -LiteralPath $stdinFile -Value 'the installer''s own standard input'
    $probeCommand = @"
`$parsed = [System.Management.Automation.Language.Parser]::ParseFile('$installer', [ref]`$null, [ref]`$null)
`$isFunction = { `$args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] }
foreach (`$definition in `$parsed.FindAll(`$isFunction, `$true)) {
    . ([scriptblock]::Create(`$definition.Extent.Text))
}
try {
    [Console]::InputEncoding = New-Object -TypeName System.Text.UTF8Encoding -ArgumentList `$true
}
catch {
    'no console: the input encoding cannot be set'
}
`$read = Invoke-NativeCommand -FilePath '$shell' ``
    -ArgumentList @('-NoProfile', '-NonInteractive', '-Command', '[Console]::In.ReadToEnd().Length')
"stdin read: `$read"
`$after = [Console]::InputEncoding
"input encoding after: `$(`$after.CodePage), preamble `$(`$after.GetPreamble().Length)"
"@
    $probe = Invoke-Child -Name 'stdin' -InputFile $stdinFile -ArgumentList @(
        '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-EncodedCommand',
        (ConvertTo-EncodedCommand -Command $probeCommand))
    Test-Expectation -Description 'a program the installer runs: its standard input closed, it reads nothing' `
        -Condition ($probe.Output -match '(?m)^stdin read: 0\s*$') -Detail "exit $($probe.ExitCode)`n$($probe.Output)"
    Test-Expectation -Description 'a program the installer runs: the caller''s console input encoding is as it was' `
        -Condition ($probe.Output -match '(?m)^input encoding after: 65001, preamble 3\s*$') `
        -Detail "exit $($probe.ExitCode)`n$($probe.Output)"

    # install.ps1's own case: characters a comparison by culture ignores, -ceq's included, so only an ordinal comparison
    # tells these names from the archive's and the checksums file's. The soft hyphen and the zero-width space go inside
    # the name, after "devlore-cli"; the NUL goes at its end.
    $softHyphen = [string][char]0x00AD
    $zeroWidthSpace = [string][char]0x200B
    $nul = [string][char]0
    $invisibles = @(
        [pscustomobject]@{
            Character = 'a soft hyphen'
            Account   = 'soft-hyphen'
            Archive   = $fixtureArchiveName.Insert(11, $softHyphen)
            Checksums = $fixtureChecksumsName.Insert(11, $softHyphen)
        }
        [pscustomobject]@{
            Character = 'a zero-width space'
            Account   = 'zero-width-space'
            Archive   = $fixtureArchiveName.Insert(11, $zeroWidthSpace)
            Checksums = $fixtureChecksumsName.Insert(11, $zeroWidthSpace)
        }
        [pscustomobject]@{
            Character = 'a trailing NUL'
            Account   = 'trailing-nul'
            Archive   = $fixtureArchiveName + $nul
            Checksums = $fixtureChecksumsName + $nul
        }
    )

    foreach ($invisible in $invisibles) {
        $own = "install.ps1's own case: $($invisible.Character)"
        $result = Invoke-FixtureInstall -Name "own-line-$($invisible.Account)" `
            -Line $neighbor, "$fixtureHash  $($invisible.Archive)"
        Test-Refusal -Case "$own in the line's name" -Expected 'refused, as 2' -Result $result -Message $noLine `
            -Requests $checksumsOnly -Id NoChecksum

        $result = Invoke-FixtureInstall -Name "own-archive-$($invisible.Account)" -Line $neighbor, $listed `
            -ArchiveAs $invisible.Archive
        Test-Refusal -Case "$own in the archive's name" -Expected 'refused, as 9' -Result $result -Message $noArchive `
            -Requests $both -Id DownloadRefused

        $result = Invoke-FixtureInstall -Name "own-checksums-$($invisible.Account)" -Line $neighbor, $listed `
            -ChecksumsAs $invisible.Checksums
        Test-Refusal -Case "$own in the checksums file's name" -Expected 'refused, as 1' -Result $result `
            -Message $noChecksums -Requests $checksumsThenRelease -Id DownloadRefused
    }

    # --- Run as a file: a failure exits 1, -Help exits 0 ---

    $prefix = Use-Account -Name 'file'
    $env:DEVLORE_VERSION = $fixtureMissingTag

    try {
        $failed = Invoke-Child -Name 'file-failure' -ArgumentList @(
            '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', "`"$installer`"", '-Prefix',
            "`"$prefix`"")
    }
    finally {
        $env:DEVLORE_VERSION = $savedVersion
    }

    Test-Expectation -Description 'as a file: a failure exits 1' -Condition ($failed.ExitCode -eq 1) `
        -Detail "exit $($failed.ExitCode)`n$($failed.Output)"
    $helped = Invoke-Child -Name 'file-help' -ArgumentList @(
        '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', "`"$installer`"", '-Help')
    Test-Expectation -Description 'as a file: -Help exits 0' -Condition ($helped.ExitCode -eq 0) `
        -Detail "exit $($helped.ExitCode)`n$($helped.Output)"

    # --- Every run removes its download directory (#1051) ---
    #
    # Whether it installed or was refused, each run of the installer removes its download directory: #958 fixed
    # install.sh leaving its own behind, and only a test keeps either installer from doing it. An account several
    # fixture runs share is checked after each, by its run's number.

    $runsOf = @{}

    foreach ($run in $script:fixtureRuns) {
        $runsOf[$run.Account] = 1 + [Int32] $runsOf[$run.Account]
        Test-Expectation `
            -Description "$($run.Account), run $($runsOf[$run.Account]): leaves no download directory behind" `
            -Condition ($run.Leftovers.Count -eq 0) -Detail "left: $($run.Leftovers -join ', ')"
    }

    foreach ($account in 'failure', 'file') {
        $left = @(Get-Leftover -Account $account)
        Test-Expectation -Description "${account}: leaves no download directory behind" `
            -Condition ($left.Count -eq 0) -Detail "left: $($left -join ', ')"
    }

    # --- irm | iex with $env:DEVLORE_BASE, in a child session: it installs into ~/.local, so CI alone runs it ---
    #
    # The published release, from GitHub: the child session is beyond the faux channel's stand-ins. What it checks, base
    # registered and the session alive, holds whichever writ it installs (#1031).

    if ($env:GITHUB_ACTIONS -eq 'true') {
        $null = Use-Account -Name 'iex'
        $command = @"
`$ErrorActionPreference = 'Continue'
`$env:DEVLORE_BASE = '$repo'
Get-Content -Raw -LiteralPath '$installer' | Invoke-Expression
"after: session alive; error action `$ErrorActionPreference"
"@
        $iex = Invoke-Child -Name 'iex' -ArgumentList @(
            '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-EncodedCommand',
            (ConvertTo-EncodedCommand -Command $command))
        $iexWrit = Join-Path (Join-Path (Join-Path $HOME '.local') 'bin') $writName
        $baseRoot = Get-LayerRoot -Writ $iexWrit -Layer 'base'
        Test-Expectation -Description ('irm | iex with $env:DEVLORE_BASE: base registered, the session goes on, its ' +
            'error action unchanged') `
            -Condition ($baseRoot -eq $repo -and $iex.Output -match 'after: session alive; error action Continue') `
            -Detail "base: $baseRoot; exit $($iex.ExitCode)`n$($iex.Output)"
    }
    else {
        Write-Output -InputObject 'SKIP irm | iex: it installs into ~/.local, so it runs in CI alone'
    }
}
catch {
    $caught = $_
    $ErrorActionPreference = $callerErrorAction
    $PSCmdlet.WriteError($caught)
    exit 1
}
finally {
    $env:DEVLORE_VERSION = $savedVersion
    $env:GH_TOKEN = $savedToken

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
    }
    catch {
        Write-Warning "cleanup: $($_.Exception.Message)"
    }
}

if ($script:failures -gt 0) {
    Write-Output -InputObject "$($script:failures) check(s) failed"
    exit 1
}

Write-Output -InputObject 'every check passed'
