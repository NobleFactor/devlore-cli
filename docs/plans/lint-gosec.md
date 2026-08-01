---
title: "Class d: gosec — three real fixes, one tightening, 49 reasoned suppressions"
issue: TBD
status: complete
created: 2026-08-01
updated: 2026-08-01
---

# Plan: Class d — gosec

Fourth rung of the 4b-3 ladder. Policy approved 2026-08-01 (per-G-code); bug candidates
queued, reviewed one at a time, and ruled: fix 1–3, tighten 4.

## Real fixes (the ruled candidates)

1. **`argFileMode` mode bound** (`pkg/op/default_funcs.go`) — the typed path returns the
   `os.FileMode` directly (no conversion); the int path already rejected negatives and now,
   with the uint path, rejects anything above `0o7777`. A template arg can no longer wrap
   into a bogus-but-plausible mode.
2. **Pack-header section validation** (`function/pack.go` + callers) —
   `readFunctionPackHeader` now takes the pack size and refuses any header whose
   source/compiled section lies outside the file. Placed at decode, before any section
   read or allocation, per the checksum trust boundary's "corruption before verification
   is an error" rule. A corrupt header can no longer wrap the int64 section-reader
   conversions negative…
3. **…or size a hostile allocation** (`function/resource.go`) — `make([]byte,
   h.CompiledSize)` is reachable only after the same validation, capping it at the file's
   real size.
4. **Run-index privacy** (`internal/cli/index.go`) — state home `0o755 → 0o700`, run index
   `0o644 → 0o600`; per-user state nothing else reads.

## Reasoned suppressions (49)

- **G204 ×13** — running tools is the product: shfmt/shellcheck/markdownlint-cli2/git/
  pre-commit, all constant argv with provider-collected or configured paths; provenance
  stated per site.
- **G301/G302/G306 ×19** — deliberately shared artifacts: install dirs and binaries
  (0o755), repo config/hooks/source files and user config (0o644), the `.pub` trust half.
  Devlore config carries no secrets (SOPS owns secrets).
- **G703 ×4** — the archive spool path is os.CreateTemp's own name; the inventory tool
  walks its own argv. (Zip-slip proper is guarded in the extraction join: Clean +
  prefix-confined Join + symlink-target resolution.)
- **G704/G705 ×6** — configured-endpoint requests are appnet's purpose; text/JSON/markdown
  output has no HTML sink.
- **G602 ×2** — proven: reaching the append requires `bestScore >= 0.3`, which only a
  scored pair sets.
- **G404 ×1** — retry jitter needs no cryptographic randomness.
- **G115 ×4 remaining** — provably bounded by the new header validation (or a file
  length); suppressed citing the proof. Test-support widening and `int(f.Fd())` likewise.

## Verification

- gosec 56 → **0** uncapped; `make vet` and full `make test` pass; `gofmt -l` clean.
- Remaining ladder: revive 28 (e), noctx 14, unused 11 (f), complexity 61 (chartered).
