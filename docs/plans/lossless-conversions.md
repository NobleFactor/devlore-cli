---
title: "Conversions Follow Python's Rule: Cross-Category Is an Error"
issue: 709
status: draft
created: 2026-08-27
updated: 2026-08-27
---

# Plan: Conversions Follow Python's Rule — Cross-Category Is an Error

**Issue:** [#709](https://github.com/NobleFactor/devlore-cli/issues/709) ·
**Blocked by:** [#711](https://github.com/NobleFactor/devlore-cli/issues/711) — JSON reload yields float64 ·
**Epic:** [#444 — The resource model](https://github.com/NobleFactor/devlore-cli/issues/444)

## Summary

`convertDirect` asks Go whether the source type is `ConvertibleTo` the target and converts if so. Go's answer
is about *representability*, not meaning, and several conversions it permits silently produce a value the
author never wrote.

Measured against `op.Convert`:

| Conversion | Result | What happened |
| --- | --- | --- |
| `int64(65)` → `string` | `"A"` | the rune at that code point |
| `int64(300)` → `int8` | `44` | wraparound |
| `int64(-1)` → `uint8` | `255` | sign reinterpretation |
| `float64(3.9)` → `int` | `3` | truncation |

All four are silent. No error, no warning, and each result is plausible enough to survive review.
`float64 → string` and `bool → string` are already rejected, because Go permits neither — and that contrast
is what makes the integer cases read as deliberate.

## The rule

**Starlark is Python-shaped, so conversions answer to Python's rule rather than Go's.**

> **A cross-category conversion is an error, whatever the value. Within the integer category, the value must
> be in range.**

Category is decided by kind, not by inspecting the value. Range is the only question a value gets asked, and
only integer→integer asks it.

| From → To | Verdict | Why |
| --- | --- | --- |
| integer → string | **error** | Python never coerces; `str(x)` says it properly |
| float → integer | **error** | `TypeError: 'float' object cannot be interpreted as an integer` |
| integer → integer | in range, or **error** | Python's `struct.error: byte format requires -128 <= number <= 127` |
| integer → float | allow | Python widens implicitly (`1 + 2.0`) |
| string ↔ `[]byte` | allow, unchanged | see Out of scope |

**`float(3.0) → int` is an error too.** It is integral and survives a round trip, and Python still rejects it:
`[1,2,3][1.0]` is a `TypeError`. The rule is about category, not about whether this particular value happens
to be safe. That is the whole reason to prefer it over a round-trip test — a round-trip rule would accept
`3.0` today and reject `3.9`, which is a distinction no author can predict.

**Integer → string is rejected by kind, not by range.** `65 → "A"` round-trips back to `65`, so a
value-based check would let it through. Nobody passing `65` to a string parameter means `"A"`.

**The strictness is fair because Starlark has the escape hatches.** `str()`, `int()`, and `float()` all exist.
An author who means `"65"` writes `str(65)`; one who means `3` writes `int(3.9)`.

## What must keep working

**`file.mkdir(mode=0o644)`.** A Starlark int reaching an `os.FileMode`, which is a `uint32`. This is
integer→integer, in range, and stays allowed — which is why the rule cannot be "reject int→uint32 by kind".

## Steps

| # | Step | ✓ |
| --- | --- | --- |
| 1 | Failing tests first — all four rows, plus `3.0 → int`, asserting each is rejected | ☐ |
| 2 | Reject integer→string and float→integer by kind, ahead of the `ConvertibleTo` branch | ☐ |
| 3 | Range check for integer→integer; out of range is an error | ☐ |
| 4 | Confirm `0o644 → os.FileMode`, `int64 → float64`, and `string ↔ []byte` still convert | ☐ |
| 5 | Error messages in Python's voice, naming the fix: `str(x)`, `int(x)` | ☐ |
| 6 | Sweep for callers that depended on the looseness | ☐ |
| 7 | `test_imm_ui_one_string.star` gains `print(42)`, which #708 had to leave out | ☐ |
| 8 | `make check`, `test-race`, `test-scenario` | ☐ |

## Verification

- **Step 1 comes first deliberately.** On the previous two branches every test written after its fix had to be
  re-proved by breaking the fix — twice, and the second time it deadlocked the build. Writing the failing test
  first makes that proof free.
- **Step 6 is the risk, and it fired.** The rule turned five save/load tests red with
  `param mode: float64 ... fs.FileMode`. Not a defect in the rule: `LoadGraph` decodes json with plain
  `json.Unmarshal`, so every reloaded number is a `float64`, and `Convert` had been truncating it back. That
  is **[#711](https://github.com/NobleFactor/devlore-cli/issues/711)**, it reaches graph identity because the
  fix changes recomputed checksums, and it must land before this plan can go green.

- `convert_struct_test.go:41` hydrates a struct from a map with a literal `float64` reaching an `int` field —
  no json involved. That one encodes the old lenient contract and belongs to this plan, not to #711.

## Placement

The guard belongs in `convertDirect`, which is where the `ConvertibleTo` branch lives. That call is the last
resort for the whole ladder, so the rule will also govern slice elements, map values, and struct fields.
**That is intended**, and it is a wider blast radius than the issue title suggests — worth stating here rather
than discovering in step 6.

## Out of scope, and why

**`string ↔ []byte` stays implicit.** Python requires an explicit `.encode()` / `.decode()`, so a strictly
Python-shaped surface would make this explicit too. Starlark has a distinct `bytes` type, which makes it a
real question — and a much wider change than #709, affecting every provider method taking bytes. Separate.

**`bool` where an integer is wanted stays rejected.** Python allows it (`True == 1`); Go says not convertible,
so we reject today. Python-like would *loosen* this. Not worth loosening: a bool arriving where a number is
expected is far more likely to be a mistake than an intent.
