---
title: "A Reloaded Number Is Read Against the Field It Fills"
issue: 711
status: draft
created: 2026-08-27
updated: 2026-08-27
---

# Plan: A Reloaded Number Is Read Against the Field It Fills

**Issue:** [#711](https://github.com/NobleFactor/devlore-cli/issues/711) ·
**Blocks:** [#709](https://github.com/NobleFactor/devlore-cli/issues/709) ·
**Excludes:** [#712](https://github.com/NobleFactor/devlore-cli/issues/712) — `any`-typed slots ·
**Epic:** [#443 — Serialization and the single codec](https://github.com/NobleFactor/devlore-cli/issues/443)

## Summary

JSON has one number type. `LoadGraph` decodes with plain `json.Unmarshal`, so every number in a reloaded
graph arrives as `float64` — an authored `0o644` reaches an `fs.FileMode` parameter as a float. `op.Convert`
truncates it back, which is why nothing has noticed.

Two things follow that are true today, before #709 touches anything:

- **The codecs disagree.** `yaml.v3` decodes an integer as `int`. The same graph reloads to different
  in-memory Go values depending on which codec wrote it.
- **`float64` cannot represent every `int64`.** A large enough integer in a saved graph is *already* corrupted
  on reload. Truncation makes the corruption look like a clean number.

## The rule

> **Convert from the value we serialize to the type of the field it fills.**

Preserve the literal through decoding; let the target type decide how to read it.

`json.Decoder.UseNumber()` yields `json.Number`, which keeps the text rather than forcing it through a
float. The precedent is already in the tree, deliberate and commented — `pkg/result/jq_filter.go:119`.

## Where the conversion belongs, and why it is not obvious

**At assembly, not at parameter binding.** The decode path is registry-aware end-to-end: `assembleGraph`
resolves each unit's action by short name through `env.Registry`, so the [Method] and its declared parameter
types are already in scope there. That is the first point where the field's type is known.

Doing it later — lazily, at dispatch — leaves `json.Number` in the graph's in-memory form, and the canonical
form is computed from that. A `json.Number` renders as a quoted string in YAML where a `float64` renders bare,
which is exactly the checksum mismatch a prototype produced:

```
op.LoadGraph: checksum mismatch:
  document   "sha256:8e5bf1a1…"
  recomputed "sha256:39ca2d67…"
```

Converting at assembly means the canonical form never sees a `json.Number` at all.

## The checksum is the risk, and the goal is that it does not move

Graph identity is settled design. **This plan's target is an unchanged checksum**, on the reasoning that
`fs.FileMode(420)` and `float64(420)` render identically in the canonical YAML — so replacing one with the
other changes nothing the hash sees.

That is a claim to **verify, not assume**. If checksums do move, this stops being a deserialization fix and
becomes a change to graph identity, which needs its own ruling against the existing format-identity rulings
rather than a decision made in passing.

There is an argument the change would be an *improvement*: a canonical form built from declared types no
longer varies with which codec decoded the document, which is the same asymmetry §the codecs disagree names.
But an improvement to identity is still a change to identity.

## Steps

| # | Step | ✓ |
| --- | --- | --- |
| 1 | A failing test first: an integer parameter, saved as JSON and reloaded, asserted to be an integer | ☐ |
| 2 | The same test for YAML, pinning the asymmetry closed | ☐ |
| 3 | Decode JSON with `UseNumber` | ☐ |
| 4 | Convert each argument to its declared parameter type in `assembleGraph` | ☐ |
| 5 | **Verify checksums do not move** — save, reload, compare to a stored document | ☐ |
| 6 | A value too large for its field is an error, not a truncation | ☐ |
| 7 | `make check`, `test-race`, `test-scenario` | ☐ |
| 8 | Confirm #709's five save/load failures are resolved by this alone | ☐ |

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | A JSON-saved integer reloads as an integer | unit | reverting to plain `json.Unmarshal` |
| 2 | JSON and YAML reload a graph identically | unit | either codec decodes numbers differently |
| 3 | `0o644` reaches `fs.FileMode` without a `float64` | unit | conversion moves out of `assembleGraph` |
| 4 | Too large for the field is an error, not a truncation | unit | the range check is dropped |
| 5 | **A saved document reloads to an identical checksum** | unit | the canonical form sees a new value type |
| 6 | A graph survives save → reload → run | scenario | any of the above regresses under a real runtime |

**Rows 1 and 2 are written before the fix.** Row 2 fails today for a reason unrelated to #709: `yaml.v3`
decodes an integer as `int` while JSON yields `float64`, so the asymmetry is a live defect and the test should
be red on arrival.

**Row 5 is the one that decides the plan.** It is not a regression guard — it is the experiment. An unchanged
checksum means the fix is contained in deserialization. A changed one means graph identity is in scope and
needs a ruling before anything merges. Run it before writing step 4, not after.

**Row 6 exists because rows 1–5 all stub the runtime.** #709's failures surfaced through
`TestJudgmentReloadDispatch` — a scenario, not a unit test — and a unit test over `LoadGraph` alone would not
have caught the `fs.FileMode` dispatch failure.

### Not covered

- **`any`-typed slots.** Out of scope by ruling; see #712. No test here asserts anything about them, and one
  that did would encode behavior #712 is going to change.
- **Non-numeric types through the codecs.** Strings, bools, lists, and dicts survive JSON unambiguously. Not
  tested here, and worth revisiting if #712 introduces a general envelope.

## Verification

- **Step 5 decides whether this plan is finished or has a second half.** An unchanged checksum means the fix
  is contained. A changed one means graph identity is in scope and needs a ruling before anything merges.
- **Step 8 is the reason for the ordering.** #711 exists because #709 turned five save/load tests red. If
  those do not go green on this branch, the diagnosis was wrong and #709 is blocked on something else.

## Scope: parameters with a declared type

This plan covers **only** parameters whose declared type says how to read the number. That is what makes the
rule work: an `fs.FileMode` wants an integer, a timeout wants a float, and the preserved literal answers
either.

**A parameter declared `any` is out of scope**, and not because it is hard — because no load-side fix can
reach it. `json.Marshal(float64(42))` emits `42`, so a float in an `any` slot loses its type at *save*, before
anything reloads. `UseNumber` preserves the literal faithfully; the literal simply no longer says "float".

Ruled 2026-08-27: for an `any` slot the **document stores the type**, since with no declared type at either
end the document is the only place the information can live. That is
[#712](https://github.com/NobleFactor/devlore-cli/issues/712) — it changes the document format, and it does
not block this plan. `any`-typed slots keep today's behavior until #712 is decided.
