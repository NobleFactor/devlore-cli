---
title: "Codegen Runs Every Time, and Its Output Must Match What Is Committed"
issue: 702
status: complete
created: 2026-08-26
updated: 2026-08-26
---

# Plan: Codegen Runs Every Time, and Its Output Must Match What Is Committed

**Issue:** [#702](https://github.com/NobleFactor/devlore-cli/issues/702) ·
**Epic:** [#452 — Code quality, coverage, and the lint gates](https://github.com/NobleFactor/devlore-cli/issues/452)
**Related:** [#670](https://github.com/NobleFactor/devlore-cli/issues/670) (CI ⊅ local checks)

## Summary

Codegen is mtime-gated. On a fresh clone every file is stamped at checkout, so whether a provider looks stale
is decided by write order rather than by its source — and git's parallel checkout makes that order
nondeterministic. When a provider does not regenerate, every validation its generator carries is skipped in
silence.

That already cost a false negative: darwin regenerated `goast`, caught a duplicate declaration through
`validate_action_name_consts`, and failed; linux did not regenerate and passed. Neither declaration was
build-tagged.

Underneath sits a second hole. Generated files are committed, CI never runs `generate` explicitly, and nothing
asserts that generation produced no diff. So a stale committed `.gen.go` passes — and when codegen *does* run,
CI tests the regenerated output while the repository holds something else.

## Goals

1. **Generator validations fire on every run**, not on whichever leg happened to look stale
2. **Committed generated output must equal generated output**, checked the way `go.mod` already is
3. **Local `make generate` keeps skipping unchanged providers** — that is the point of the mtime gate

## Mechanism

Two candidate mechanisms were considered before picking one, because "make codegen unconditional" hides the
real decision.

**Rejected — `make -B generate`.** `--always-make` propagates into the `build/star: FORCE` sub-make and
rebuilds targets that have nothing to do with codegen. It forces far more than the question requires.

**Rejected — a `FORCE` prerequisite on the 30 grouped rules.** It would make them regenerate always, including
locally, which costs goal 3 outright.

**Chosen — delete the outputs, then generate.** `NEW_OP_INVENTORY` already enumerates one output per provider,
and each is the head of a grouped rule. Removing the heads makes all 30 rules unconditionally stale, and each
rule re-emits its full group — so the existing variable does the enumeration and no new list has to be kept in
sync.

```make
regenerate: ## Regenerate every generated file from scratch, ignoring mtimes
	rm -f $(NEW_OP_INVENTORY)
	$(MAKE) generate
```

`inventory` is already unconditional, so it needs nothing.

Then in `quality-gate`, beside the `go.mod` check it mirrors:

```yaml
- name: Generated files are current
  run: |
    make regenerate
    git diff --exit-code
```

**One leg is the right scope.** Generated output is platform-independent — it has to be, since the repository
holds one copy — so currency is not a per-platform question. Forcing regeneration on `quality-gate` makes the
validations fire deterministically there, and the diff check makes every other leg's mtime-gated build safe,
because the committed files are then known to be current.

**A deletion that is not recreated fails the check**, which is the #572 class of bug — a rule naming an output
the generator no longer emits — caught for free.

## Steps

| # | Step | ✓ |
| --- | --- | --- |
| 1 | Add the `regenerate` target; add it to `.PHONY` | ✅ |
| 2 | Verify it actually forces all 30 rules — count generator invocations, expect 30 | ✅ |
| 3 | Confirm a clean tree stays clean: `make regenerate` then `git diff` is empty | ✅ |
| 4 | Prove it fails correctly — edit a `provider.go`, do not regenerate, confirm the diff check catches it | ✅ |
| 5 | Add the `Generated files are current` step to `quality-gate`, after `Check go.mod tidy` | ✅ |
| 6 | Confirm generator validations now run for every provider on that leg, `goast` included | ✅ |
| 7 | Confirm local `make generate` still skips unchanged providers | ✅ |
| 8 | `make check`, `make test-race`, `make test-scenario` | ✅ |

## Outcome

Landed as planned, with one mechanism change forced by a bootstrap cycle the plan did not anticipate.

| # | Step | Result |
| --- | --- | --- |
| 1 | `regenerate` target | touches sources; deleting outputs is impossible, see below |
| 2 | Forces all 30 rules | **159 files, 30 providers** — matches the darwin CI leg exactly |
| 3 | Clean tree stays clean | zero generated files in the diff after a full regeneration |
| 4 | Proves it fails | dropping a claim directive moved `ui/gen/provider.gen.go` |
| 5 | CI step added | after `Check go.mod tidy`, mirroring its shape |
| 7 | Local skip intact | plain `make generate` writes 0 files |
| 8 | Gates | `check`, `test-race`, `test-scenario` all pass |

### The mechanism the plan chose could not work

`rm -f $(NEW_OP_INVENTORY)` was wrong, and not by ordering — unconditionally. The generated files hold the
`op.Announce*()` call sites that `devlore-inventory` scans; `inventory` is a prerequisite of the star binary;
and `build/star: FORCE` rebuilds star on every run. **The outputs are also inputs**, so deleting them destroys
the bootstrap that would regenerate them. Demonstrated by doing it: inventory found 8 packages instead of 19,
then failed with `no op.Announce*() call sites found`.

Touching the sources forces the same staleness with nothing removed, and needs no LKG and no installed star —
which matters, because CI has neither.

For the record, deletion *would* be safe with an LKG present: `STAR` resolves to `build/star.lkg` when it
exists, and `build/star: FORCE` is then never consulted (Makefile:145). That path was not taken, because it
would make the CI check depend on an artifact CI does not have.

### Step 4 needed two attempts

The first probe edited a method doc comment and changed no generated output — method doc comments do not
reach the generated artifacts. Stopping there would have produced the conclusion that the check was broken. A
claim directive, which *is* emitted, produced the expected diff.

### The open question is answered

Generated output is **not** platform-dependent: regenerating all 159 files on darwin reproduced the committed
tree byte for byte. One CI leg is the right scope.

## Verification

- Step 4 is the one that matters: a check that cannot fail is not a check. It must be demonstrated failing on a
  deliberately stale generated file before it is trusted.
- Step 2 guards against the fix being a no-op — if `rm -f` misses outputs, some rules stay fresh and the
  nondeterminism survives in a form that now *looks* handled.

## Open question

**Does anything generated legitimately differ by platform?** The assumption above is no, since one copy is
committed. If some output embeds a path separator or a GOOS-dependent list, the diff check would fail on the
`quality-gate` runner for a legitimate reason, and the scope would have to widen. Step 3 answers this.
