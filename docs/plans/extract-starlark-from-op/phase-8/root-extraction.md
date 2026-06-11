---
title: extract op.Root → pkg/fsroot
status: complete
created: 2026-06-11
updated: 2026-06-11
---

## Task

Move `op.Root` — and its companions `Path`, `rootBase`, and the confined/unconfined root impls — out of `pkg/op` into
a standalone **`pkg/fsroot`** package. `root.go` already depended on nothing in `op` (stdlib + `yaml` only, zero `op`
type references), so the code **lifted out untouched**; the work was the *consumer sweep* (`op.Root` → `fsroot.Root`
across every provider, the executor, `graph.go`, …). Its own focused PR.

The package is named **`fsroot`**, not `root`: `root.Root` stutters, and — worse — a `root` package collides with the
ubiquitous `root` local variable (a `root` var shadows a `root` package, breaking `root.Path`/`root.NewPath` resolution
wherever both meet). `fsroot` keeps the type name `Root` (so `fsroot.Root`) while leaving every `root` variable alone.

**Sequence:** after the encrypt work (`EncryptFile`), before `pkg/signing`. This is the family of self-contained
capabilities already pulled out of `op` in 13.0(i) (`pkg/status`, `pkg/result`, `pkg/platform`, `pkg/process`,
`pkg/sink`); `Root` is the same shape.

## Shape

- **`fsroot.OpenConfined(dir string) (fsroot.Root, error)`** — the confined constructor; wraps a stdlib **`os.Root`**
  (Go 1.24 confined-FS) for path-escape-safe I/O, via the unexported `confinedRoot`.
- **`fsroot.OpenUnconfined(dir string) fsroot.Root`** / **`fsroot.OpenWritableUnconfined(dir string) fsroot.Root`** —
  the unconfined read-only / read-write constructors (`unconfinedRootReader` / `unconfinedRootReaderWriter`), operating
  on absolute paths through `os.*`.
- **`fsroot.Root`** — the interface, the surface `op.Root` exposed: `Name`, `FS`, `Close`, `NewPath`, `Open`,
  `OpenFile`, `ReadFile`, `WriteFile`, `Stat`, `Lstat`, `Remove`, `Rename`, `Symlink`, `Readlink`, `MkdirAll`. It
  **adds the full-path accessor `Name()`** that `os.Root` deliberately hides.
- **`fsroot.Path`** — the path handle (relative + absolute), minted by `fsroot.NewPath` or `Root.NewPath`.

## Payoff

`pkg/sops` (the `Encrypter`) and its `pkg/sops`-local `.sops.yaml` `locate` walk can now take a typed, confined
`fsroot.Root` instead of the bare `string` boundary — `NewPath`/`ReadFile` bounded to the root, no `..`-escape. That
upgrade is the one remaining follow-up: `Encrypt(data, sourcePath, rootDir string)` →
`Encrypt(data []byte, sourcePath string, root fsroot.Root)` (a one-line change plus its call site in
`encryption.Provider.EncryptFile`).

## Status

- **Complete** — extracted to `pkg/fsroot`; consumer sweep done; **`go test ./pkg/...` passes (46 ok, 0 fail)** — the
  real bar (a prior `go build` check compiled no `_test.go` files and hid test breakage). Surfaced and fixed: the
  read-only sentinel moved with `Root` and is now the standard `errors.ErrUnsupported` (op-side `export_test.go`
  re-export deleted); `root.`-qualifier sweep misses in tests; and collateral from a too-broad `root`→`fsroot` text
  replace (a graph subgraph ID, a CLI flag, file-provider error strings, generated files) — reverted, with the file
  provider's `*.gen.go` regenerated via `make generate`. Doc-comment godoc links (`[op.Root]` → `[fsroot.Root]`) swept.
  Only the `Encrypter` `string` → `fsroot.Root` boundary upgrade (Payoff) is deferred.
