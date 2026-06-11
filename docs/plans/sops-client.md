---
title: "Unified SOPS client in pkg/op/sops"
issue: pending
status: superseded
created: 2026-03-15
updated: 2026-06-11
---

# Plan: Unified SOPS Client

> **Superseded** by [`extract-starlark-from-op/phase-8/sops-config-discovery.md`](extract-starlark-from-op/phase-8/sops-config-discovery.md).
> This plan covers the original `pkg/op/sops` consolidation — since relocated to `pkg/sops` — and explicitly scoped
> *out* SOPS encryption (see Non-Goals), which is now in scope (writ, via `encryption.Provider.EncryptFile`). The
> env-side wiring it describes (`op.BindingConfig` / `op.ContextBase` `SopsClient`, `WithSopsClient` on the
> environment) has been removed: decryption is config-free, signing flows through `GraphSpec.WithSopsClient`, and
> config discovery/resolution is documented in the successor. Retained for history only.

## Summary

Consolidate all SOPS operations into a single `pkg/op/sops` package that provides config discovery, decryption, signing,
verification, and encryption detection through one `Client` type. This eliminates duplicate code across three packages,
unifies the `Signature` type, and scopes all I/O through `op.Root`.

## Goals

1. **Single entry point**: one `sops.Client` replaces `secrets.Manager`, `signing.BuildSignerChain`, and direct SOPS
   library calls in the encryption provider
2. **One Signature type**: `sops.Signature` replaces both `signing.Signature` and `op.Signature` — no more field-by-field
   copying
3. **Fail-fast on missing config**: `NewClient(dir)` returns an error if `.sops.yaml` is not found — callers decide
   whether SOPS is required at a higher level

## Non-Goals

- No new signing backends beyond the existing four (GPG, AWS KMS, GCP KMS, Azure KV)
- No SOPS encryption (only decryption and signing)
- No changes to `.sops.yaml` format or schema

## Current State

SOPS functionality is spread across three packages with duplicate logic:

| Package                       | Purpose                               | Config discovery                          | SOPS library usage                              |
| ----------------------------- | ------------------------------------- | ----------------------------------------- | ----------------------------------------------- |
| `internal/signing/`           | Receipt signing via KMS/GPG backends  | `FindSopsConfig()` — walks up from dir    | No — uses `.sops.yaml` for key identifiers only |
| `internal/writ/secrets/`      | File decryption, encryption detection | `findSopsConfig()` — identical walk logic | Yes — `decrypt.DataWithFormat()`                |
| `pkg/op/provider/encryption/` | Graph action for file decryption      | None — no config                          | Yes — `decrypt.Data()` hardcoded to YAML        |

### Duplicate code

| What                   | Location 1                  | Location 2                      |
| ---------------------- | --------------------------- | ------------------------------- |
| `.sops.yaml` tree walk | `signing/signer.go:105-119` | `writ/secrets/secrets.go:42-60` |
| `Signature` struct     | `signing/signer.go:17-27`   | `pkg/op/graph.go:139-149`       |
| Field-by-field copy    | `cli/receipts.go:131-136`   | —                               |

### Consumers

| Consumer                            | What it calls                              | What it needs                                        |
| ----------------------------------- | ------------------------------------------ | ---------------------------------------------------- |
| `internal/cli/receipts.go`          | `signing.BuildSignerChain(dir).Sign(data)` | `Sign(data) → *Signature`                            |
| `internal/writ/commands.go`         | `secrets.NewManager(dir)`, `m.Decryptor()` | `Decryptor() → func(string, []byte) ([]byte, error)` |
| `internal/writ/graph_builder.go`    | `secrets.Manager.Decryptor()`              | Same decryptor function                              |
| `pkg/op/provider/encryption/`       | `decrypt.Data(data, "yaml")`               | `Decrypt(data, sourcePath) → ([]byte, error)`        |
| `internal/writ/migrate/analysis.go` | `EncryptSOPS` constant                     | Detection only                                       |

## Design

### Package structure

```
pkg/op/sops/
  client.go      — Client, NewClient, Config types
  decrypt.go     — Decrypt, Decryptor, format detection
  detect.go      — IsEncrypted, IsSecretFile
  sign.go        — Sign, Verify, Signature, Signer interface, SignerChain
  gpg.go         — gpgSigner (unexported)
  aws_kms.go     — awsKMSSigner (unexported)
  gcp_kms.go     — gcpKMSSigner (unexported)
  azure_kv.go    — azureKVSigner (unexported)
  errors.go      — Error types
```

### Client API

