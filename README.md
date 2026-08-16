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

Both are single native binaries with man pages and completions for bash, zsh,
fish, and PowerShell. The CLI surface is being unified under the `devlore`
name; `lore` and `writ` are the current entry points.

## Install

```bash
# From source
go install github.com/NobleFactor/devlore-cli/cmd/lore@latest
go install github.com/NobleFactor/devlore-cli/cmd/writ@latest

# Or use the install scripts
./install.sh      # macOS, Linux
./install.ps1     # Windows

# Then let the tools finish their own setup (completions, man pages)
lore self install
writ self install
```

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

DevLore is a [Noble Factor](https://noblefactor.com) project. Project home:
[devlore.org](https://devlore.org).
