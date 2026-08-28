---
issue: https://github.com/NobleFactor/devlore-cli/issues/720
title: "A graph carrying a Starlark lambda cannot write its receipt, and one fixture hides it"
status: draft
proof_run: TBD — defined by the charter's own decision (see Exit criteria)
created: 2026-08-27
updated: 2026-08-27
---

# Charter — the `function.Resource` receipt defect, and the coverage gap that hid it

**Chartered 2026-08-27** from the docker package rewrite
([docker.devlore-package.md](docker.devlore-package.md)), while probing whether `plan.choose` could
express a runtime-detection guard. The probe succeeded; it also surfaced this.

**No solution is assumed.** Two findings are chartered together because the second is why the first
survived: the defect is real, and nothing in the suite can see it.

## Finding 1 — receipt writing fails whenever the graph holds a lambda

Measured against `build/devlore-test` on 2026-08-27, `feature/docker.devlore-package`:

| Fixture | Lambdas in graph | Expectations | Exit |
| --- | --- | --- | --- |
| `test_compensation.star` | none | pass | 0 |
| `test_choose_then_action.star` | none | pass | 0 |
| `test_choose_exists.star` | one, `default=lambda:` | pass | **1** |
| `test_choose_lambdas.star` | many | pass | **1** |

The error, verbatim:

```
Error: writing receipt: op.Graph: pack tag:devlore.noblefactor.com,2026-01-01:sha256:<hash>
#github.com/NobleFactor/devlore-cli/pkg/op/provider/function.Resource: function.Resource: pack …:
mmap <session-root>/.devlore/function/resource/sha256/<xx>/<hash>: no such file or directory
```

A Starlark lambda is archived as a content-addressed `function.Resource` at plan time. At
receipt-write time the file is not on disk. Routing the receipt to a real path rather than
`/dev/null` changes nothing, so this is not an artifact of the output destination.

**Expectations pass in every case.** The graph plans, executes, and satisfies its assertions. Only
the receipt fails, and only afterwards.

## Finding 2 — one fixture is exercised through the CLI

`cmd/devlore-test/cli_test.go` drives exactly one script — `test_hello.star` — and contains zero
references to `choose`. The other 103 fixtures under
`cmd/devlore-test/devloretest/data/` are driven in-process, where receipts are apparently not
written at all.

So the suite is green while the receipt is unwritable. This is the same failure shape as the
knowledge extractor that passes CI continuously while emitting a near-constant artifact: a success
signal that does not cover the thing it appears to certify.

## Why this matters

Receipts are the mechanism compensation reads. A package whose graph carries one lambda produces no
receipt, and therefore cannot be decommissioned by compensation — which is the entire proof the
docker package exists to demonstrate.

The exposure is not hypothetical. All nine `test_choose_*` fixtures use `default=lambda: …`, so the
**idiomatic** way to write a decision tree is the failing way. The docker package works around it
with Ruling 8 (phase scripts contain no lambdas), building every branch from invocations instead.
That workaround is sound on its own merits — a branch that performs work beats one that computes a
string — but it should not be load-bearing for a defect.

## Diagnosis — found 2026-08-27, cause confirmed

`cmd/devlore-test/devloretest/commands.go`:

```go
result, err := runner.Start(cmd.Context())      // line 153
...
if err := writeReceipt(outputs.entries["receipt"], receiptFmt, runner.Graph()); err != nil {
    return fmt.Errorf("writing receipt: %w", err)   // line 166
}
```

`Runner.Start` opens its workspace with `fsroot.OpenScratch("devlore-test-*")` under
`defer iox.Close(&err, workspace)`, and its own comment states the consequence: *"a scratch root, so
the tree is removed by the same Close that releases it."*

So the workspace — including `.devlore/function/resource/sha256/…` — is **deleted when `Start`
returns**, and the receipt is written afterwards. Packing the graph then tries to read the function
pack off a path that no longer exists.

Graphs without lambdas are unaffected because nothing in them requires a disk read at pack time.
A `function.Resource` is deliberately metadata-only: `newFromURI` in
`pkg/op/provider/function/resource.go` documents that *"No content is archived — the on-disk pack is
the source of truth, rehydrated lazily."* The pack itself is written correctly at plan time
(`resource.go:396-410`, `WriteFile(sp, packBuf.Bytes(), 0o600)`). Nothing about the archiving is
broken; only the read outlives the tree it reads from.

**Production is not affected.** `lore` and `writ` run against durable roots, not `OpenScratch`, so
their `.devlore/` survives the run and a function pack is still readable when the receipt is
written. This answers the severity question this charter originally listed as needing to be settled
first: the defect is confined to `devlore-test`.

## Candidate fixes

Not a shortlist to choose from.

1. **Write the receipt inside `Start`**, before the workspace closes. Smallest change; makes the
   receipt part of the run rather than of the command.
2. **Defer workspace teardown past receipt writing** — hand the caller a closer rather than closing
   in `Start`. Keeps the receipt in the command layer, moves the lifetime.
3. **Pack function resources eagerly at seal time** rather than lazily at receipt time, so the graph
   document is self-contained and no read is needed later. Largest change; also the one that would
   make receipts robust to any root going away, not just this one.

## What remains to be discovered

- [ ] Should `cli_test.go` drive every fixture, a representative subset, or a characteristic-based
      selection? Driving all 104 through the CLI has a runtime cost worth measuring.
- [ ] Does any other resource type share the metadata-only-plus-lazy-read shape, and would it fail
      the same way? `mem.Resource` also uses `op.ContentAddressedReader`.

## Exit criteria

- [ ] A fixture whose graph contains a lambda writes its receipt and exits 0.
- [ ] `cli_test.go` exercises at least one lambda-bearing graph through the CLI, and that test is
      **proved to fail before it is trusted** — reverting the fix must turn it red.
- [ ] Ruling 8 in the docker plan is revisited: with the cause known to be harness-confined, a
      no-lambdas rule for package scripts may no longer be needed for receipt reasons — though it
      may still stand on its own merits, since a branch that performs work beats one that computes
      a string.

## Related

- [Docker devlore package](docker.devlore-package.md) — Ruling 8 and the probe that found this
- [LintStarlark charter](lint-starlark.md) — whether a linter should flag lambdas depends on
  whether this fix lands first
- `cmd/devlore-test/devloretest/data/test_choose_then_action.star` — the lambda-free fixture added
  during the probe; passes and writes its receipt