```go
package sops

// Client provides SOPS operations. Config discovery happens at construction time.
type Client struct {
    config *sopsConfig
}

// NewClient creates a Client by searching for .sops.yaml upward from searchDir. Returns an error if no .sops.yaml is
// found. The op.Root parameter was removed to avoid an import cycle between pkg/op and pkg/op/sops.
func NewClient(searchDir string) (*Client, error)

// --- Signing ---

// Sign signs data using the first available backend from .sops.yaml. Returns nil signature if no signing backends are
// configured (age-only configs have no signing capability).
func (c *Client) Sign(data []byte) (*Signature, error)

// Verify checks a signature against data using the backend identified by sig.Method.
func (c *Client) Verify(data []byte, sig *Signature) error

// --- Decryption ---

// Decrypt decrypts SOPS-encrypted data. Format is inferred from sourcePath extension. Plaintext data passes through
// unchanged.
func (c *Client) Decrypt(data []byte, sourcePath string) ([]byte, error)

// Decryptor returns a decryption function matching the signature expected by the execution engine:
// func(source string, data []byte) ([]byte, error).
func (c *Client) Decryptor() func(source string, data []byte) ([]byte, error)

// --- Detection (package-level, no config needed) ---

// IsEncrypted reports whether data contains SOPS metadata or age armor.
func IsEncrypted(data []byte) bool

// IsSecretFile reports whether a filename indicates a SOPS-encrypted file.
func IsSecretFile(filename string) bool
```

### Signature type

```go
// Signature represents a cryptographic signature produced by a SOPS-configured backend.
type Signature struct {
    Method string `json:"method" yaml:"method"`     // gpg, aws_kms, gcp_kms, azure_kv
    Value  string `json:"value" yaml:"value"`        // base64-encoded signature data
    KeyID  string `json:"key_id" yaml:"key_id"`      // fingerprint, ARN, key URL
}
```

This single type replaces both `signing.Signature` and `op.Signature`. The `op.Graph.Signature` field changes to
`*sops.Signature`.

### Config types (unexported)

```go
// config models the .sops.yaml file structure.
type config struct {
    CreationRules []creationRule `yaml:"creation_rules"`
}

type creationRule struct {
    PathRegex string `yaml:"path_regex"`
    PGP       string `yaml:"pgp,omitempty"`
    Age       string `yaml:"age,omitempty"`
    AWSKMS    string `yaml:"aws_kms,omitempty"`
    GCPKMS    string `yaml:"gcp_kms,omitempty"`
    AzureKV   string `yaml:"azure_kv,omitempty"`
}
```

Config parsing is internal — consumers never see it.

### Signer interface (unexported)

```go
type signer interface {
    name() string
    available() bool
    sign(data []byte) (*Signature, error)
}
```

Backend implementations (`gpgSigner`, `awsKMSSigner`, etc.) are unexported. The `Client.Sign()` method iterates them
in priority order internally.

## Implementation Phases

### Phase 1: Create `pkg/op/sops` package — complete

- [x] Create `pkg/op/sops/client.go` — `Client`, `NewClient`, config discovery via `op.Root`
- [x] Create `pkg/op/sops/detect.go` — move `IsEncrypted`, `IsSecretFile`, `hasSopsMetadata` from `writ/secrets/detect.go`
- [x] Create `pkg/op/sops/decrypt.go` — move `Decrypt`, `Decryptor`, `detectFormat` from `writ/secrets/crypto.go`
- [x] Create `pkg/op/sops/sign.go` — `Signature` type, `signer` interface, `signerChain`, config parsing, backend
      construction
- [x] Create `pkg/op/sops/gpg.go` — move from `signing/gpg.go`
- [x] Create `pkg/op/sops/aws_kms.go` — move from `signing/aws_kms.go`
- [x] Create `pkg/op/sops/gcp_kms.go` — move from `signing/gcp_kms.go`
- [x] Create `pkg/op/sops/azure_kv.go` — move from `signing/azure_kv.go`
- [x] Create `pkg/op/sops/errors.go` — move from `signing/errors.go`
- [x] Tests for Client, detection, decryption, signing
- [x] `go vet` + tests pass (25/25; `make check` has pre-existing failure in `pkg/op/provider` blank import)

### Phase 2: Migrate `op.Graph.Signature` — complete

- [x] Change `op.Graph.Signature` field from `*op.Signature` to `*sops.Signature`
- [x] Delete `op.Signature` type from `pkg/op/graph.go`
- [x] Update all references to `op.Signature` across the codebase
- [x] Tests pass

### Phase 3: Migrate `internal/cli/receipts.go` — complete

- [x] Replace `signing.BuildSignerChain(dir).Sign(data)` with `sops.NewClient(dir)` + `client.Sign(data)`
- [x] Remove the field-by-field `signing.Signature` → `op.Signature` conversion
- [x] Tests pass

