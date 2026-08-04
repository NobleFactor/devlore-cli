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

## Noted, not changed (rulings available)

1. quality-gate could additionally run `make lint` as belt-and-suspenders alongside the
   dogfooded star path.
2. cobra prints the full usage block under a `ui.fail` verdict (SilenceUsage unset) —
   cosmetic noise on failures.

## Verification

- Full-config lint: 0 findings uncapped; `make vet` + full `make test` green; gofmt clean.
- Live probe: dirty → exit 1 with the correct verdict message; clean → exit 0.
