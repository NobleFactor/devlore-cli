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
    error record. A refusal is checked for install.sh's sentence, exactly, from the same tags, and every case for each
    request the installer made of GitHub, in order, and for those it sent GH_TOKEN with. A refusal runs in this
    session, so its exit status is the error Write-Fatal throws; run as a file, that error exits 1, which 'as a file: a
    failure exits 1' checks.

    Three checks run against GitHub itself, so that what the stand-ins model is what this edition's cmdlets do: a
    release GitHub does not have, the name GitHub's download host says it served, and a file a release lacks.

    Prints PASS or FAIL for each check, with what the installer printed under each failure, and exits 1 if any
    check failed. Run by .github/workflows/installers.yaml on Windows, under both editions (#950, #965).

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

# Initialize-FixtureArchive: the faux channel's archive for this platform (#1031): this checkout's lore, star and writ,
# as `make dist` builds and packs them for a release, named for the fixtures' tag, so the installer under test installs
# the programs under test. In CI, one job builds them for every platform, off Windows, and $env:DEVLORE_TEST_DIST names
# them. Without it, off Windows, this suite builds this platform's; on Windows it refuses, since Windows builds
# nothing and needs neither tar nor zip. Returns the archive's path.
function Initialize-FixtureArchive {
    [CmdletBinding()]
    [OutputType([string])]
    param()

    $dist = $env:DEVLORE_TEST_DIST

    if (-not $dist) {
        if ($onWindows) {
            throw ('set $env:DEVLORE_TEST_DIST to a directory holding what `make dist DEVLORE_VERSION=' +
                "$fixtureTag`` made, off Windows: Windows builds no archive")
        }

        $platform = "$fixtureOs/$fixtureArch"
        Write-Information -InformationAction Continue `
            "Building the faux channel: make dist PLATFORM=$platform DEVLORE_VERSION=$fixtureTag"
        $log = Join-Path $scratch 'dist.log'

        # Continue, not Stop: PowerShell makes each line a native command writes to stderr an error record when it
        # carries the stream, as it does here. The exit status is what says whether it built.
        $ErrorActionPreference = 'Continue'
        & make -C $repo dist "PLATFORM=$platform" "DEVLORE_VERSION=$fixtureTag" *> $log

        if ($LASTEXITCODE -ne 0) {
            throw "make dist exited ${LASTEXITCODE}:`n$(Get-Content -Raw -LiteralPath $log)"
        }

        $dist = Join-Path $repo 'dist'
    }

    $archive = Join-Path $dist $fixtureArchiveName

    if (-not (Test-Path -LiteralPath $archive -PathType Leaf)) {
        throw "$dist has no ${fixtureArchiveName}: it holds no make dist DEVLORE_VERSION=$fixtureTag for this platform"
    }

    return $archive
}

# Get-GitHubRefusal: the error record this edition's web cmdlets throw when GitHub refuses a request, carrying the
# status, the content type, the headers and the body where they carry them. GitHub's API refuses with its JSON, the
# message under "message", and -Documentation is its documentation_url; GitHub's download host refuses with the message
# alone, in plain text. -Header is any more headers GitHub sends, as its API sends x-ratelimit-* with a rate limit. PowerShell 7 throws an HttpResponseException holding the response, with the body in ErrorDetails, JSON
# re-indented after a newline. Windows PowerShell 5.1 throws a WebException holding the response, with the body as
# sent; the response here is a DevLoreFixtureResponse, which carries the status and the content type as an
# HttpWebResponse does. So the installer reads GitHub's status and message from these stand-ins on both editions, and
# the same sentence is checked on both.
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

# Get-ClockTime: a time in seconds since the epoch as this machine's clock shows it, HH:mm, whatever the culture.
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

# Get-LookAlikeName: the name given, but for an X in place of its last dot: a name a pattern would take for it (#1002).
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