Note: `NewClient` signature changed from `(root op.Root, searchDir string)` to `(searchDir string)` to avoid import
cycle between `pkg/op` and `pkg/op/sops`. Config discovery uses direct `os.Stat` (identical to original implementation).

### Phase 4: Migrate `internal/writ/secrets/` consumers — complete

- [x] Replace `secrets.NewManager(dir)` with `sops.NewClient(dir)` in `writ/commands.go`
- [x] Replace `secrets.NewManager` + `m.Decryptor()` with `sops.NewClient` + `client.Decryptor()` in
      `writ/graph_builder.go`
- [x] No external callers of `secrets.IsEncrypted()` or `secrets.IsSecretFile()` found — only internal to secrets
      package
- [x] Tests pass

### Phase 5: Wire `*sops.Client` through `BindingConfig` → `ContextBase` — complete

- [x] Add optional `SopsClient *sops.Client` field to `op.BindingConfig`
- [x] Add `WithSopsClient(client *sops.Client)` builder method
- [x] Add `SopsClient *sops.Client` field to `op.ContextBase`
- [x] Add `SopsClient *sops.Client` field to `execution.ExecutorOptions`
- [x] Wire `ExecutorOptions.SopsClient` into `ContextBase` in executor
- [x] Remove `Data["decryptor"]` usage — `SopsClient` passed through ExecutorOptions
- [x] Tests pass

### Phase 6: Migrate `pkg/op/provider/encryption/` — complete

- [x] Replace direct `decrypt.Data(data, "yaml")` with `p.Context().SopsClient.Decrypt(data, sourcePath)`
- [x] Remove direct SOPS library import from encryption provider
- [x] Add nil-client guard (returns error if SopsClient not configured)
- [x] Tests pass (7/7 including new nil-client test)

### Phase 7: Delete replaced packages — complete

- [x] Delete `internal/signing/` (entire package — 7 files)
- [x] Delete `internal/writ/secrets/` (entire package — 6 files)
- [x] Verify no remaining imports of deleted packages
- [x] All tests pass

## Files to Create/Modify

| File                                     | Action | Purpose                                                              |
| ---------------------------------------- | ------ | -------------------------------------------------------------------- |
| `pkg/op/sops/client.go`                  | Create | Client, NewClient, config discovery                                  |
| `pkg/op/sops/decrypt.go`                 | Create | Decrypt, Decryptor, format detection                                 |
| `pkg/op/sops/detect.go`                  | Create | IsEncrypted, IsSecretFile                                            |
| `pkg/op/sops/sign.go`                    | Create | Signature, signer chain, config parsing                              |
| `pkg/op/sops/gpg.go`                     | Create | GPG signing backend                                                  |
| `pkg/op/sops/aws_kms.go`                 | Create | AWS KMS signing backend                                              |
| `pkg/op/sops/gcp_kms.go`                 | Create | GCP KMS signing backend                                              |
| `pkg/op/sops/azure_kv.go`                | Create | Azure Key Vault signing backend                                      |
| `pkg/op/sops/errors.go`                  | Create | Error types                                                          |
| `pkg/op/graph.go`                        | Modify | Change `Signature` field to `*sops.Signature`, delete `op.Signature` |
| `pkg/op/binding_config.go`               | Modify | Add optional `SopsClient` field and `WithSopsClient` method          |
| `pkg/op/context.go`                      | Modify | Add `SopsClient *sops.Client` field to `ContextBase`                 |
| `internal/cli/receipts.go`               | Modify | Use `sops.Client` for signing                                        |
| `internal/writ/commands.go`              | Modify | Use `sops.Client` for decryption                                     |
| `internal/writ/graph_builder.go`         | Modify | Use `sops.Client.Decryptor()`                                        |
| `pkg/op/provider/encryption/provider.go` | Modify | Use `sops.Client.Decrypt()`                                          |
| `internal/signing/`                      | Delete | Replaced by `pkg/op/sops/`                                           |
| `internal/writ/secrets/`                 | Delete | Replaced by `pkg/op/sops/`                                           |

## Resolved Questions

- [x] **Client wiring**: `*sops.Client` is specified on `op.BindingConfig` and flows into `op.ContextBase` as a typed
      field — the same pattern used for `Writer`, `DryRun`, `ProgramName`, `Root`, and `Platform`. This replaces the
      untyped `Data["decryptor"]` workaround. Each source root constructs its own client at the call site. Multi-repo
      writ builds multiple binding configs — one per source root — each with its own client. Providers access the client
      via `p.Context().SopsClient`.

- [x] **Verify dispatch**: `Client.Verify(data, sig)` dispatches by `sig.Method` internally. Callers never pick the
      backend — the signature carries that information.
