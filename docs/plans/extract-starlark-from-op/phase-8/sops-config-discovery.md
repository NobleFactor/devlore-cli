---
title: sops config discovery, anchoring, and resolution (git-style)
status: draft
created: 2026-06-10
updated: 2026-06-11
---

## Summary

`.sops.yaml` config drives **encryption only**; **decryption is config-free**; **signing has left this package** for
its own `pkg/signing` (no `.sops.yaml` — see below). `pkg/sops` is now pure getsops orchestration for secret
encrypt/decrypt: discovery is ours, crypto/resolution/validation are getsops's. For encryption we perform a
git-style, `Root`-bounded upward walk from the target file (per-directory `.sops.yaml` over an XDG global fallback),
then hand getsops a `(confPath, filePath)` — it loads the file, selects the matching creation rule, resolves
recipients, and validates. We find the files; getsops turns one into validated recipients.

## Status

| Item | ✅ | Notes |
|---|---|---|
| Package relocated `pkg/op/sops` → `pkg/sops` | ✅ | committed |
| Decrypt — config-free | ✅ | getsops `decrypt.DataWithFormat`; tested green |
| `pkg/sops` = getsops-only surface decided | ✅ | `Decrypt` free func + `Encrypter` (cache) + discovery; no `Client` / `NewClient(searchDir)` baggage |
| Encrypt — discovery + getsops-resolution design | ✅ | this doc |
| Encrypt — `sops.Encrypter` impl | ⬜ | not started |
| `encryption.Provider.EncryptFile` + `CompensateEncryptFile` | ⬜ | signature settled, not built |
| Encrypt tests | ⬜ | not started |
| Signing split to `pkg/signing` decided | ✅ | getsops has no signing (verified); separate concern |
| Signing key config independent of `.sops.yaml` decided | ✅ | its own config |
| `pkg/signing` — design | ◑ | **draft** — [`graph-signing.md`](graph-signing.md): data-layer signing, Ed25519/ECDSA; key-custody + trust-model questions open |
| `pkg/signing` — impl (`Sign`/`Verify`/`Signature`, stdlib over canonical bytes) | ⬜ | not started |

## Config consumers

- **Decrypt — config-free.** getsops reads the encrypted file's own embedded metadata plus ambient identities
  (`SOPS_AGE_KEY`, GPG, KMS); `.sops.yaml` is never consulted. The decrypt path must never trigger discovery.
- **Encrypt — per-file, config-driven.** (writ, via the encryption provider.) The file being encrypted has a path;
  its `path_regex`-matched creation rule supplies the recipients. This is getsops `LoadCreationRuleForFile`'s home
  ground — a real `filePath` to match and resolve against.
- **Sign — moved out.** Graph-provenance signing is no longer a sops concern; it lives in `pkg/signing` with its own
  key configuration (not `.sops.yaml`). getsops has no signing capability, so it could never have been getsops-backed.

## Discovery (ours)

Walk **up** from the target file's directory to `RuntimeEnvironment.Root`, collecting every `.sops.yaml`; then append
the XDG fallback `${XDG_CONFIG_HOME}/devlore/sops.yaml` (default `~/.config/devlore/sops.yaml`). Precedence is deepest
in-tree → shallower → fallback — git's `.gitignore`-over-`core.excludesFile` model (local overrides global).

getsops's own `FindConfigFile`/`LookupConfigFile` are **not** used: they walk to `maxDepth` rather than our `Root`, and
return a single file, not the ordered chain.

The walk uses the repo's **`pkg/gitignore/gitignore`** package for git-consistent path semantics, rather than a
hand-rolled `os.Stat` loop. The boundary is our own **`op.Root`** — the provider passes `root.Name()` (the root
directory) as the upper bound, so the walk stops there. This works because `op.Root` exposes the full path via
`Name()`; stdlib `os.Root` deliberately hides it, so the boundary string would be unavailable if `Root` were only
that. `pkg/sops` takes the boundary as a `string` for now (it cannot import `pkg/op`); after the `pkg/root` extraction
(see [`root-extraction.md`](root-extraction.md)) the `Encrypter` upgrades to a typed `root.Root`.

## Resolution (getsops)

For a target file, walk the discovered chain (deepest → fallback) and call, on each:

```go
cfg, err := config.LoadCreationRuleForFile(confPath, filePath, kmsEncryptionContext)
```

**First non-nil `*config.Config` wins.** Per call, getsops:

1. `os.ReadFile(confPath)` + `yaml.Unmarshal` the **whole** file into its unexported `configFile`
   (`creation_rules`, `destination_rules`, `stores`).
2. Selects the first creation rule whose `path_regex` matches `filePath` **relative to `dir(confPath)`**.
3. Resolves that rule's recipients (`age`/`pgp`/`kms`/`gcp_kms`/`azure_keyvault`/`hc_vault_transit_uri`, each
   string-or-list) into validated `[]sops.KeyGroup`, applying shamir threshold, key-group `merge`, and dedup.
4. Validates — e.g. the mutually-exclusive `encrypted_suffix`/`unencrypted_suffix`/`encrypted_regex`/… check.

