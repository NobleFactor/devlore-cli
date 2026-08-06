---
title: "The lint gate's verdict: file-isolated JSON, no silent pass"
issue: TBD
status: complete
created: 2026-08-04
updated: 2026-08-04
---

# Plan: the lint gate's verdict

Next-steps item 1 after the complexity plan closed: CI's quality-gate passed PR #319 while
eleven findings existed. Investigation (probe package, both lint paths, stream inspection)
found the gate could not fail at all.

## Root cause, three layers

1. CI's "Lint Go" step runs `./build/star lint go ./...` — the star lint provider, not
   golangci-lint directly.
2. The provider asked for JSON on stdout (`--output.json.path stdout`), but stdout is
   shared: the repo config's own text format and golangci v2's stats footer pollute the
   stream, so it never parses as JSON.
3. `runGolangciLint` silently ignored the parse failure (`if jsonErr == nil` with no else)
   — zero issues, verdict passes. Every lint run since the stream turned dirty has passed
   unconditionally.

The `.star` command script was correct throughout (it reports and `ui.fail`s properly);
the exit-code path was also correct — earlier "exit 0" readings were a shell-pipe
measurement artifact ($? read head's status, not star's).

## The fix

`runGolangciLint(config, paths)` writes the JSON report to a dedicated temp file
(`--output.json.path <file>`) so no stdout format can pollute it, captures stderr
separately, and treats a missing or unparseable report as a hard error naming the stderr —
never a pass. `golangciArgs` gains the report-path parameter. Verified live: a probe
package with two planted findings fails the gate (`Go lint failed: 2 lint issues`,
exit 1); a clean package passes (exit 0).

## Follow-on rulings (both ruled and done, 2026-08-04)

1. quality-gate now also runs `make lint` directly ("Lint Go (direct)") as
   belt-and-suspenders alongside the dogfooded star path — a future regression in star's
   own reporting pipeline cannot silently re-open the gate.
2. star's root command sets `SilenceUsage`: a failing verdict prints only the error
   (verified live — `ui.fail` exits 1 with no usage block; unknown subcommands still show
   help, which is a genuine usage situation).

## Verification

- Full-config lint: 0 findings uncapped; `make vet` + full `make test` green; gofmt clean.
- Live probe: dirty → exit 1 with the correct verdict message; clean → exit 0.

## Fallout (2026-08-04, post-merge)

The gate's first honest run — on PR #321 itself — caught a real finding: an `unconvert`
complaint at `pkg/op/provider/file/helpers_unix.go:29`, visible only under Linux build tags
(`Stat_t.Dev` is `int32` on Darwin, `uint64` on Linux, so the conversion is required on one
platform and redundant on the other). Darwin-side recounts could never see it. Two process
failures compounded it into a red develop:

1. The pre-push recount ran on Darwin only. Recounts now also run `GOOS=linux`.
2. The merge command piped `gh pr checks --watch` through `tail`, so the shell gated the
   merge on `tail`'s exit status and merged the red PR. Merges now query check conclusions
   explicitly before merging.

Fix-forward: the suppression gains `unconvert` with the platform reason (removal would break
the Darwin build). Verified zero findings under both `GOOS` values.

Also noted for charter, since root-caused (correcting this paragraph's original non-TTY
attribution — the narrator keys only color off TTY, never suppression): the per-issue
`ui.warn` lines were invisible everywhere because every tool's runtime environment kept
`NewRuntimeEnvironmentSpec`'s Discard pre-fill, so the failing step reported a count
without naming the findings. Fixed by docs/plans/fix-narrator-wiring.md.
