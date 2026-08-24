---
title: "Any: claims that assert existence, not kind"
issue: https://github.com/NobleFactor/devlore-cli/issues/616
status: approved
created: 2026-08-23
updated: 2026-08-23
---

# Plan: Any — claims that assert existence, not kind

## Summary

Give the claim surface the permissive option the discovery surface already has. A mutation target's kind is
not the author's business — a move moves whatever is there — but today the claim's Go type *is* the kind
assertion, so a kind-indifferent operation must either name a kind it doesn't care about or multiply into
one method per kind. **`file.Any`** is the claim that asserts existence and nothing else; it resolves
to the observed kind inside the `Pending → Active` transition, where the model already consults the disk.

The immediate payoff: **`file.move` moves any kind again from a plain authored path**, `file.move_directory`
retires, and a symbolic link can be moved — a capability that regressed when kind-honest activation landed
(#611, 2026-08-22).

## The rulings this plan implements (USER, 2026-08-23)

1. **A permissive claim type exists: `file.Any`.** It asserts "some taxonomy entry exists at this
   rel" — a dangling symbolic link satisfies it (the link is there); a FIFO, socket, or device does not
   (the taxonomy has no variant). It is a first-class announced variant, not a placeholder.
2. **The kind resolves in the `Pending → Active` transition** — `Active` replaces the unasserted object
   with the variant the disk shows; `Gone` leaves it `Any`, because nothing was observed. The catalog
   owns transitions, so the catalog owns the resolution; the resource supplies the how, through a seam
   shaped like `op.RootBinder`.
3. **The taxonomy is exactly the announced variants.** The shared base is demoted to an unexported
   `file.resource` — unnameable and un-hand-buildable outside the package, and never announced (it has no
   exported constructor). Its name pairs with the interface's: `Resource`/`resource`. **Amended
   2026-08-23**: the interface was first named `Entry` and the base `entry`; the USER ruled that if the
   interface is file's answer to the house `Resource` convention, it must carry that name, so both moved
   (see *Naming — the history* below).
4. **`file.move` takes `source Resource` and drops `on_missing`.** Moving something that is gone cannot
   achieve anything, so a missing source fails — under Stop-by-default that is now a pre-flight verdict at
   the starting line rather than a dispatch error. The tolerance was also unsound on a producer: the ignore
   path returned a nil product, handing downstream consumers a nil promise — the pathology that had `Skip`
   dropped from `MissingResourcePolicy` (#605).
5. **An interface-typed resource slot designates a mint type.** `Resource` mints `Any` for authored
   strings. The designation lives on the interface, once, not per parameter — two methods with the same
   slot type must claim the same way.
6. **`file.unlink` retires.** It and `remove` are one operation: both discover the entry at the path,
   `archiveAndPrune`, mint a `MutationDeleteFile` receipt, and `markEntryGone`, differing only in the
   claim type and a pre-check. A kind-indifferent `remove` covers a file, an empty directory, and a
   symbolic link (whose removal is already lstat-semantic — the link goes, the target stays). Removed,
   not deprecated.
7. **The removal family splits by blast radius, never by kind** — `remove` takes one entry, `remove_all`
   takes an entry and everything beneath it; both accept any kind. This is the stdlib's split, arrived at
   for the same reasons: `os.Remove` deliberately does not ask the kind (it tries `unlink`, then `rmdir`
   — "cheaper on average than doing a Stat plus the right one"), which is also why Go never needed an
   `Unlink`; and `os.RemoveAll` opens by calling `Remove`, so the recursive form is the simple form plus
   a fallback. Python splits the other way — `os.remove` / `os.rmdir` / `shutil.rmtree`, three functions
   keyed on kind, with `os.unlink` kept as an explicit legacy alias — which is the shape this ruling
   declines.
8. **`remove_all` over a symbolic link removes the LINK, never the tree it designates.** A follow is an
   explicit act (`file.resolve`); a removal never performs one. The mechanism already guarantees it —
   the destructive step is a rename into the recovery store, and a rename does not follow — and Go agrees
   by construction (`RemoveAll` starts with `Remove`, whose `unlink` takes the link and never recurses).
   `shutil.rmtree` refuses this case outright rather than choose; we do not need that insurance, because
   we cannot accidentally choose the other way.
9. **`on_missing` stays an explicit parameter with a fail-safe default, on both removals.** Deliberate
   divergence, recorded: `os.RemoveAll` tolerates a missing path (that is `rm -rf`'s `-f`), while
   `file.remove_all` defaults to `stop`. Fail-safe beats familiarity, and an author writes
   `on_missing="ignore"` to get the stdlib's behavior. (`pathlib.Path.unlink(missing_ok=False)` is the
   same posture independently arrived at.)
10. **Policy parameters go last — everywhere, not only here (RULED 2026-08-23).** A *policy* parameter
    governs the method's behavior when the work cannot proceed as asked; every other parameter describes
    the work itself. The ordering that follows is *what → what else to do → what if it can't*:
    `(target, prune, boundary, on_missing)`. The practical win is that coupled parameters stay adjacent —
    `boundary` is meaningless unless `prune` is true, and today `on_missing` is wedged between the target
    and that pair. The generated Starlark surface inherits Go's order, so the rule needs stating in one
    place only. The rule's home is the design record, not this plan: `4-resource-management.md` §3 states
    it beside `MissingResourcePolicy`'s definition, where the next author of a policy-bearing method will
    look.

## The type hierarchy

| Type | Kind | Role |
| --- | --- | --- |
| `file.Resource` | interface | `op.Resource` + `Path() fsroot.Path` + `sealedResource()` — the provider's headline type, an interface because file alone has a kind axis (lives in `resource.go`) |
| `file.resource` | unexported struct | the shared implementation (embeds `op.ResourceBase`, carries `SourcePath`); declares no `sealedResource()`, so a bare base is not a `Resource` and cannot enter a taxonomy signature (lives in `resource_base.go`) |
| `file.Any` | exported struct | the unasserted claim; embeds `resource`, declares `sealedResource()`, announced via `DiscoverAny` |
| `file.Regular` / `file.Directory` / `file.SymbolicLink` | exported structs | the kinded variants; embed `resource`, declare `sealedResource()`, announced |

All four variants are covered by compile-time interface guards, so a variant that stops satisfying
`Resource` fails to build rather than failing a review.

The base is unexported and stays that way: nothing outside `pkg/op/provider/file` names it, every embed
site is in-package, and it is never announced — announcement requires an exported
`func(*op.RuntimeEnvironment, any) (*T, error)` constructor, and the base has none. Only claimable types
need announcing, and the base is not one.

**Naming — the history, because the first answer was wrong.** This plan originally named the interface
`file.Entry` and the permissive variant `file.AnyEntry`, on the argument that file's answer to the
house `Resource` convention was the `Entry` interface: the other nine providers have no kind axis, so
their `Resource` is a struct, while file's has to be an interface over variants.

**Overruled by the USER, 2026-08-23, and rightly.** If the interface *is* file's answer to the
convention, then it should be *named* `Resource` — the convention should be satisfied in name, where a
newcomer meets it, not merely in spirit where only its author can see it. Naming consistency is a
long-haul property: one rule with no exceptions is easier to teach, easier to learn, and cheaper to
maintain, while a rule with one exception is two rules, and the exception is what everyone trips over.
So the interface took the provider's headline name, the base took its lowercase form, and the permissive
variant became `Any` — which reads better than `AnyEntry` anyway at the two places it appears: the
authored `kind="any"` and the intent row `#…/file.Any`.

What that bought beyond consistency: file stopped being the provider a newcomer must learn an exception
for, and it became the **template for [#625](https://github.com/NobleFactor/devlore-cli/issues/625)**,
where the other nine adopt the same sealed shape — there for correctness, since an exported struct
resource is hand-buildable and bypasses the catalog.

The reasoning behind the old name did not die; it moved. "Entry" is the standard filesystem word, always
qualified elsewhere (`fs.DirEntry`, `struct dirent`, .NET's "file system entry"), and the POSIX resonance
is load-bearing rather than decorative — a directory entry is a *name bound to an inode*, not the file
itself, which is exactly why this model claims **rels** and why `remove` over a symbolic link takes the
link. That belongs in `Resource`'s doc comment, where it explains the model, rather than in a type name.
Documentation obligation, still standing: the package holds both `file.Resource` (a claimed, cataloged
resource) and `fs.DirEntry` (a read-time enumeration record used by `walkDir`/`applyGitignore`), and the
doc comment must keep them apart.

*(The plan's filename predates the rename; it is left alone because six issues reference it.)*

## Why this is one rule, not a pile of special cases

- **Identity is untouched.** The catalog key for location addressing strips the URI fragment, so the
  Go type riding the URI is metadata, not identity. Resolving the kind orphans nothing and breaks no
  dedup.
- **The machinery exists.** Activation already replaces the ledger's object — copy-on-bind swaps in a
  run-bound copy through `ResourceCatalog.rebindEntry`. Kind resolution is the same swap with a different
  constructor, at the same moment.
- **It lands on the campaign's own split.** The graph document records `#…/file.Any` — intent:
  *something must be here*. The trace records `Regular` — observation: *this is what was there*. Graph =
  intent, trace = observation.
- **The collision rule dissolves.** A kinded claim and an unasserted claim on one rel are one identity,
  settled at plan time (the kinded claim wins the ledger slot — it asserts more, and the unasserted claim
  is satisfied by anything). By the time any discovery runs, the entry is already kinded, so the
  cross-kind intern error never fires.

**The residual, stated plainly:** a claim's Go type is not stable across the plan/run boundary for this one
variant. Code that type-asserts on a *claimed* entry rather than an *observed* one would be surprised. That
is a documented rule and a pin, not new machinery.

## The file-provider surface: every method, and what changes

All 27 announced methods, audited against the feature (2026-08-23).

**A. Changed by the feature (3)**

| Method | Change |
| --- | --- |
| `file.move` | `source *Regular` → `source Resource`; `on_missing` removed. Moves any kind; a missing source is unmet intent at pre-flight |
| `file.move_directory` | **Retired** — it existed only because the claim had to name a kind |
| `file.observe` | Signature already `resource Resource`, so no change there — but it *gains* authored-string claiming through the mint designation. `file.observe(resource="a.txt")` refuses today (it is the pinned refusal scenario) and claims after |

**B. Exposed by the feature — decided here (3)**

| Method | Finding |
| --- | --- |
| `file.remove` | `target *Regular` **contradicts its own documentation** ("file or empty directory"), and a directory is not a `*Regular`. Kind-indifferent destruction of a single entry → `target Resource` |
| `file.remove_all` | `target *Directory` is the only thing keeping `rm -rf` off a file or a link; the body already observes whatever is there → `target Resource`. Over a symbolic link it removes the link, never the designated tree (ruling 8) |
| `file.unlink` | **Retired (RULED 2026-08-23)** — subsumed by a kind-indifferent `remove`: the two bodies are the same operation, differing only in the claim type and a pre-check |
| `file.backup` | `sourcePath string` — the one mutation that consumes an existing entry **without claiming it at all**, delegating to the typed `Move`. With `move` claiming a `Resource`, backup should claim one too, or it stays the surface's last unclaimed mutation source |

**C. Examined, keeps its kind (5)** — the assertion is the operation's meaning, not a restriction on a
kind-indifferent act:

| Method | Why the kind stays |
| --- | --- |
| `file.copy` (`source *Regular`) | reads content; a directory has none |
| `file.read_bytes` / `file.read_text` (`resource *Regular`) | same |
| ~~`file.remove_all`~~ | **moved to group B (2026-08-23)**: its body is already kind-agnostic (`discoverEntryAt` → `archiveAndPrune`), so the `*Directory` claim was the only thing preventing `rm -rf` over a file or a link. The recursion distinguishes it from `remove`; the kind never did |
| `file.walk_tree` (`root *Directory`) | walking requires a directory |

**D. No claim; untouched (16)** — `discover`, `resolve` (take a path, produce an entry); `exists`,
`is_dir`, `is_file` (path predicates); `find`, `glob` (pattern discovery); `join`, `name`, `parent`,
`root` (path algebra); `link` (its `source_path` may legitimately dangle, so claiming it would be wrong);
`mkdir`, `write_bytes`, `write_file`, `write_text` (destinations are products, never claims).

## Epic and issue placement

**Epic: #444 — The resource model (`Epic:ResourceModel`).** This plan amends a surface that shipped with
[#586](https://github.com/NobleFactor/devlore-cli/issues/586) (phase 4 of
[resource-construction](resource-construction.md)); it is a follow-on feature, not a continuation of that
plan's phases.

**Feature: [#616](https://github.com/NobleFactor/devlore-cli/issues/616)** — *Any: claims that assert
existence, not kind*.

| Phase | Task |
| --- | --- |
| 1 | [#617](https://github.com/NobleFactor/devlore-cli/issues/617) the taxonomy reshape: unexported base, `Any` variant |
| 2 | [#618](https://github.com/NobleFactor/devlore-cli/issues/618) kind resolution at the `Pending → Active` transition |
| 3 | [#619](https://github.com/NobleFactor/devlore-cli/issues/619) an interface-typed resource slot designates its mint type |
| 4 | [#620](https://github.com/NobleFactor/devlore-cli/issues/620) `file.move` unifies; `move_directory` retires |
| 5 | [#621](https://github.com/NobleFactor/devlore-cli/issues/621) the rest of the mutator surface |
| 6 | [#622](https://github.com/NobleFactor/devlore-cli/issues/622) closure — the design record catches up |

## Phases

### Phase 1 — the taxonomy reshape — status: complete 2026-08-23 (#617; PR #624)

1. The base struct is demoted to the unexported `file.resource` and the interface takes the name
   `file.Resource`; embed sites and composite literals follow (`&Regular{Resource: *base}` →
   `{resource: *base}`). Promoted access (`x.SourcePath`, `x.Path()`) is unaffected in Go; any code
   naming the embedded field as `.Resource` is swept, not assumed absent.
2. `file.Any` lands: embeds `resource`, declares `sealedResource()`, and announces with a discovery
   constructor (`DiscoverAny`) — the constructor claiming and rehydration both key on.
3. `Any.Exists` is lstat plus "any taxonomy kind admits" — the predicate `ResourceKind.admits` already
   spells (`ResourceKindAny`). Explicitly not the base's following `Stat`: a dangling link must count.
4. `Any`'s content identity delegates to the observed kind at call time (lstat, then the kinded
   `Digest`/`Etag`), so an unasserted claim does not silently lose drift detection before it resolves.

**Phase 1 record (2026-08-23, merged as #624 — develop `9425d29d`; #617 closed) — the taxonomy reshape
and the rename, landed together; one finding made it not behavior-neutral after all.**

- The former `file.Resource` struct becomes the unexported `file.resource` (in `resource_base.go`), the
  former `file.Entry` interface becomes `file.Resource` (in `resource.go`), and `Any` lands in `any.go`
  — embedding `resource`, declaring `sealedResource()`, announced through `DiscoverAny`. `EntryKind`
  became `ResourceKind` and its permissive value's authored spelling became **`kind="any"`**. The
  generator recognized the new variant with no prompting — a resource type is identified by an exported
  `func(*op.RuntimeEnvironment, any) (*T, error)`, so `any.gen.go` was emitted automatically, and the
  base stays unannounced because it has no such constructor.
- The rename reached three consumers outside the package (`archive`, its test, `starcode`), and two
  named returns had to stop shadowing the new lowercase type (`resource *resource` → `candidate`). File
  moves went through `git mv`, so history follows.
- **The finding: unexporting the base silently removed every promoted field from the Starlark surface.**
  `TestLintCopyright` failed with `"Regular" object has no attribute "source_path"` — the bridge's
  `collectFieldInfo` skipped any unexported field, anonymous ones included, so an unexported embed took
  its whole field set out of the attribute surface. Fixed at the bridge, where the defect lives: the
  surface now mirrors **Go's** promotion rules, under which `outer.Field` resolves through an unexported
  embed. Verified empirically before writing the fix — reflection's read-only flag, which blocks reads
  through an unexported NAMED field, is deliberately not set for an anonymous one, so the promoted value
  is readable (`CanInterface() == true`). Pinned in `go_receiver_test.go`.
- Two other over-reaches from the mechanical rename, caught by the compiler and fixed: `receipt.Resource()`
  and `state.Resource()` are `op.Receipt`'s method, not the local type; and `helpers.go`'s error text
  named the now-unexported type.
- Pins: the permissive predicate's full matrix (regular, directory, link, **dangling link → present**,
  missing → absent) with the taxonomy bound pinned unix-only via mkfifo (a FIFO exists but no variant
  could resolve to it); content-identity delegation to the observed kind, and its failure direction on a
  missing path.
- **The file-organization question closed itself.** The rename decided it: the interface took
  `resource.go` (the package's public face) and the base moved to `resource_base.go`, so every file names
  what it holds and nothing had to be merged.
- A doc sweep followed the mechanical rename: the missing `_ Resource = (*Any)(nil)` compile-time guard
  (the variant this phase introduced was the one not covered), the broken article the substitution left
  ("an Resource"), and `Resource`'s own doc comment, which still described the pre-rename world.
- Gate: make check 103 ok / 0 fail; vet clean under darwin, windows, and linux; gofmt clean. CI green on
  all three platforms.

### Phase 2 — kind resolution at the transition — status: complete 2026-08-23 (#618), in tree

1. A seam shaped like `op.RootBinder`: a resource that can resolve its own kind implements it
   (`file.Any` does; nothing else needs to).
2. `ResourceCatalog.VerifyExistence` drives it: on the `Pending → Active` branch, a resolvable entry is
   replaced in the ledger via `rebindEntry` with the variant the disk shows, and the transition marks the
   resolved object Active. The `Gone` branch resolves nothing.
3. Idempotence is free: `VerifyExistence` early-returns on an already-`Active` entry, so resolution
   happens exactly once, at the consuming scope's starting line.
4. Pins: the resolution both directions (Active resolves to the observed kind; Gone stays `Any`),
   and the ledger holding one entry throughout.

**Phase 2 record (2026-08-23) — the seam lands; the audit found the part that would have failed
silently.**

- `op.KindResolver` joins `op.RootBinder` on the transition: `ResolveKind() (Resource, error)` returns
  the typed resource the observation names, freshly built and uninterned. `file.Any` implements it by
  delegating to the same `observed()` helper its content-identity tiers use; no other scheme has a kind
  axis, so no other scheme implements it and no other transition changes.
- **The audit's catch: `candidateOfMode` builds an UNLINKED candidate** — a fresh URI whose type
  fragment names the observed kind, and **no catalog id, no producer stamp**. Swapping that in naively
  would have stranded `byID` on a row whose occupant answers `""` to `ID()`, and `markActive` would have
  recorded the state under the empty id. Identity therefore crosses the swap and is stamped by the
  **catalog** (`resolveKind`), not by the implementation — identity is the catalog's business, and an
  implementation that stamped its own would be claiming an authority it does not have.
- The namespace needs nothing: location addressing keys on the fragment-stripped URI, so `Any` and
  `Regular` spellings name one identity. That is also what lets the type fragment move with the
  resolution — which is how the trace comes to say `Regular` where the graph's intent says `Any`.
- **A resolution failure is not an existence failure.** The entry has already been observed to exist, so
  a resolver that errors leaves it Active and unasserted rather than converting a kind problem into a
  missing-resource verdict.
- Pins: five at the catalog level with fixtures (resolution with identity carried forward; the Gone
  branch resolving nothing; exactly-once; the failure tolerance; a non-resolver untouched) and two at
  the file level against a real filesystem (an `Any` claim becomes `*Regular` with one id and one ledger
  row; an unmet claim stays `Any` and goes Gone).
- Gate: make check 103 ok / 0 fail; vet clean under darwin, windows, and linux; gofmt clean.

### Phase 3 — the designated mint type — status: complete 2026-08-23 (#619), in tree

1. An interface-typed resource slot designates its mint type; `Resource` designates `Any`. The
   registration lives beside the announcement, not at the call site.
2. `bindAuthoredValue`'s interface refusal **narrows**: it fires only for an interface with no designated
   mint type, and `test_judgment_entry_slot_refusal.star` is re-authored to that narrower rule rather
   than deleted.
3. `4-resource-management.md` §5.7 rule 6 is amended in place, with the reason recorded — a claim asserts
   a kind, and now an interface can designate the claim that asserts none.

**Phase 3 record (2026-08-23) — the feature becomes reachable from a plan.**

- `op.RegisterResourceMint(resourceInterface, mint)` mirrors `RegisterPlanPathNormalizer`'s shape: a
  registry keyed on the interface, populated from provider init beside the announcement. `file` registers
  `Resource → *Any`.
- The claiming seam substitutes the mint type **ahead of** the plan-space grammar, the source-constructor
  lookup, and the conversion — so an authored string bound to an interface behaves exactly as one bound
  to an explicitly kinded parameter, with no downstream branch. `*Any` joined the normalizer
  registration in the same init, so plan-space grammar applies to it like any other file claim.
- **The refusal narrows rather than disappearing**: an interface with no designation still refuses, and
  the message now names the missing designation instead of the interface's silence.
- **Scenario re-authored, not deleted — and its direction flipped.** The narrowed refusal turned out to
  be *unreachable from Starlark*: every announced resource-interface parameter is `file.Resource`, which
  is designated. So `test_judgment_entry_slot_refusal.star` became
  `test_judgment_interface_slot_mints.star`, pinning what an author now experiences —
  `file.observe(resource="observed.txt")` claims where it used to refuse, and the stored intent row's
  type fragment names `file.Any`. The refusal keeps a Go pin, where an undesignated interface can be
  constructed directly.
- Pins: the two planner directions (undesignated refuses; designated mints) plus the star.
- Gate: make check 103 ok / 0 fail; vet clean under darwin, windows, and linux; gofmt clean.

### Phase 4 — `file.move` unifies — status: pending

1. `file.move(source Resource, destination_path)` — no `on_missing`; `moveEntry`'s shared core is already
   kind-agnostic, so the two typed fronts collapse onto it.
2. `file.move_directory` retires (removed, not deprecated — the governing principle); writ migrate's layer
   registration reverts to `file.move`.
3. The missing-source path retires with the policy parameter: a missing source is unmet intent at
   pre-flight, which is where the author learns of it.

### Phase 5 — the rest of the mutator surface — status: pending

Group B of the surface audit, each landing with its own pins and authoring sweep:

1. `file.remove` takes `target Resource` — the type stops contradicting the documented contract, and a
   single-entry removal works on a file, an empty directory, or a symbolic link. `remove` keeps
   `on_missing`: unlike `move`, a missing target means the goal already holds, and the method produces no
   resource whose absence could become a nil promise.
2. **`file.unlink` is removed**, and its callers move to `remove` — the full sweep, enumerated
   2026-08-23: writ decommission (the planner's `file.Unlink` branch, the readback action set, and the
   completion tally that sums unlink+remove), `test_file_unlink.star`, the Go unit tests naming it, and
   the generated action name. The non-empty-directory guard stays where it is: a non-empty directory is
   `remove_all`'s business under any claim type.
3. `file.backup` claims its source (`source Resource`) rather than taking a bare path — the surface's last
   unclaimed mutation source.
4. `file.remove_all` takes `target Resource` — `rm -rf` over any kind, and over a symbolic link it takes the
   link (ruling 8). `file.copy`, `file.read_*`, and `file.walk_tree` keep their kinds, with the reason
   recorded beside each in the design doc so the asymmetry reads as a decision rather than an oversight.
5. **Policy-last ordering applied** (ruling 10). The audit found this touches nothing outside
   `file.Provider`: all six policy-typed parameters in the codebase are its `onMissing` slots, and three
   of them retire in this feature (`move` drops it; `move_directory` and `unlink` are removed), leaving
   `remove` and `remove_all` as the only carriers. `TransitionPolicy` and `RetryPolicy` are node-level
   configuration, and `ConflictPolicy` is read from the runtime environment rather than passed — but the
   rule pre-decides their position should any of them ever become a per-call parameter.

### Phase 6 — closure — status: pending

1. `3.5.4-file-provider.md` records the four-variant taxonomy, `Any`'s predicate, and the unified
   `move`; `4-resource-management.md` records the resolution-at-transition rule beside the kind-honest
   `Exists` block; both status files follow.
2. The `resource-construction` plan gains a back-reference: phase 4's `move_directory` is superseded here,
   so the record shows why it existed and why it stopped existing.

## Judgment scenarios

Authored as predictions before implementation; each graduates to a devlore-test case.

1. **Move a symbolic link.** The capability that regressed: `file.move(source="the-link", …)` renames the
   link itself (lstat semantics — the target is untouched, and a dangling link still moves). The headline
   proof.
2. **Move a directory through the unified move.** writ migrate's case: one `file.move`, a `*Directory`
   observed at dispatch, the tree renamed.
3. **The claim resolves at activation.** An `Any` claim over a regular file: the stored document says
   `#…/file.Any` (intent), the trace says `Regular` (observation), and the ledger holds one entry
   throughout — graph = intent, trace = observation, in one scenario.
4. **An unmet unasserted claim.** The path does not exist at the starting line: the entry goes `Gone` as
   `Any` (nothing was observed to resolve to), and the run fails pre-flight with the catalog's
   verdict before any dispatch.
5. **Kinded and unasserted claims on one rel.** A copy claims `a.txt` as `*Regular` and a move claims it
   unasserted: one catalog entry, the kinded object in the slot, both consumers linked.
6. **A discovery over a resolved claim.** `file.discover(path, kind="regular")` on a rel already claimed
   unasserted: the entry is kinded by then, so the discovery reaches it with no cross-kind error.
7. **The narrowed refusal.** An authored string into a `Resource` slot now *claims*; an authored string into
   an interface with no designated mint type still refuses, with the narrower message.
8. **A missing source fails at pre-flight, not at dispatch** — the `on_missing` removal, pinned as the
   verdict's location, not merely its presence.
9. **`remove` removes a symbolic link, and only the link.** `test_file_unlink.star` re-authored onto
   `file.remove`: the link goes, its target survives untouched, and compensation restores the link — the
   proof that retiring `unlink` lost no behavior.
10. **`remove` still refuses a non-empty directory.** The guard is kind-independent: `remove_all` owns
    subtrees, whatever the claim type says. Worth stating because the guard is pure policy — the
    mechanism (a rename into the recovery store) would move a populated tree perfectly well; the refusal
    exists so that destroying a tree must be *said*, not stumbled into.
11. **`remove_all` over a symbolic link to a populated directory removes the link only.** The link is
    archived and recoverable, the designated tree is untouched, and the trace records one Gone entry —
    the link's. The scenario is authored against a link whose target holds files, so a follow would be
    unmistakable in the result.

## Verification

Every phase: `make check`, `make vet` under GOOS windows and linux, `gofmt -l`. The Windows baseline is
zero; any movement is a defect in the phase that caused it. Phase 1's rename is expected to be
behavior-neutral — the suite must stay green across it with no test edits beyond the field name.

## Open questions

1. ~~Does `file.unlink` survive?~~ — **RULED 2026-08-23: retired.** One act, one spelling.
2. **Do other schemes want a permissive claim?** `Any` is a file-taxonomy answer. `pkg`, `svc`, `git`,
   and `appnet` have no kind axis today, so nothing is owed — but the interface-designates-a-mint-type
   mechanism is scheme-neutral if one ever appears.
3. **Should `Any` be authorable as a *discovery* result kind?** `file.discover(kind="entry")` already
   returns whatever it observes, kinded. There is no evident need for a discovery to return an unasserted
   object, and doing so would create the only path to an `Any` that never resolves.