It returns a public `config.Config`: `KeyGroups`, `ShamirThreshold`, the encrypted/unencrypted suffix-and-regex
settings, `MACOnlyEncrypted`. That struct is the encrypt input.

**What we mine:** the recipient resolution (security-critical, backend-specific — do not reimplement) and the schema
validation. **What we do not get:** a standalone validator (validation happens *inside* the load), and any
load-once-select-many path (`configFile` and `loadConfigFile` are unexported — every call re-reads and re-unmarshals
the whole file).

## Anchoring

getsops anchors `path_regex` at `dir(confPath)` — it strips that prefix off `filePath` before matching. For an in-tree
`.sops.yaml` that is correct (the config's own directory). The XDG fallback anchors at `~/.config/devlore`, **not
`Root`** — so the fallback must carry a **catch-all rule** (`path_regex: ""`), which matches regardless of anchor.

gitignore parallel: a per-directory `.gitignore` anchors at its directory; the global `core.excludesFile` anchors at
the worktree root. The `Root`-anchor "lie" is not reachable through getsops's public loader (`confPath` both loads and
anchors), which is why the fallback leans on a catch-all rather than path-specific rules.

## Cache

Cache the **walk**, not the parse — getsops re-reads and re-unmarshals the whole file on every
`LoadCreationRuleForFile`, and its `configFile` is unexported.

```go
type Client struct {
	mutex   sync.Mutex
	visited map[string][]string // start dir -> ordered config file paths, deepest .sops.yaml .. XDG fallback
}
```

- `visited` memoizes the upward walk (which files, in what order) per start directory; many directories under one
  subtree share a chain, so the walk runs once per subtree.
- Per-session, snapshot, no staleness check, no watcher, no `Closer` — the cache is in-memory, and every getsops
  backend resource (KMS clients, the file read, GPG subprocesses) is released per operation.
- Both maps mutex-guarded — encryption and signing run under `gather` concurrency.

**Known cost — bulk encryption.** writ encrypting many files re-reads and re-parses the same `.sops.yaml` per file,
because getsops exposes no load-once-select-many. Acceptable for now; revisit only if profiling demands it (the only
fix is to stop using getsops's loader and parse ourselves, forfeiting its resolution + validation).

## Implementation: `encryption.Provider.EncryptFile` (to build + test)

writ's encryption is a new compensable action on the encryption provider, mirroring `DecryptSopsFile`:

```go
// EncryptFile encrypts source's content for the SOPS recipients resolved from the .sops.yaml governing source's
// path and writes the encrypted document to destinationPath.
func (p *Provider) EncryptFile(source *file.Resource, destinationPath string) (*file.Resource, *Receipt, error)

// CompensateEncryptFile removes the encrypted file created by EncryptFile.
func (p *Provider) CompensateEncryptFile(receipt *Receipt) error
```

- Recipients, encrypted-field rules, and format come from the creation rule getsops resolves for `source`'s path —
  discovery walks up from `source` to `Root`, then the XDG fallback, and `LoadCreationRuleForFile` resolves the rule.
- Compensable pair (Appendix A): the forward method returns `(result, receipt, error)`; `CompensateEncryptFile`
  removes the ciphertext on undo — symmetric with `DecryptSopsFile` / `CompensateDecryptSopsFile`.
- **Status: not started** — needs implementation and tests.

## Signing — split into its own package (`pkg/signing`)

Signing leaves sops entirely. **Verified: getsops has no signing capability** — its `MasterKey` interface only
encrypts/decrypts the data key (no `Sign`), and there is no `Signature` type or `Sign` function anywhere in the
library. Signing a graph's canonical content for provenance is a *devlore* concern, not a SOPS one.

- **`pkg/signing`** owns `Sign` / `Verify` / `Signature`. Its design is **data-layer signing over canonical bytes
  with stdlib Ed25519/ECDSA** — see [`graph-signing.md`](graph-signing.md). That likely means the old hand-rolled
  KMS/GPG sign backends are **deleted, not moved** (stdlib replaces them) — pending the key-custody decision in that
  doc. `pkg/op/graph.go` imports it; `signing` needs nothing from `pkg/op`, so no cycle.
- **Signing does not use `.sops.yaml`.** A graph-provenance signing key is independent of the encryption recipients
  for secrets; `pkg/signing` gets its own key configuration. Two unrelated concerns sharing one config file was the
  original coupling, now undone.
- We still sign only the **top-level graph** (sub-graphs intentionally unsigned). `pkg/sops` is therefore
  **encryption/decryption only** — pure getsops orchestration.

## Open questions

- **gitignore fidelity.** How far to carry git's pattern semantics (`!` negation, `**`, trailing-`/` dir-only) into
  `path_regex`, versus documenting the `^`/unanchored mapping and stopping there.

## Related

- getsops `config` (v3.12.1): `LoadCreationRuleForFile` loads the whole file, selects the first matching creation
  rule, resolves recipients into `[]sops.KeyGroup`, and validates — single file, no overlay (the only "merge" is
  `keyGroup.Merge`, recipients within one rule). We use it for resolution; we own discovery and the cross-file chain.
- git ignore precedence and `core.excludesFile` — the discovery reference model.