# Invoke-FixtureInstall: runs the installer in this session against the fixture release. The release carries the files
# of -Before first, by name and file, then, when -Line is given, a checksums file of those lines (under -ChecksumsAs,
# default its own name, written in -Encoding, default UTF-8 with no byte-order mark), then an archive for every
# platform: this platform's under -ArchiveAs, default its own name, or none with -NoArchive. GitHub lists the files
# named in -Refuse but answers their links with 404. -Lookup says how the installer finds the release: Tag, the
# fixture's tag in DEVLORE_VERSION; NoSuchTag, a tag GitHub has no release for in DEVLORE_VERSION; Latest, NoRelease or
# RateLimited, no DEVLORE_VERSION, and GitHub answers the newest-release lookup with the fixture release, with no
# release or with its rate limit, which resets 23 minutes after it is asked. GH_TOKEN is -Token, or unset. Returns the
# message the installer threw (empty when it finished), the stack it threw from, every address it asked GitHub for, in
# order, those it sent GH_TOKEN with, everything it printed, the scratch account's prefix, and the reset of the last
# rate limit GitHub answered with, in seconds since the epoch, or 0.
#
# The stand-ins for GitHub are functions defined here, so they shadow the cmdlets for this run and no other: a command
# is found by walking out from the scope that calls it, and the installer runs in a scope inside this one. Each answers
# only the request the installer makes, as it makes it, and anything else as GitHub answers an unknown address. The
# variables they read are named fixture*: a variable is found the same way, so one the installer also names would be
# the installer's.
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
        $Arguments = @{}
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

    # GitHub's API: the newest release, and a release by its tag, which only the fixture's tag names.
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
            throw "the stand-in for GitHub takes the installer's request, -UseBasicParsing and JSON accepted: $Uri"
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

    # The release's public links, as GitHub's download host answers them: the file whose name matches in any case,
    # named in the Content-Disposition of the response this edition's cmdlet returns with -PassThru, or 404 and "Not
    # Found" in plain text.
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
            throw ("the stand-in for GitHub takes the installer's request, -UseBasicParsing, -OutFile and -PassThru: " +
                $Uri)
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
    # Requirement 7).
    $output = New-Object -TypeName 'System.Collections.Generic.List[string]'
    $errorRecords = New-Object -TypeName 'System.Collections.Generic.List[string]'
    $message = ''
    $stack = ''
    $env:DEVLORE_VERSION = $fixtureTag

    if ($Lookup -ceq 'NoSuchTag') {
        $env:DEVLORE_VERSION = $fixtureMissingTag
    }
    elseif ($Lookup -cne 'Tag') {
        $env:DEVLORE_VERSION = ''
    }

    $env:GH_TOKEN = $Token

    try {
        & $block -Prefix $prefix @Arguments *>&1 | ForEach-Object {
            if ($_ -is [System.Management.Automation.ErrorRecord]) {
                $errorRecords.Add("$_")
            }
            $output.Add("$_")
        }
    }
    catch {
        $message = $_.Exception.Message
        $stack = $_.ScriptStackTrace
    }
    finally {
        $env:DEVLORE_VERSION = $savedVersion
        $env:GH_TOKEN = $savedToken
    }

    $reset = 0

    if ($fixtureResets.Count -gt 0) {
        $reset = $fixtureResets[$fixtureResets.Count - 1]
    }

    return [pscustomobject]@{
        Message  = $message
        Stack    = $stack
        Requests = $fixtureRequests.ToArray()
        Tokens   = $fixtureTokens.ToArray()
        Output   = $output -join "`n"
        Errors   = $errorRecords.ToArray()
        Prefix   = $prefix
        Reset    = $reset
    }
}

# Test-Refusal: two checks of a fixture run that must be refused. "<case>: <expected>" passes when the run failed
# through Write-Fatal, which run as a file exits 1, with exactly -Message, and stopped before it extracted or installed
# anything. "<case>: asks GitHub for ..." is Test-Request's, of the addresses in -Requests.
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
        [string[]]
        $Requests
    )

    $fatal = $Result.Stack.StartsWith('at Write-Fatal,')
    $extracted = $Result.Output.Contains('Extracting') -or (Test-Path -LiteralPath (Join-Path $Result.Prefix 'bin'))
    Test-Expectation -Description "${Case}: $Expected" `
        -Condition ($Result.Message -ceq $Message -and $fatal -and -not $extracted) `
        -Detail ("expected: $Message`nmessage:  $($Result.Message)`nthrown by Write-Fatal: $fatal; extracted or " +
            "installed: $extracted`n$($Result.Output)`nstack:`n$($Result.Stack)")
    Test-Request -Case $Case -Result $Result -Requests $Requests
}

