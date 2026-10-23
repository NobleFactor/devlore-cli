---
title: "A secret with no format before .sops deploys as a JSON wrapper"
issue: https://github.com/NobleFactor/devlore-cli/issues/977
status: active
created: 2026-10-21
updated: 2026-10-23
---

# Plan: writ encrypts and decrypts every secret one way, as bytes, so token.sops deploys as its plaintext

Lane 47 of #949. One pull request.

## Summary

A secret whose name carries no format before `.sops`, such as `token.sops`, deploys as `{"data": "..."}` instead of
its plaintext. Writ chooses between two of sops's modes from the name: structured, for `.yaml`, `.yml`, `.json`, `.env`
and `.ini`, where sops parses the document and encrypts each value; and binary, for anything else, where sops encrypts
the file's bytes as one value. Encrypting `token`, writ chooses binary; decrypting `token.sops`, it finds no format in
the name, looks at the ciphertext, sees the `{` every binary document begins with, and chooses JSON, whose output is the
wrapper.

Writ is to use one mode, binary, for every secret, in both directions. The name plays no part beyond the two suffixes
writ acts on, `.sops` (decrypt) and `.tmpl` (render); the format detection is deleted, and with it #977.

## Rulings

- 2026-10-22: "tests should confirm results if we encrypted a file for testing, we should confirm the result by
  comparing observed to expected content."
- 2026-10-22: "we might create some test files and push them through the round trip: encrypt, decrypt, and compare."
- 2026-10-22, of structured and binary: "I don't like doing things two ways when one way works. as long as i have sops
  edit, why do i care about structured when our use of it is so narrowly defined. we have a handful of common, but not
  exhaustive list of file types that might contain secrets."

## Current State

At `0e7bcdd4`, `pkg/sops/decrypt.go` `detectFormat(path, data)` strips `.sops` and `.tmpl`, reads the extension left
(then the last one, which decides nothing the first did not), and for a name with neither falls back to the content:
`{` gives `json`, `-----BEGIN AGE` gives `binary`, anything else `binary`. `Encrypter.Encrypt`
(`pkg/sops/encrypter.go:68`, through `storeForFormat`) and `Client.Decrypt` (`decrypt.go:37`) are the only callers,
and the only places writ chooses a format. No guide tells the user to run the sops command line.

sops itself, at v3.12.1 (`cmd/sops/formats/formats.go:55`), reads a name's last suffix only, and anything not `.yaml`,
`.yml`, `.json`, `.env` or `.ini` is binary: every `*.sops` name is binary to `sops decrypt` and `sops edit`. Writ's
structured `secrets.yaml.sops` is not, without `--input-type yaml`.

`pkg/sops/decrypt_test.go` pins the format detection, the content fallback among it. Six checks of what decryption
gives accept anything containing a fragment of the cleartext, and so would pass the wrapper:
`pkg/sops/encrypter_test.go:47`, `pkg/op/provider/encryption/provider_test.go:290`, `:293` and `:463`, and
`cmd/writ/writ/deploy/sops_integration_test.go:75` and `:85`. That deploy test and the encryption provider's tests build
their ciphertext with sops's YAML store directly, as writ will no longer. `docs/guides/writ/index.md:118-120` tells the
user to name a format, since without one a secret "deploys as a JSON wrapper today (#977)".

## Requirements

### Requirement 1: One mode, binary

- `Encrypter.Encrypt` encrypts with sops's binary store, and `Client.Decrypt` decrypts as binary, whatever the name.
  `detectFormat` and `storeForFormat` are deleted.
- What deploys is the bytes that were encrypted, exactly.
- A secret writ encrypted before in structured mode, any `.yaml`, `.yml`, `.json`, `.env` or `.ini` secret, no longer
  decrypts, and is encrypted again. Nothing reads the old form (the governing principle: no legacy).

### Requirement 2: Every test compares what it observes with what it expects, exactly

