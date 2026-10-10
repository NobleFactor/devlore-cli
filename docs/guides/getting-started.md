---
title: "Getting Started"
description: "Install DevLore and deploy your first environment"
tool: "devlore"
category: "tutorial"
order: 1
---

# Getting Started with DevLore

DevLore is a suite of three tools for managing portable development environments.
**Writ** orchestrates your portable environment through platform-aware symlinks, decryption, and template expansion.
**Lore** handles software installation by capturing tribal knowledge about packages.
**Star** runs Starlark-powered operations, defined as extensions.

Together, they let you declare your environment once and deploy it everywhere you work.

## What you'll learn

- Install lore, star and writ
- Upgrade them, and choose which builds they follow
- Create an environment repository
- Deploy your first project
- Install software from a manifest

## Install

One command installs lore, star and writ, and registers the layers you give it. The installers are served by
the DevLore site's develop environment, from which devlore is released today.

On macOS and Linux:

```bash
curl --fail --silent --show-error --location \
  https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net/install.sh |
  bash -s -- --base=<path-or-url> --team=<path-or-url> --personal=<path-or-url>
```

On Windows, in Windows PowerShell 5.1 or PowerShell 7, the preferred form:

```powershell
& ([scriptblock]::Create((irm https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net/install.ps1))) `
    -Base <path-or-url> `
    -Team <path-or-url> `
    -Personal <path-or-url>
```

Or through `irm | iex`, which takes no parameters, so the layers come from the environment. This form leaves
the installer's settings in your PowerShell session
([#982](https://github.com/NobleFactor/devlore-cli/issues/982)); the form above leaves nothing behind.

```powershell
$env:DEVLORE_BASE = '<path-or-url>'
$env:DEVLORE_TEAM = '<path-or-url>'
$env:DEVLORE_PERSONAL = '<path-or-url>'
irm https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net/install.ps1 | iex
```

| bash | PowerShell | Environment | Layer |
| --- | --- | --- | --- |
| `--base <loc>` or `--base=<loc>` | `-Base <loc>` | `DEVLORE_BASE` | base |
| `--team <loc>` or `--team=<loc>` | `-Team <loc>` | `DEVLORE_TEAM` | team |
| `--personal <loc>` or `--personal=<loc>` | `-Personal <loc>` | `DEVLORE_PERSONAL` | personal |

- Each is optional. A location is a working-tree root or a repository URL, as `writ repo set` takes it; an
  SSH URL clones over SSH. A flag wins over its variable. In bash, each flag takes its value after a space or an
  `=`.
- The installer never asks. A layer you don't give is skipped, and the run's last lines name the
  `writ repo set` command that registers it later. A layer writ already has is not skipped: the
  summary names it as already registered.
- It installs into `~/.local` on every platform: the programs in `~/.local/bin`, with their man pages,
  completions and star's extensions. `--prefix` (`-Prefix`) changes that. `DEVLORE_VERSION` installs a
  particular release, and `DEVLORE_TOOLS` one program. `GH_TOKEN`, if you set it, lifts GitHub's limit of 60
  anonymous API requests an hour.
- Running the command again is safe. It's also how to recover from a failure.

### By hand

The installer does five things you can do yourself. Pick a release from
[the releases page](https://github.com/NobleFactor/devlore-cli/releases); every file's name carries its tag.

```bash
tag=v0.1.0-dev.20260929224715
platform=linux_arm64    # darwin_amd64, darwin_arm64, linux_amd64 or linux_arm64
archive=devlore-cli_${tag}_${platform}.tar.gz
release=https://github.com/NobleFactor/devlore-cli/releases/download/$tag

# 1. Download the archive and the checksums file
curl --fail --silent --show-error --location --remote-name "$release/$archive"
curl --fail --silent --show-error --location --remote-name "$release/devlore-cli_${tag}_checksums.txt"

# 2. Verify the archive against the line that names it exactly (on macOS: shasum --algorithm 256 --check)
awk -v archive="$archive" '$2 == archive' "devlore-cli_${tag}_checksums.txt" | sha256sum --check

# 3. Extract it, with the programs in bin/ beside share/, where star finds its extensions
mkdir -p devlore/bin
tar --extract --gzip --file "$archive" --directory devlore
mv devlore/lore devlore/star devlore/writ devlore/bin/

# 4. Let each program install itself into ~/.local
for program in lore star writ; do devlore/bin/$program self install; done

# 5. Register your layers
writ repo set personal <path-or-url>
```

On Windows the archive is `devlore-cli_<tag>_windows_amd64.zip` or `…_windows_arm64.zip`:

```powershell
$tag = 'v0.1.0-dev.20260929224715'
$archive = "devlore-cli_${tag}_windows_amd64.zip"
$checksums = "devlore-cli_${tag}_checksums.txt"
$release = "https://github.com/NobleFactor/devlore-cli/releases/download/$tag"

Invoke-WebRequest -Uri "$release/$archive" -OutFile $archive -UseBasicParsing
Invoke-WebRequest -Uri "$release/$checksums" -OutFile $checksums -UseBasicParsing
$expected = ((Get-Content $checksums | Where-Object { ($_ -split '\s+')[1] -ceq $archive }) -split '\s+')[0]
if (-not $expected) { throw "$checksums has no line for $archive" }
if ((Get-FileHash -Path $archive -Algorithm SHA256).Hash -ne $expected) { throw "Checksum mismatch for $archive" }

Expand-Archive -Path $archive -DestinationPath devlore
New-Item -ItemType Directory -Path devlore\bin -Force | Out-Null
Move-Item -Path devlore\lore.exe, devlore\star.exe, devlore\writ.exe -Destination devlore\bin
foreach ($program in 'lore', 'star', 'writ') { & "devlore\bin\$program.exe" self install }

