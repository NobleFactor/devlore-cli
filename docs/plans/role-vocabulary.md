---
title: "ProviderFlags: a Surfaces bit field and a Placement value, each distinctly typed"
issue: 715
status: draft
created: 2026-08-27
updated: 2026-08-27
---

# Plan: ProviderFlags — Surfaces and Placement

**Issue:** [#715](https://github.com/NobleFactor/devlore-cli/issues/715) ·
**Epic:** [#451 — Rename op to workflow](https://github.com/NobleFactor/devlore-cli/issues/451) ·
**Parent plan:** [workflow-rename.md](workflow-rename.md)

## Summary

`+devlore:surface=graph` produces `RoleAction`; `+devlore:surface=module` produces `RoleModule`. Two
vocabularies for one idea, connected only by a switch in `generate.star` — an author writes `graph` and the
generated code says `Action`.

The `op → workflow` rename forces the issue: `Graph` becomes `Definition`, so `surface=graph` will name a type
that no longer exists, and `module` is starlark's word for a namespace rather than a term in the new taxonomy.
**Neither current word survives.**

## The target

```go
+devlore:surface=script|workflow     // a list; default both
+devlore:placement=promoted          // one value; default qualified
```

| Directive | Accessor | Type | Constants |
| --- | --- | --- | --- |
| `+devlore:surface=` | `Surfaces()` | `Surfaces` (bit field) | `SurfaceScript`, `SurfaceWorkflow` |
| `+devlore:placement=` | `Placement()` | `Placement` (value) | `PlacementPromoted`, `PlacementQualified` |

The packed type is **`ProviderFlags`**, and each zone has its **own type**.

Every directive value is the same word as its constant, and every constant names the accessor that projects
it. Term selection and its alternatives are recorded on #715; this plan implements the outcome.

**`Dispatch()` → `Surfaces()`** because the accessor had the same defect as the constants: it named *how
methods are invoked*, and #677 changed the zone to mean *eligibility for a surface*.

**`ProviderRole` → `ProviderFlags`** for the same reason one level up. `Role` was specific and went stale; a
deliberately generic name cannot, and the accessors carry the meaning. Precedent: 14 `*Flag`/`*Flags` types in
Go's stdlib, and C#'s `BindingFlags`.

### The zones are not the same kind of thing

Surface is a **bit field** — each bit is one surface, and a provider supports one or more. Placement is a
**value stored in a zone** — a name is promoted or qualified, never both.

```go
// ProviderFlags packs a provider's Surfaces and Placement.
type ProviderFlags uint16

// Surfaces — which surfaces a provider's methods reach. A BIT FIELD: one or more.
type Surfaces uint8

const (
	SurfaceScript   Surfaces = 0x01
	SurfaceWorkflow Surfaces = 0x02
	// remaining bits reserved for further surfaces
)

// Placement — how a provider's method names are placed. A VALUE: exactly one.
type Placement uint8

const (
	PlacementQualified Placement = 0x00
	PlacementPromoted  Placement = 0x01
)

const placementShift = 8

func (f ProviderFlags) Surfaces() Surfaces   { return Surfaces(f) }
func (f ProviderFlags) Placement() Placement { return Placement(f >> placementShift) }
```

**Plural for the set, singular for the value.** `flags.Surfaces()` returning `SurfaceScript|SurfaceWorkflow`
reads correctly; `flags.Surface()` returning two things reads as a contradiction. The asymmetry means someone
reading only the accessor signatures learns which zone is a set and which is a value. Individual constants stay
singular — each names one surface.

**Each zone gets its own type**, following `reflect`, where `flag.kind()` returns `Kind` rather than another
`flag`. That makes `flags.Placement() == SurfaceScript` a compile error rather than nonsense that compiles.
C# disagrees — `MethodAttributes` packs a `MemberAccessMask` zone and flag bits in one enum — and Go's answer
was chosen for the type safety.

The zones are **read differently**, and that difference is the point:

```go
flags.Surfaces()&SurfaceWorkflow != 0     // test a bit — may hold several
flags.Placement() == PlacementPromoted    // compare a value — holds exactly one
```

Which settles the directive grammar. The two take different shapes because they ARE different shapes:

```go
+devlore:surface=script|workflow     // a list; one or more
+devlore:placement=promoted          // one value; default qualified
```

An earlier draft of this plan wrote `placement=promoted|qualified`, using `|` to mean "one of these" while
`surface=` used it to mean "any of these" — the same separator carrying two meanings, in a scheme whose whole
purpose is a consistent vocabulary. Corrected here.

**`PlacementQualified = 0x0000` is honest**, not awkward: it is the zero *value* of an enumeration, not an
absent flag. A provider declaring nothing gets `SurfaceScript|SurfaceWorkflow|PlacementQualified` — both
surfaces, names under the provider's own — which is exactly the default.

**Nothing can express a nonsense placement.** There is no way to write "promoted and qualified", not because a
check rejects it but because the zone holds one value. The flag spelling could not give that.

**Explicit values, no `iota`.** The block being replaced computes these with shifts (`1 << iota`,
`1 << (iota + 8)`). Per the standing rule, new constant declarations state their values; since this work
rewrites the block, the replacement states them.

**Constants take the accessor's name, not the type's,** so the two zones are visible at every call site:

```go
op.RoleModule|op.RoleAction|op.RoleRoot                    // today: three undifferentiated peers
op.SurfaceScript|op.SurfaceWorkflow|op.PlacementPromoted   // two surfaces and one placement, distinctly typed
```

That flattening is how #708 went wrong: `ui` was given root placement after reasoning about one surface, and
silently promoted six names into `plan.*`.

## Scope

| Name | References | of which `.gen.go` |
| --- | --- | --- |
| `RoleAction` | 134 | 28 |
| `RoleModule` | 104 | 28 |
| `RoleRoot` | 103 | 2 |
| `RootProviders` | 41 | 0 |
| `Dispatch()` | 13 | 0 |
| `ProviderRole` (type) | see below | — |
| `roleDispatchMask` / `rolePlacementMask` | 5 / 5 | 0 |

`ProviderRole` is renamed to `ProviderFlags` throughout; the two zone types are new.

**The authoring surface is four lines.** Only three providers declare anything:

```go
pkg/op/provider/ui/provider.go:32     +devlore:root=true
pkg/op/provider/plan/provider.go:51   +devlore:surface=module
pkg/op/provider/flow/provider.go:29   +devlore:root=true
pkg/op/provider/flow/provider.go:30   +devlore:surface=graph
```

`RootProviders()` becomes `PromotedProviders()`, which also resolves `promoteRootMethods` carrying two words
for one idea in a single identifier.

## The bootstrap, which is the hard part

`generate.star` emits the constant names as strings into every `.gen.go`. Renaming the constants in Go breaks
all 58 generated references at once; the tree stops compiling; `star` cannot be rebuilt; and codegen — the only
thing that could rewrite those files — cannot run. Same shape as #708's two-pass problem and #702's `rm -f`
cycle: **the output is an input.**

There is no two-pass ordering that escapes it here, because the constant name and its generated use must
change together.

**The LKG escape hatch is exactly this case.** `STAR ?= $(if $(wildcard $(STAR_LKG)),$(STAR_LKG),build/star)`,
and `build/star: FORCE` is never consulted when the LKG exists — so a prebuilt `star` regenerates a tree it
could not itself compile. The Makefile documents it for precisely this: *"when in-tree changes break star
compilation."*

`cmd/devlore-inventory` does **not** import `pkg/op` (verified), so `make generate`'s `inventory` step survives
the transition too.

### Order

1. `make star-lkg` — snapshot a working `star` **before** any edit. Without this the tree cannot recover.
2. Rename the constants, the accessor, and the masks in `pkg/op`.
3. Update `generate.star`'s mapping to emit the new names and accept the new directive values.
4. Update the four directive lines in `ui`, `plan`, `flow`.
5. `make regenerate` — runs on the LKG binary, rewriting all 58 generated references.
6. Update the hand-written consumers: registry slices, `plan.Provider`, `starlarkbridge`.
7. Build; the tree compiles again on its own.
8. Remove the LKG so `star` is rebuilt from source thereafter.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | `Surface()` and `Placement()` project the right zones | unit | a mask is wrong after renaming |
| 2 | `+devlore:surface=script` yields `SurfaceScript` | e2e, codegen | the generator mapping is wrong |
| 3 | `+devlore:placement=promoted` yields `PlacementPromoted` | e2e, codegen | as above |
| 4 | An absent `surface=` still yields both | e2e, codegen | the default is lost |
| 5 | An absent `placement=` yields `PlacementQualified` | unit | the zero value is not the qualified value |
| 6 | `ui` still promotes six globals AND six `plan.*` names | unit | placement semantics changed |
| 7 | `flow` still surfaces under `plan.*` only | unit | its surface value regressed |
| 8 | `plan` is still absent from a workflow surface | unit | its surface value regressed |
| 9 | Every gate green | `check`, `test-race`, `test-scenario` | anything above regresses |

**Rows 6–8 are the regression guard that matters.** This is a pure rename: the three providers that declare
directives must behave identically afterwards. `ui` is the sharpest, because it exercises both zones —
`root_test.go` already asserts its six globals, and #717 established that it also promotes six names into
`plan.*`. Both must survive.

**Row 5 is new behavior, not a rename.** `PlacementQualified` does not exist today. Naming the zero value is
the one semantic change here — the encoding is unchanged, but the zone becomes an enumeration with both of its
values named rather than a flag whose absence means something unstated. It needs its own assertion.

**A row worth adding once the type name settles:** nothing should be able to express a nonsense placement. That
is guaranteed by the encoding rather than by a check, so the assertion is that `Placement()` is compared with
`==` everywhere and never with `&` — a lint or a review rule rather than a test.

### Not covered

- **Nothing about the type name.** Settled: `ProviderRole` → `ProviderFlags`, with `Surfaces` and `Placement`
  as distinct zone types. Folded into this plan rather than deferred, because leaving `Role` on the type while
  removing it from every constant would be the worst of both.
- **The `op → workflow` rename itself.** This is its vocabulary prerequisite, not a phase of it.

## Verification

- The LKG must be taken **first**. Step 1 is not a convenience; without it steps 2–5 leave a tree that cannot
  rebuild the tool that would repair it.
- After step 8, confirm `star` builds from source and `make regenerate` still produces no diff — otherwise the
  LKG is load-bearing rather than an escape hatch.