- The six fragment checks compare the decrypted bytes with the expected bytes, entire.
- **The round trip.** Test files in `pkg/sops/testdata/roundtrip/`, named as a user would name the cleartext: `token`
  (`hunter2\n`), `brace` (`{"k": 1}\n`), `blob` (a NUL and bytes that are not UTF-8), `secrets.yaml` with comments and
  uneven indentation, `secrets.json`, `app.env`, `app.ini`, `server.pem` and `not-yaml.yaml` (text that is not YAML).
  `TestEncrypter_RoundTrip` becomes a table over the directory: each file encrypted with `NewEncrypter().Encrypt` under
  its own name, decrypted with `Client.Decrypt` under its name plus `.sops`, as deploy decrypts it, and the result
  compared with the file, byte for byte. Every file comes back exactly; one that does not is a defect, reported.
- The deploy test and the encryption provider's tests build their ciphertext with writ's encrypter, and the deploy test
  adds `token.sops` from `hunter2\n`; each deployed file is compared with its cleartext, entire.
- `decrypt_test.go`'s format rows go with `detectFormat`.
- At `0e7bcdd4`, `token`, `blob` and the deploy row fail (each comes back as the wrapper), and `secrets.yaml` fails if
  sops's YAML store rewrites it; the run says which. All pass after.

### Requirement 3: The guide

- `docs/guides/writ/index.md`: any file can be a secret, named with `.sops` after its own name; it deploys as the bytes
  that were encrypted. `sops decrypt` shows one and `sops edit` changes one, with no options, given a key it was
  encrypted for. The sentence about the JSON wrapper goes.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/977-secret-with-no-format-before` and reviewed with the owner (`5465dab2`, approved
      2026-10-23; this box ticked a commit late, logged on NobleFactor/noblefactor-engprod#317)

### Phase 2: The tests, failing (Requirement 2)

- [x] The round trip's test files, the deploy row and the six exact comparisons; what fails at `0e7bcdd4` through
      `make test`, reported. All nine files fail: `token` and `server.pem` come back as the `{"data": ...}` wrapper;
      `blob` as the wrapper, its bytes that are not UTF-8 replaced by U+FFFD; `brace` and `secrets.json` re-indented
      by the JSON store; `secrets.yaml` rewritten by the YAML store (its comment moved, its quotes dropped, its
      indentation evened); `not-yaml.yaml` refused at encryption by the YAML store; and `app.env` and `app.ini`
      encrypt to documents `IsEncrypted` does not recognize, so `Client.Decrypt` would pass them through and deploy
      the ciphertext. The deploy test fails on `token.sops`, the wrapper; its two YAML rows and the encryption
      provider's three comparisons pass

### Phase 3: One mode (Requirements 1, 2 and 3)

- [ ] Binary in both directions; `detectFormat` and `storeForFormat` deleted; the tests' ciphertext built by writ's
      encrypter; the guide; every test passing

### Phase 4: Merge

- [ ] `make check-all`, finished green as the PR script runs it, read before this box is ticked
- [ ] PR script `~/Workspace/NobleFactor/devlore-cli.977-secret-with-no-format-before.bash` written, shown, and
      handed over; the PR resolves #977. This box's commit, the pull request's last, sets this plan `complete`

CI on the pull request is the script's gate, not a box here: the script merges only when every check has passed, and
no commit could record it (#199). Nothing acts on the checks while the gate watches them.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/sops/decrypt.go` | Modify | Requirement 1 |
| `pkg/sops/encrypter.go` | Modify | Requirement 1 |
| `pkg/sops/decrypt_test.go` | Modify | Requirement 2 |
| `pkg/sops/encrypter_test.go` | Modify | Requirement 2: the round trip |
| `pkg/sops/testdata/roundtrip/*` | Create | Requirement 2: the round trip's test files |
| `pkg/op/provider/encryption/provider_test.go` | Modify | Requirement 2 |
| `cmd/writ/writ/deploy/sops_integration_test.go` | Modify | Requirement 2 |
| `docs/guides/writ/index.md` | Modify | Requirement 3 |

## Out of Scope

- **How writ finds `.sops.yaml`**, which differs from the sops command's way: #1144, filed 2026-10-22 and resolved
  elsewhere ("at best a nice-to-have. Sev-3"). This plan leaves the discovery as it is.

## Open Questions

None.