# Test-Verified: two checks of a fixture run that must install. "<case>: <expected>" passes when the run finished, said
# it verified the checksum, and installed the writ it verified, whose version is the release's (#1031). "<case>: asks GitHub for ..." is Test-Request's: by
# default the checksums file's link, then the archive's.
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
        -Condition (-not $Result.Message -and $Result.Output.Contains('success: Checksum verified') -and
            $version.Contains($fixtureTag)) `
        -Detail "message: $($Result.Message)`nwrit --version: $version`n$($Result.Output)"
    Test-Request -Case $Case -Result $Result -Requests $Requests
}

# Get-RequestName: what an address the installer asked GitHub for is, in words, for the name of a check.
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

# Test-Request: PASS when the fixture run asked GitHub for exactly the addresses in -Requests, in that order: each file
# by its public link, the checksums file before the archive and the archive only once the checksums file has its line
# (Requirement 1), and GitHub's API only for the newest release's tag, or for the release when a link answered 404
# before the release was found (#1008). These are the requests install.sh makes in the same case. The check names each
# by what it is, so its name says what the installer may ask for.
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

# Test-GitHubItself: two of the installer's own readers of GitHub's answers, against GitHub itself on this edition, so
# that what the stand-ins model is what this edition's cmdlets give the installer. The newest release's checksums file,
# asked for in upper case, comes back under its own name, which Get-ServedName must read; and a file the release lacks
# is refused with 404 and "Not Found" in plain text, which Get-GitHubAnswer must report. The installer's functions are
# defined here from its source, and Main is not run: the installer has no hook, and run whole it never asks for a name
# it does not build.
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
        $headers['Authorization'] = "token $env:GH_TOKEN"
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

Write-Information -InformationAction Continue "PowerShell $($PSVersionTable.PSVersion) ($($PSVersionTable.PSEdition))"

$block = [scriptblock]::Create((Get-Content -Raw -LiteralPath $installer))
$savedVersion = $env:DEVLORE_VERSION
$savedToken = $env:GH_TOKEN

try {
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
        Test-Expectation -Description "script-block form: run ${run}, every stream redirected, carries no error record" `
            -Condition ($result.Errors.Count -eq 0) `
            -Detail "error records:`n$($result.Errors -join "`n")`n----`n$($result.Output)"
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
    $env:DEVLORE_VERSION = $fixtureMissingTag
    $message = ''
    try {
        & $block -Prefix $prefix
    } catch {
        $message = $_.Exception.Message
    } finally {
        $env:DEVLORE_VERSION = $savedVersion
    }
    Test-Expectation -Description 'a forced failure: throws its message, and the next statement runs' `
        -Condition ($message.Contains("Could not fetch release $fixtureMissingTag of NobleFactor/devlore-cli")) `
        -Detail "message: $message"

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
        -Result $result -Message $noChecksums -Requests $checksumsThenRelease

    $result = Invoke-FixtureInstall -Name 'case-2' -Line $neighbor
    Test-Refusal -Case 'case 2: no line for the archive' -Result $result -Message $noLine -Requests $checksumsOnly `
        -Expected 'refused, naming the archive and the checksums file'

    $result = Invoke-FixtureInstall -Name 'case-3' -Line $neighbor, "$fixtureHash  ${fixtureArchiveName}.sig"
    Test-Refusal -Case 'case 3: a line only for <archive>.sig' -Expected 'refused, as 2' `
        -Result $result -Message $noLine -Requests $checksumsOnly

    $pattern = Get-LookAlikeName -Name $fixtureArchiveName
    $result = Invoke-FixtureInstall -Name 'case-4' -Line $neighbor, "$fixtureHash  $pattern"
    Test-Refusal -Case 'case 4: a line that matches only as a pattern, a dot replaced' -Expected 'refused, as 2' `
        -Result $result -Message $noLine -Requests $checksumsOnly

    $result = Invoke-FixtureInstall -Name 'case-5' -Line $neighbor, "$fixtureHash  $upper"
    Test-Refusal -Case 'case 5: a line naming the archive in another case' -Expected 'refused, as 2' `
        -Result $result -Message $noLine -Requests $checksumsOnly

    $mismatch = 'ab' * 32
    $result = Invoke-FixtureInstall -Name 'case-6' -Line $neighbor, "$mismatch  $fixtureArchiveName"
    Test-Refusal -Case 'case 6: a mismatch' -Expected 'refused, naming both hashes' -Result $result `
        -Message "Checksum verification failed!`nExpected: $mismatch`nActual:   $fixtureHash" -Requests $both

    $result = Invoke-FixtureInstall -Name 'case-7' -Line $neighbor, $listed -Refuse $fixtureChecksumsName
    Test-Refusal -Case 'case 7: the checksums file''s download refused (404)' -Result $result `
        -Expected 'refused, with GitHub''s message and the file''s name' -Requests $checksumsThenRelease `
        -Message $noChecksums

    $result = Invoke-FixtureInstall -Name 'case-8' -Line $neighbor, $listed -Refuse $fixtureArchiveName
    Test-Refusal -Case 'case 8: the archive''s download refused (404)' -Result $result `
        -Expected 'refused, with GitHub''s message and the archive''s name' -Requests $both -Message $noArchive

    # The checksums file's link has shown the release exists, so GitHub's API is asked nothing.
    $result = Invoke-FixtureInstall -Name 'case-9' -Line $neighbor, $listed -NoArchive
    Test-Refusal -Case 'case 9: no archive for this platform in the release' -Result $result -Message $noArchive `
        -Expected 'refused, naming the archive and the tag' -Requests $both

    $result = Invoke-FixtureInstall -Name 'case-10' -Line $neighbor, $listed -ArchiveAs $upper
    Test-Refusal -Case 'case 10: the archive published under a name in another case' -Expected 'refused, as 9' `
        -Result $result -Requests $both -Message ("Could not download $fixtureArchiveName from release " +
            "${fixtureTag}: GitHub served $upper, a file by another name")

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
        -Message 'Could not determine the latest release of NobleFactor/devlore-cli: GitHub lists none'

    # The rate limit says when to run again (Requirement 3b): the reset as a clock time and in minutes, and, only when
    # GH_TOKEN is unset, that setting it raises the limit.
    $rateLimited = "Could not list the releases of NobleFactor/devlore-cli: GitHub answered HTTP 403: $fixtureRateLimit"
    $result = Invoke-FixtureInstall -Name 'case-12-rate-limit' -Line $neighbor, $listed -Lookup RateLimited
    $clock = Get-ClockTime -Seconds $result.Reset
    Test-Refusal -Case 'case 12: a rate limit, on the latest-release lookup' -Result $result `
        -Expected 'refused, saying so, with GitHub''s message, when to run again, and that GH_TOKEN raises the limit' `
        -Requests $fixtureNewest -Message ("$rateLimited`nGitHub's API limit for this address is used up. It resets at " +
            "$clock (in 23 minutes); run the installer again after that. Setting GH_TOKEN raises the limit.")

    $result = Invoke-FixtureInstall -Name 'case-12-rate-limit-token' -Line $neighbor, $listed -Lookup RateLimited `
        -Token 'fixture-token'
    $clock = Get-ClockTime -Seconds $result.Reset
    Test-Refusal -Case 'case 12: a rate limit, with GH_TOKEN set' -Result $result `
        -Expected 'refused, saying so, with GitHub''s message and when to run again, and nothing of GH_TOKEN' `
        -Requests $fixtureNewest -Message ("$rateLimited`nGitHub's API limit for your token is used up. It resets at " +
            "$clock (in 23 minutes); run the installer again after that.")

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
            "${fixtureTag}: GitHub served $($fixtureChecksumsName.ToUpperInvariant()), a file by another name")

    # A byte-order mark is part of the first line, as self upgrade and sha256sum read it: its hash is not 64 hex
    # characters, so the archive's line behind it is no line.
    $result = Invoke-FixtureInstall -Name 'case-15' -Line $listed, $neighbor `
        -Encoding (New-Object -TypeName System.Text.UTF8Encoding -ArgumentList $true)
    Test-Refusal -Case 'case 15: the archive''s line behind a byte-order mark' -Expected 'refused, as 2' `
        -Result $result -Message $noLine -Requests $checksumsOnly

    # GitHub's API is asked for the release only after its checksums file's link answers 404.
    $result = Invoke-FixtureInstall -Name 'case-16' -Line $neighbor, $listed -Lookup NoSuchTag
    Test-Refusal -Case 'case 16: a pinned DEVLORE_VERSION that names no release' -Expected 'refused, naming the tag' `
        -Result $result -Message "Could not fetch release $fixtureMissingTag of NobleFactor/devlore-cli: $notFound" `
        -Requests "$fixtureDownload/$fixtureMissingTag/devlore-cli_${fixtureMissingTag}_checksums.txt",
        "$fixtureApi/releases/tags/$fixtureMissingTag"

    # GH_TOKEN goes to GitHub's API alone, which it lets past the anonymous limit, and never to a download link.
    $result = Invoke-FixtureInstall -Name 'token' -Line $neighbor, $listed -Lookup Latest -Token 'fixture-token'
    Test-Verified -Case 'the newest release, with GH_TOKEN set' -Expected '"Checksum verified", installs' `
        -Result $result -Requests $fixtureNewest, $fixtureChecksumsLink, $fixtureArchiveLink
    Test-Expectation -Description 'the newest release, with GH_TOKEN set: sends it to GitHub''s API alone' `
        -Condition (($result.Tokens -join "`n") -ceq $fixtureNewest) -Detail "sent with: $($result.Tokens -join ', ')"

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
            -Requests $checksumsOnly

        $result = Invoke-FixtureInstall -Name "own-archive-$($invisible.Account)" -Line $neighbor, $listed `
            -ArchiveAs $invisible.Archive
        Test-Refusal -Case "$own in the archive's name" -Expected 'refused, as 9' -Result $result -Message $noArchive `
            -Requests $both

        $result = Invoke-FixtureInstall -Name "own-checksums-$($invisible.Account)" -Line $neighbor, $listed `
            -ChecksumsAs $invisible.Checksums
        Test-Refusal -Case "$own in the checksums file's name" -Expected 'refused, as 1' -Result $result `
            -Message $noChecksums -Requests $checksumsThenRelease
    }

    # --- Run as a file: a failure exits 1, -Help exits 0 ---

    $prefix = Use-Account -Name 'file'
    $env:DEVLORE_VERSION = $fixtureMissingTag
    try {
        $failed = Invoke-Child -Name 'file-failure' -ArgumentList @(
            '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', "`"$installer`"", '-Prefix',
            "`"$prefix`"")
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
    #
    # The published release, from GitHub: the child session is beyond the faux channel's stand-ins. What it checks, base
    # registered and the session alive, holds whichever writ it installs (#1031).

    if ($env:GITHUB_ACTIONS -eq 'true') {
        $null = Use-Account -Name 'iex'
        $command = @"
`$env:DEVLORE_BASE = '$repo'
Get-Content -Raw -LiteralPath '$installer' | Invoke-Expression
'after: session alive'
"@
        $iex = Invoke-Child -Name 'iex' -ArgumentList @(
            '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-EncodedCommand',
            (ConvertTo-EncodedCommand -Command $command))
        $iexWrit = Join-Path (Join-Path (Join-Path $HOME '.local') 'bin') $writName
        $baseRoot = Get-LayerRoot -Writ $iexWrit -Layer 'base'
        Test-Expectation -Description 'irm | iex with $env:DEVLORE_BASE: base registered, the session goes on' `
            -Condition ($baseRoot -eq $repo -and $iex.Output -match 'after: session alive') `
            -Detail "base: $baseRoot; exit $($iex.ExitCode)`n$($iex.Output)"
    } else {
        Write-Information -InformationAction Continue `
            'SKIP irm | iex: it installs into ~/.local, so it runs in CI alone'
    }
} finally {
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
    } catch {
        Write-Warning "cleanup: $($_.Exception.Message)"
    }
}

if ($script:failures -gt 0) {
    Write-Information -InformationAction Continue "$($script:failures) check(s) failed"
    exit 1
}
Write-Information -InformationAction Continue 'every check passed'
