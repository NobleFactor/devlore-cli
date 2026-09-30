# DevLore

**New machine to productive developer in minutes.** DevLore sets up real
developer machines — pick a role like "Azure Cloud developer" or "Apple Mobile
App developer" and the tools arrive installed, configured, and verified the way
your team actually uses them. It works on the physical machine (macOS, Linux,
and Windows natively), which is exactly where devcontainers, cloud IDEs, and
VM-based tooling stop.

Software installation is the visible tip. The value is everything under the
waterline: the post-install steps nobody wrote down. Deploying Docker on a
fresh Linux box, for example, means removing conflicting packages, adding the
vendor repository, installing five packages in order, configuring group
membership, setting up rootless mode, generating shell completions, and
verifying with a hello-world run — DevLore captures that whole sequence as an
executable, verifiable package, not a wiki page.

## The tools

| Binary | Purpose |
|--------|---------|
| `lore` | Deploys software with its tribal knowledge: prepare → install → provision → verify, with receipts recording what actually happened |
| `writ` | Manages your environment: dotfiles, configuration layers, drift detection, and reconciliation as role definitions evolve |
| `star` | Runs Starlark-powered operations, defined as extensions |

All three are single native binaries with man pages and completions for bash,
zsh, fish, and PowerShell. The CLI surface is being unified under the `devlore`
name; `lore`, `star` and `writ` are the current entry points.

## Install

One command installs lore, star and writ, and registers the layers you give it.
The installers are served by the DevLore site's develop environment, from which
devlore is released today.

On macOS and Linux:

```bash
curl --fail --silent --show-error --location https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net/install.sh |
  bash -s -- --base=<path-or-url> --team=<path-or-url> --personal=<path-or-url>
```

On Windows, in Windows PowerShell 5.1 or PowerShell 7:

```powershell
& ([scriptblock]::Create((irm https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net/install.ps1))) `
    -Base <path-or-url> `
    -Team <path-or-url> `
    -Personal <path-or-url>
```

Every flag is optional, and the installer never asks. The
[getting-started guide](docs/guides/getting-started.md) describes the flags,
the `irm | iex` form, and the install by hand. To build from source, see
[Building](#building).

Homebrew and MacPorts packaging are staged in [`packaging/`](packaging/) and
will ship with the first tagged release.

## Cross-platform, genuinely

macOS, Linux, and Windows are first-class targets — including a native
PowerShell provider on Windows, not a WSL shim. One manifest describes a role;
each platform deploys it with its native package managers (Homebrew, MacPorts,
apt, dnf, winget, and more) plus the provisioning steps those managers don't
do.

## The registry

Packages live in [devlore-registry](https://github.com/NobleFactor/devlore-registry) —
the curated catalog of deployment knowledge: lifecycle manifests, per-platform
phase scripts, and the knowledge assets that let AI assistants author and
validate packages. Content is served from GitHub today; OCI distribution
(point DevLore at the registry you already run) is the planned path.

## Building

**Prerequisites:** Go 1.26+ and **GNU make 3.82+**.

macOS ships GNU make 3.81 — the last GPLv2 release, so it will never advance — and the build uses
`.ONESHELL:`, which 3.82 introduced. Older make ignores that directive silently and fails with
`syntax error: unexpected end of file`, so the Makefile refuses to run on it and says why:

```bash
brew install make
export PATH="$(brew --prefix)/opt/make/libexec/gnubin:$PATH"   # or invoke gmake
```

```bash
make build   # Build binaries to bin/
make test    # Run the test suite
```

All paths follow the
[XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html);
see [docs/](docs/) for architecture and guides.

## Contributing

Contributions arrive under Apache-2.0 §5 with a
[Developer Certificate of Origin](https://developercertificate.org/) sign-off
(`git commit -s`). See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).

---

DevLore is a [Noble Factor](https://github.com/NobleFactor) project.
