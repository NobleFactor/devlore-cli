---
title: sops config discovery, anchoring, and resolution (git-style)
status: draft
created: 2026-06-10
updated: 2026-06-11
---

## Summary

`.sops.yaml` config drives **encryption** and **signing**; **decryption is config-free**. The division of labor:
**discovery is ours**, **resolution and validation are getsops's**. We perform a git-style, `Root`-bounded upward walk
from the target file, collecting per-directory `.sops.yaml` over an XDG global fallback. For each discovered config we
hand getsops a `(confPath, filePath)` and it loads the file, selects the matching creation rule, resolves recipients,
and validates. We find the files; getsops turns one into validated recipients.

## Config consumers

- **Decrypt — config-free.** getsops reads the encrypted file's own embedded metadata plus ambient identities
  (`SOPS_AGE_KEY`, GPG, KMS); `.sops.yaml` is never consulted. The decrypt path must never trigger discovery.
- **Encrypt — per-file, config-driven.** (writ, via the encryption provider.) The file being encrypted has a path;
  its `path_regex`-matched creation rule supplies the recipients. This is getsops `LoadCreationRuleForFile`'s home
  ground — a real `filePath` to match and resolve against.
- **Sign — recipient set, no path.** `Client.Sign(canonical)` signs the graph's bytes with `CreationRules[0]` — no
  `filePath`, no path matching. It does not fit the per-file loader; see Open Questions.

## Discovery (ours)

Walk **up** from the target file's directory to `RuntimeEnvironment.Root`, collecting every `.sops.yaml`; then append
the XDG fallback `${XDG_CONFIG_HOME}/devlore/sops.yaml` (default `~/.config/devlore/sops.yaml`). Precedence is deepest
in-tree → shallower → fallback — git's `.gitignore`-over-`core.excludesFile` model (local overrides global).

getsops's own `FindConfigFile`/`LookupConfigFile` are **not** used: they walk to `maxDepth` rather than our `Root`, and
return a single file, not the ordered chain.

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

## Signing — resolved (out of the discovery design)

Only the **top-level graph** is signed: `GraphSpec.WithSopsClient` → `Sign(canonical)` → `CreationRules[0]`. There is
no `filePath`, so `CreationRules[0]` is correct and getsops's per-file loader does not apply. **Sub-graphs are
intentionally not signed** — we sign the graph, not its sub-graphs — so the removed `env.Sops` propagation conduit
stays removed (the plan provider's dropped `WithSopsClient` is the intended end state, not a stopgap). The getsops
loader and the discovery chain are therefore **encryption-only**.

## Open questions

- **gitignore fidelity.** How far to carry git's pattern semantics (`!` negation, `**`, trailing-`/` dir-only) into
  `path_regex`, versus documenting the `^`/unanchored mapping and stopping there.

## Related

- getsops `config` (v3.12.1): `LoadCreationRuleForFile` loads the whole file, selects the first matching creation
  rule, resolves recipients into `[]sops.KeyGroup`, and validates — single file, no overlay (the only "merge" is
  `keyGroup.Merge`, recipients within one rule). We use it for resolution; we own discovery and the cross-file chain.
- git ignore precedence and `core.excludesFile` — the discovery reference model.
