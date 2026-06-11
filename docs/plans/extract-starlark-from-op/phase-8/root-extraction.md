---
title: extract op.Root → pkg/root
status: planned
created: 2026-06-11
updated: 2026-06-11
---

## Task

Move `op.Root` — and its companions `Path`, `RootReaderWriter`, `rootBase` — out of `pkg/op` into a standalone
**`pkg/root`** package. `root.go` already depends on nothing in `op` (stdlib + `yaml` only, zero `op` type
references), so the code **lifts out untouched**; the work is the *consumer sweep* (`op.Root` → `root.Root` across
every provider, the executor, `graph.go`, …). Its own focused PR.

**Sequence:** after the encrypt work (`EncryptFile`), before `pkg/signing`. This is the family of self-contained
capabilities already pulled out of `op` in 13.0(i) (`pkg/status`, `pkg/result`, `pkg/platform`, `pkg/process`,
`pkg/sink`); `Root` is the same shape.

## Shape

- **`root.Open(dir string) (root.Root, error)`** — the constructor; allocates a `root.root` (unexported impl).
- **`root.root`** — the impl. It wraps a stdlib **`os.Root`** (Go 1.24 confined-FS) for path-escape-safe operations
  and **adds the full-path accessor `Name()`** that `os.Root` deliberately hides.
- **`root.Root`** — the interface, the surface `op.Root` exposes today: `Name`, `NewPath`, `Open`, `OpenFile`,
  `ReadFile`, `WriteFile`, `Stat`, `Lstat`, `Remove`, `Rename`, `Symlink`, `Readlink`, `MkdirAll`.

## Payoff

`pkg/sops` (the `Encrypter`) and the gitignore-based `.sops.yaml` walk then take a typed, confined `root.Root`
instead of the bare `string` boundary — `NewPath`/`ReadFile` bounded to the root, no `..`-escape. At that point the
`Encrypter` signature upgrades `Encrypt(data, sourcePath, rootDir string)` →
`Encrypt(data []byte, sourcePath string, root root.Root)` (a one-line change plus its call site in
`encryption.Provider.EncryptFile`).

## Status

- **Planned** — not started. Follows `EncryptFile`; precedes `pkg/signing`.