writ repo set personal <path-or-url>
```

## Upgrade

One command upgrades lore, star and writ together. Run it from any of the three; `writ` here stands for whichever you
use:

```bash
writ self upgrade
```

It upgrades every devlore program installed in the same place as the one you run (`~/.local`, unless you gave the
installer `--prefix`) to one release. It finds the newest build on your channel, downloads it from GitHub, checks it
against the release's published checksums, and has each program install itself from it. A program you didn't install
there isn't added. When everything is already current it changes nothing, so running it again is always safe, and
it's also how to finish a run that failed partway. `writ self upgrade --dry-run` says what it would do and changes
nothing.

A **channel** is the stream of builds an upgrade follows, and there are two:

| Channel | What `writ self upgrade` installs |
| --- | --- |
| `develop` | the newest build from the `develop` branch, where work lands first |
| `release` | the newest release; with pre-releases, the newest release candidate when one is newer |

A **pre-release** is a build GitHub publishes marked as not yet a release. Every `develop` build is one, and so is a
**release candidate**, a build published ahead of a release so it can be tried first. Each build carries the channel
it was made on, and `writ --version` names it after the version:

```text
writ version v0.1.0-dev.<stamp> (develop), build <commit>
```

Today every published build is a `develop` build, so that is the channel the installer gives you, and `release` has
no release to upgrade to yet.

On Windows a program can't be overwritten while it runs, so the upgrade sets the one you ran aside, as
`writ.exe.old` beside the new `writ.exe`; a later upgrade or install removes it once nothing runs it.

### Switch channels

`--channel` switches, and the switch sticks: the builds it installs carry the new channel, so a plain
`writ self upgrade` afterwards stays on it, unless you set `self.channel` (below), which outranks it.

```bash
writ self upgrade --channel release                 # releases only
writ self upgrade --channel release --prerelease    # releases and their release candidates
writ self upgrade --channel develop                 # back to every develop build
```

To make the choice a standing setting rather than a property of the installed build, set it in the configuration
lore, star and writ share:

```bash
writ config set self.channel=release self.prerelease=true
```

`self.channel` outranks the channel a build carries, and `--channel` outranks both, for the run it's given to.
`self.prerelease` is read only beside `self.channel`. `--prerelease` lasts only while the build it installs is itself
a pre-release; `self.prerelease=true` keeps release candidates coming after a regular release is installed.
`writ config unset self.channel` hands the choice back to the build, and `writ config unset self.prerelease` too if
you set it: `config unset` refuses a key that isn't set, and then changes nothing.

### A particular release, or an archive you have

`DEVLORE_VERSION`, as the installer reads it, upgrades to one particular release, whatever its channel:

```bash
DEVLORE_VERSION=<tag> writ self upgrade
```

While it's set, a plain `writ self upgrade` keeps installing that release, so leave it unset otherwise.

`--from` installs an archive you downloaded, as under [By hand](#by-hand), without asking GitHub. The release's
checksums file must be beside it, under its own name, `devlore-cli_<tag>_checksums.txt`:

```bash
writ self upgrade --from ./devlore-cli_<tag>_linux_arm64.tar.gz
```

`GH_TOKEN`, if you set it, lifts GitHub's limit of 60 anonymous API requests an hour, as it does for the installer.
Nothing prompts.

### If your build can't upgrade itself

Builds published before [#947](https://github.com/NobleFactor/devlore-cli/issues/947) fetch nothing: their
`self upgrade` reinstalls the build you already have. If `writ --version` names no channel after the version, run the
installer again, as under [Install](#install); the build it installs upgrades itself from then on.

## Initialize a repository

A writ repository is a directory containing your environment organized into projects.
Each project is a subdirectory whose files get deployed to your home directory.

```bash
# Create your environment directory
mkdir -p ~/my-environment

# Register it with writ
writ config set writ.repos.personal ~/my-environment
```

## Create your first project

A project is simply a directory in your repository. Files inside it mirror
the structure of your home directory:

```bash
cd ~/my-environment
mkdir -p noblefactor/Home/.config/git

# Move your existing gitconfig into the project
mv ~/.config/git/config noblefactor/.config/git/config

# Add more files
cp ~/.zshrc noblefactor/.zshrc
```

## Deploy the project

```bash
writ deploy noblefactor
```

Writ creates symlinks from your home directory to the project files:

```
~/.zshrc → repos/personal/noblefactor/.zshrc
~/.config/git/config → repos/personal/noblefactor/.config/git/config
```

## Check status

```bash
writ reconcile noblefactor
```

```
noblefactor (personal)
  ✓ Linked  .zshrc
  ✓ Linked  .config/git/config
```

## Add software with lore

If your project includes a `packages-manifest.yaml` file, writ automatically
delegates to lore for software installation:

```yaml
# noblefactor/packages-manifest.yaml
packages:
  - gh
  - jq
  - ripgrep
  - neovim:
      with: [lsp]
```

```bash
writ deploy noblefactor
# → symlinks configuration files
# → calls lore to install gh, jq, ripgrep, neovim
```

See [Packages Manifest](/guides/writ/packages-manifest/) for the full format reference.

Or install packages directly with lore:

```bash
lore deploy gh jq ripgrep neovim
```

## Next steps

- [Manage environments](/guides/writ/manage-environments/) — Learn conflict handling, removal, and upgrades
- [Platform awareness](/guides/writ/platform-awareness/) — Configure platform-specific variants
- [Secrets](/guides/writ/#secrets) — Deploy sops-encrypted files
- [Deploy packages](/guides/lore/deploy-packages/) — Use lore's four-phase pipeline
- [Create manifests](/guides/lore/create-manifests/) — Package tribal knowledge for sharing
