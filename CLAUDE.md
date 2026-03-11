# Claude Code Instructions for devlore-cli

## Session Protocol

**At the start of EVERY session and after EVERY compact:**
1. Read this file (`CLAUDE.md`) in full
2. Read `~/.claude/CLAUDE.md` (root instructions)
3. Review issue #65: https://github.com/NobleFactor/devlore-cli/issues/65

## Error Tracking

All errors of judgment are tracked in **issue #65**. Before completing any task, consult this issue and follow the verification checklist.

## Governing Principle

**This is a greenfield product. There is NO legacy. There are NO old APIs. There are NO previous versions to support.**

Any code that:
- Maintains backward compatibility
- Preserves old API signatures
- Accepts legacy formats
- Provides fallbacks for old behavior

**IS A CRITICAL BUG.**

When in doubt, DELETE. There are no legacy users to break.

## Verification Checklist

Before completing any task:

- [ ] Grep for `legacy|backward|compat|deprecated` — remove all matches
- [ ] Verify no stub functions return success without implementation
- [ ] Verify removed comments have corresponding code removal
- [ ] Run tests to confirm rejection of old formats/signatures
- [ ] Consult issue #65 for known error patterns

## Build and Test

**Use `make` for ALL build and test operations. NEVER run `go build`, `go test`, or `go vet` directly.**

- `make build` — build with ldflags (version, commit, date)
- `make test` — run tests with timeout
- `make check` — vet, lint, shell-lint, complexity, test
- `make vet` — go vet


## Repository-Specific Notes

- Default branch: `develop`
- Package manifest filename: `packages-manifest.yaml` or `packages-manifest.json` (NO other names)
- Schema: `schema.DevloreSchema` only (NO aliases)
- Decryptor signature: `func(string, []byte) ([]byte, error)` only (NO fallbacks)
