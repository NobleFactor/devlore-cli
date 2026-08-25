---
title: "Sealed provider resources: every announced resource type is an interface"
issue: https://github.com/NobleFactor/devlore-cli/issues/625
status: approved
created: 2026-08-23
updated: 2026-08-24
---

# Plan: Sealed provider resources — every announced resource type is an interface

## Summary

**Every announced resource type is a sealed interface, implemented by an unexported struct.** One rule,
no exceptions: a provider resource can be produced only by the provider that declares it.

`file` announces four such types (its kind axis); the other eight announce one each. That is not an
exception — it is one rule applied four times in one place and once in eight others.

**This is a correctness change. Consistency is its dividend, not its motive** — and the dividend is worth
naming, because it compounds: one rule with no exceptions is easier to teach, easier to learn, and
therefore easier to onboard into and cheaper to maintain for as long as the codebase lives. A rule with
one exception is two rules, and the exception is what every newcomer trips over.

## The correctness case, concretely

The threat is not a struct literal somebody might one day write. **No `&<provider>.Resource{…}` exists
outside its own package anywhere in the tree** — checked across all eight on 2026-08-24. The live threat
is a path the framework already takes.

`tryHydrateStruct` ([pkg/op/convert.go:441](../../pkg/op/convert.go)) is generic map→struct hydration
inside `op.Convert`. Its guard admits any conversion whose source is a string-keyed map and whose target
has concrete kind `reflect.Struct`. A resource slot declaring `*service.Resource` satisfies that today.
So an author who supplies a dict where a resource is expected gets `reflect.New(concrete).Elem()` — a
freshly minted `service.Resource` with its exported fields filled from the map.

The embedded `op.ResourceBase` has only unexported fields, so it stays zero. `URI()` returns `""`. The
value is not merely unclaimed, it is **identity-less** — and it reaches
[pkg/op/provider/service/provider.go:50](../../pkg/op/provider/service/provider.go), which reads
`name.Name` directly (20 such reads, none re-canonicalizing) and calls `sm.Disable(name.Name)` against
the host. A host mutation performed for a resource the ledger never issued.

Every guarantee the resource model makes is bypassed on that path:

- *claims are true when made* — nothing was verified;
- *identity is the catalog key* — the URI is empty;
- *a string is a key, never a constructor* — the dispatch seam refuses run-time construction, then a map
  does the same thing one layer down;
- *the graph carries complete intent* — an unclaimed resource is intent the document never recorded.

**Sealing removes the path rather than policing it.** The guard requires `reflect.Struct`. Once the slot
type is an interface, `tryHydrateStruct` declines and `Convert` falls through to the registered
constructor, which interns through the catalog. Nothing is left to remember to check.

`file` is exposed here too, and more quietly: its variants carry **zero exported fields**, so hydration
fills nothing yet still forges and returns an identity-less `*Regular`. Part 1 alone would leave the
most-used provider open on the very path that motivates the feature — which is why part 2 is not polish.

**What this does not fix.** `ConvertFrom` — e.g.
[pkg/op/provider/service/resource.go:305](../../pkg/op/provider/service/resource.go) — returns
`&Resource{Name: str}`, the same identity-less value, and it ships. Its own doc comment hands the problem
downstream: *"Provider methods consuming the projected Resource are responsible for re-canonicalization
… when full identity is required."* That is in-package construction, which the seal permits by design.
It is a separate defect and needs its own issue.

## The rulings this plan implements

From 2026-08-23 (USER):

1. **Every `<provider>.Resource` is an interface** — sealed by an unexported method, so implementations
   cannot be added from outside the package.
2. **The underlying struct is unexported (`<provider>.resource`) and is what carries the identity
   payload.**
3. **Naming is uniform**: the interface takes the provider's headline name (`Resource`), the struct takes
   its lowercase form (`resource`).

From 2026-08-24 (USER), after phase-1 investigation:

4. **The URI fragment names the interface, not the struct.** This supersedes the fragment clause of
   ruling 2. Because ruling 3 gives the interface the name the struct gave up, `typeIDOf` yields a
   byte-identical string — `…/provider/service.Resource` before and after. See *The correction* below for
   why the alternative was rejected.
5. **The work splits in two parts.** Part 1: the eight single-resource providers. Part 2: `file`'s four
   variants. The goal is reached only at the end of part 2 — the intermediate state is `file` announcing
   structs while the others announce interfaces, and that is accepted as temporary, not as a stopping
   point.
6. **`file`'s variant interfaces are discriminated by `kind() <Interface>`** — `kind() Regular` on
   `Regular`, `kind() Directory` on `Directory`, and so on.

## The correction

The original plan rested on an enabling fact that is **false**, and phase 1 exists to surface exactly
this kind of thing. It claimed `pkg/op/provider/<p>/gen/*.gen.go` declares the provider's own package, so
generated announcements could name unexported types. Verified 2026-08-24 with `go list`:

```
github.com/NobleFactor/devlore-cli/pkg/op/provider/service       name=service
github.com/NobleFactor/devlore-cli/pkg/op/provider/service/gen   name=service
```

Two import paths, the same package *name*, **different packages**. The gen file imports the provider as
`provider` and can name only its exported identifiers. The earlier check evidently read the
`package service` line and stopped before the import.

That mattered because the fragment is `typeIDOf(goType) = PkgPath() + "." + Name()`
([pkg/op/resource.go:529](../../pkg/op/resource.go)). Holding to ruling 2's fragment clause would have
required each provider to export a `ResourceType() reflect.Type` seam for the gen file to announce — and
`reflect.New` on that seam forges a resource, reopening the door the feature exists to close. It would
also have moved every URI, contradicting this plan's own no-drift criterion.

Ruling 4 dissolves both problems: the gen files name `provider.Resource` and `provider.DiscoverResource`,
exactly as they do today, and **need no changes at all**.

A second correction: the original said nine providers were in scope besides `file`. There are **eight** —
`appnet`, `function`, `git`, `json`, `mem`, `pkg`, `service`, `yaml` — confirmed by enumerating
`AnnounceResource` call sites.

## What ruling 4 costs

The receiver machinery is written for concrete types, and `reflect` treats an interface differently in
three ways at once. All three surface in phase 1 and are paid once.

**Where it breaks.**

1. **The two `PointerTo` promotions** — [pkg/op/helpers.go:258](../../pkg/op/helpers.go) and
   [pkg/op/receiver_type.go:357](../../pkg/op/receiver_type.go). Both read *"if not a pointer, make it
   one, so pointer-receiver methods are visible."* On an interface that yields `*Resource` — a pointer to
   interface, whose method set is **empty**. Providers that announce method metadata fail loudly at init
   (`parseParameters` → `MethodByName` → `"method Equal: not found on type service.Resource"`).
   `file.AnyKind`, which announces `nil`, fails **silently** with zero methods, because `parseParameters`
   returns an empty but non-nil map and the announced path then matches nothing.

2. **`NewMethod` cannot consume an interface method at all** —
   [pkg/op/method.go:178](../../pkg/op/method.go). Per `reflect`, an interface type's `Method` has
   `Func == nil` and a `Type` carrying **no receiver**. So `do.Type.In(0)` yields the first *parameter*
   for `Equal(other any)`, producing the action name `"..Equal"`, and **panics** outright for a no-arg
   method like `Etag()`, where `NumIn()` is 0. Ten lines on, `doFn := do.Func` is the zero `Value` and
   `doFn.Call` panics.

3. **Unexported methods become visible.** `NumMethod` counts them *only* for interface types, so
   `sealedResource` and — after part 2 — `kind` would enter the iteration.

**The change: `receiverType` stores the concrete type and the type id, not the interface.**

Point 2 is why *"stop promoting interfaces"* is not the fix. Enumeration is not the problem; the
machinery cannot build a dispatchable method from an interface however it is reached. Route it back to
concrete types instead, and split the one field that is doing two jobs:

- **the concrete `*resource`** serves method enumeration, promotion, dispatch, and the `byType` key at
  [pkg/op/receiver_registry.go:960](../../pkg/op/receiver_registry.go) — which must key on the concrete
  type, because `marshalReflect` looks up by `reflect.TypeOf(value)`.
- **a stored `typeID string`**, computed once from the interface at announce time, serves the two
  `typeIDOf(ProviderType()) == typeID` comparisons at `receiver_registry.go:486` and `:513`, which match
  against an id lifted from a URI.

Those two consumers sit on adjacent lines and want opposite things: `:513` needs the interface's id,
`:514` needs a concrete type to `reflect.New`. That is the whole difficulty, and splitting the field
resolves it — including `UnpackerByTypeID`, which starts working again for `function`, `json`, `mem`, and
`yaml` because `:513` matches on the stored id while `:514` mints from the concrete type.

With the concrete type restored, points 1 and 3 evaporate on their own: promotion applies to a struct
again, and `reflect` on a non-interface excludes unexported methods, so no `PkgPath()` filter is needed
anywhere.

`AnnounceResource` takes the concrete type as an **explicit argument** rather than resolving it through
`RegisterResourceMint`, so announcement carries no init-order dependency on registration. The gen file
emits both.

**The dividend.** The type id stops being re-derived from a Go type at every site and becomes a declared
identity. Today any Go rename silently invalidates every saved document; afterwards the id is a value that
can be seen and pinned.

Checked and *not* a cost: `plannerForType` ([pkg/op/planner.go:147](../../pkg/op/planner.go)) does the
same nil-interface probe but is only ever called with `metadata.Planner` — never a resource type. And
`deriveMethodParams` ([pkg/op/receiver_type.go:702](../../pkg/op/receiver_type.go)) already filters
`!m.IsExported()`.

One consequence ruling 6 does add: `file`'s variant interfaces each declare `kind()`, so part 2 must
confirm the concrete-type routing holds for four interfaces rather than one.

## What serialization does — and does not — do

**URIs do not move.** Ruling 4 makes the fragment `typeIDOf(interface)`, and ruling 3 gives the interface
the name the struct gave up, so the string is identical. The `resources` section of a document is
byte-identical for the same intent, before and after every phase.

**Resource payloads do not move.** A resource encodes as its URI (`UnmarshalJSON` takes a bare URI
string), and `encoding/json` marshals the exported fields of an unexported struct exactly as it does an
exported one.

**Receipt product-type ids move for resource-bearing results, and ruling 4 does not cover them.** This is
a separate string, derived from a different type by a different function.

`ReceiptBase.Commit` records `canonicalIDOf(result)` — the produced value's **dynamic** type
([pkg/op/receipt.go:438](../../pkg/op/receipt.go)). `ProductTypeByID` resolves it through an index built
from **every** action method's **declared** result type
([pkg/op/receiver_registry.go:542](../../pkg/op/receiver_registry.go)). The two need not agree, and today
they agree only by coincidence:

- `file.Move`, `Remove`, `RemoveAll`, and `Backup` already return the `Resource` interface, so they record
  a concrete dynamic id — `*…/file.Regular`, say. That id resolves because `file.Copy` independently
  *declares* `*Regular` and contributes the key. `*Directory` comes from `Mkdir`, `*SymbolicLink` from
  `Link`. Every kind those four can produce happens to be some other method's declared return.

**Sealing removes the coincidence.** The dynamic type becomes `*…/file.regular`, an unexported struct no
method declares, so nothing contributes the key and the lookup misses. And the miss is **silent** —
`retypeStampedResult` ([pkg/op/recovery_stack.go:647](../../pkg/op/recovery_stack.go)) reads:

```go
productType, ok := ReceiverRegistry().ProductTypeByID(s.resultType)
if !ok {
	return
}
```

A resumed run then leaves the result as its raw decoded value — the bare URI string — instead of retyping
it into a resource. No error, no log; the receipt quietly stops being a resource.

So this is not a bug the change introduces. It is a **latent fragility the change converts into a live
silent failure across every provider**, and phase 1 fixes it by recording the announced identity on both
sides. Because the failure mode passes a test suite unnoticed, it needs its own pin: a resume that
asserts the *retyped* result, not merely that resume succeeds.

Once both sides agree, the id is `…/service.Resource` without the `*` that today's pointer return
produces. Receipts therefore **do** change, and that is accepted: the alternative is preserving a `*` that
no longer describes anything.

**The delta is narrow.** `ResultType` is `omitempty` and `canonicalIDOf` returns `""` for nil, so a
method with no result omits the field entirely. `file.WalkTree` returns `any` — whatever its reducer
produced — and a reducer returning an int records `int` before and after. **Only receipts whose result is
a provider resource move at all.**

**And error text diverges from document text.** `%T` prints `*service.resource`; the URI says
`service.Resource`. Anyone matching one against the other is now wrong.

## How ruling 6 works

Go compares full method signatures, result types included. So:

```go
type Regular interface {
	Resource
	kind() Regular
}

type Directory interface {
	Resource
	kind() Directory
}
```

`*regular` has `kind() Regular` and therefore **cannot** satisfy `Directory`. And because a type may
declare only one method named `kind`, mutual exclusivity is structural rather than conventional — a
concrete type cannot be two kinds at once. Recursive interfaces are legal Go, and there is no collision:
no file resource has a `Kind` method today, and `ResourceKind` is a separate enum type.

This is why the discriminator is not four differently-named markers. Four names would keep the kinds
distinct but would say nothing about exclusivity; one name with four result types says both, and says it
to the compiler.

**Without a discriminator the kind system collapses silently.** `Regular` and `Directory` have identical
exported method sets — verified 2026-08-24:

```
Regular    Digest Equal Etag Exists MismatchesKind String Unmarshal{JSON,Text,YAML}
Directory  Digest Equal Etag Exists MismatchesKind String Unmarshal{JSON,Text,YAML}
```

As bare interfaces they would be structurally interchangeable, `r.(Directory)` would succeed for a
regular file, and every kind-honesty guarantee established by
[#616](https://github.com/NobleFactor/devlore-cli/issues/616) would evaporate without a single test
failing. This is the load-bearing detail of part 2.

## Epic and issue placement

**Epic: #444 — The resource model (`Epic:ResourceModel`).**
**Feature: [#625](https://github.com/NobleFactor/devlore-cli/issues/625).**

| Part | Phase | Task | Scope |
| --- | --- | --- | --- |
| 1 | 1 | #641 | framework repairs and `service` — the template |
| 1 | 2 | #658 | the generator inspects the implementation, not the interface |
| 1 | 3 | #642 | `git`, `appnet` |
| 1 | 4 | #643 | `json`, `yaml`, `mem`, `function` — the `Unpacker` four |
| 1 | 5 | #644 | `pkg` — the widest footprint |
| 2 | 6 | #645 | `file`'s four variants, discriminated by `kind()` |
| — | 7 | #646 | the rule becomes structurally enforceable |
| — | 8 | #647 | closure — the design record states the contract |

Supersedes #626–#630, which were filed against the original phase shape and are closed as superseded.

Phases are grouped by risk, not by footprint. The `Unpacker` four share one failure mode and are proved
together; `pkg` is alone because it has the widest consumer surface (7 files) and the open question about
exported behavioral fields.

## Phases

### Part 1 — the eight single-resource providers

#### Phase 1 — the framework repairs and `service` — status: complete

Land the framework change from *What ruling 4 costs* first, then prove the whole shape on the provider with
the smallest external footprint: `service`, with **zero** files outside its own package naming the type
(confirmed 2026-08-24 — every other hit is a historical plan document).

The full shape: sealed `Resource` interface, unexported `resource` struct, exported constructors
unchanged in signature, announcement unchanged. `Resource.Name` is a field read 20 times in
`pkg/op/provider/service/provider.go`, so the interface gains a `Name() string` method — all in-package.

**Acceptance — serialization must not move.** A graph planned before and after produces the same
`resources` section for the same intent. Round-trip pins stay byte-identical and a saved document still
reloads.

**The judgment scenario lands here, red first.** A dict supplied where a resource slot is expected mints
an identity-less resource today; it must fail conversion after. Red before the seal, green after —
otherwise the correctness claim is argued rather than demonstrated.

**Landed 2026-08-24 (#641).** Four things the phase established that the plan had not predicted:

1. **The implementation crosses the package boundary by registration, not by argument.** The generated
   announcement cannot name an unexported type, so `op.RegisterResourceImplementation` is called from the
   provider's own init. Ordering is guaranteed by the language — the generated package imports the provider
   package, so Go initializes the provider first.
2. **A sealed provider must also designate its own mint.** An interface asserts no kind, so an authored
   string bound to an interface-typed slot is refused (§5.7 rule 6) unless the interface names a claim type.
   `file` needs the rule because it has four variants to choose between; a single-implementation provider
   designates itself and the question does not arise. Every phase from here needs the same two lines.
3. **`op.Resource` does not expose everything `ResourceBase` provides** — `ReachabilityURI`, `Equal`,
   `Format`, and the marshalers were reachable only because embedding leaked the base's whole surface. Every
   caller is in-package, so tests reach the struct rather than widening the exported contract; a later phase
   with an out-of-package caller will have to decide differently.
4. **A provider's own package announces nothing.** It imports neither its gen package nor the inventory, so
   a test needing the registry populated belongs in `pkg/op/inventory`, not beside the provider.
5. **The generated announcement DOES change — its method metadata.** The plan predicted the gen files would
   be untouched. The announced *type* and *constructor* are indeed unchanged, which is what ruling 4 was for,
   but the generator derives method metadata by inspecting the exported type. Sealing moves the methods onto
   the unexported struct, where the generator cannot see them, so `service`'s three entries became `nil`.
   Harmless there — all three are framework-facing, and one of them is the `TargetConverter` probe this phase
   proves is now unreachable. **The rule for later phases: a method that must be dispatchable has to be
   declared on the sealed interface.** The failure mode is a silently smaller Starlark surface, not a build
   error.

#### Phase 2 — the generator inspects the implementation — status: complete

Finding 5 above is a defect, not a note: sealing silently shrinks a provider's dispatchable surface, and no
stage reports it. `service` lost three framework-facing entries and nothing noticed; the next provider with an
author-facing resource method would lose it just as quietly.

**No build error is possible.** An interface omitting a method its implementation has is well-formed Go. The
interface guard proves the struct satisfies the interface; the converse is not expressible as a type
constraint.

The fix removes the failure rather than detecting it. The emitted metadata is plain strings and never names a
type — only the *inspection target* is the exported interface, incidentally. Inspecting the implementation
instead makes the metadata describe the struct dispatch reflects on, so the two cannot disagree. Ruling 3
makes resolving one name from the other deterministic.

**This lands before the provider sweep** ([#658](https://github.com/NobleFactor/devlore-cli/issues/658)). Otherwise every
remaining phase inherits the same silent risk and needs a manual per-provider review — five chances to miss
one, with no signal when you do. It also narrows phase 7 to asserting the shape rather than policing dispatch.

What it does not settle: whether a method *belongs* on the interface stays a design question per provider. It
stops being a silent regression and becomes an ordinary question about the public contract.

**Landed 2026-08-24 (#658).** The detection needed no type query: only an interface has zero receiver methods,
so emptiness at the announced name is the signal, and the implementation's name follows from ruling 3. A struct
with no methods falls back to itself, which is correct either way. `service`'s three entries returned with no
hand-editing of `*.gen.go`, and no other provider's announcement moved — verified by comparing the metadata
shape of all twelve resource announcements before and after.

#### Phase 3 — `git` and `appnet` — status: pending

One external file and two respectively; neither implements `Unpacker`. Mechanical application of the
phase-1 template.

#### Phase 4 — the `Unpacker` four: `json`, `yaml`, `mem`, `function` — status: pending

These share the failure mode phase 1 repaired, so they are the proof that the repair holds. `mem` is the
content-addressed store, where identity *is* the digest and the fragment is doubly load-bearing;
`function` carries the Starlark-callable path, where bytecode travels in the document's content section.
Each needs its content-section round-trip pinned, not assumed.

#### Phase 5 — `pkg` — status: pending

Seven external files, the widest surface. Answers the plan's standing question: a resource whose exported
behavioral fields consumers read must expose them as interface methods, or those consumers change. Size
it before transforming it.

### Part 2 — `file`

#### Phase 6 — `file`'s four variants become interfaces — status: pending

`AnyKind`, `Regular`, `Directory`, `SymbolicLink` each become a sealed interface over an unexported
struct — `anyKind`, `regular`, `directory`, `symbolicLink` — discriminated by `kind() <Interface>` per
ruling 6. The announcements keep naming `provider.Regular` and friends, so the four fragments stay
byte-identical.

**Settled by the `AnyKind` rename (USER, 2026-08-24; landed ahead of this phase).** The lowercase rule
would have given `any` for the unasserted variant, which is a predeclared identifier: declaring a type by
that name does not fail to compile — it silently rebinds all 139 bare `any` occurrences in the package,
including the constructors' own `value any` parameters. Renaming the exported type to `AnyKind` resolves
it at the source: `anyKind` shadows nothing, and the variant's name now states its assertion (a kind, any
kind) rather than borrowing Go's word for the empty interface.

`op.RegisterResourceMint` gains the four interface→struct registrations. `*AnyKind`'s resolution at
activation (`op.KindResolver`) and the supersession rules from #616 must be re-pinned against interface
types rather than concrete ones — this is where a silent regression would hide.

At the end of this phase the rule holds with no exceptions, and the feature's goal is met.

### Closure

#### Phase 7 — the rule becomes structurally enforceable — status: pending

1. `4.3-resource-registration.md` states the shape as **the contract** for adding a provider resource.
2. A test asserts it structurally: every announced resource type is an interface, and the struct behind
   it is unexported. A future provider that exports its struct fails the suite rather than a review.

Without this the rule is a convention, and conventions decay.

#### Phase 8 — closure — status: pending

The `3.5.x` per-provider design docs drop any language describing the resource as a struct.
`4-resource-management.md` records what the seal buys: the model's guarantees now rest on the compiler
rather than on convention. Status files follow.

## Judgment scenarios

1. **A dict cannot forge a resource.** Supplying a map where a resource slot is expected fails
   conversion instead of minting an identity-less value. Red before phase 1, green after — the scenario
   that makes the correctness claim demonstrable.
2. **A resource cannot be built from outside.** `&service.resource{…}` does not compile beyond its
   package and no outside type satisfies `service.Resource`. A compile-time property, pinned as a
   documented negative.
3. **The `resources` section does not move.** For a given intent it is byte-identical before and after
   every phase. Any drift there is a defect in the phase that caused it, never an accepted consequence.
   Receipt product-type ids are the one deliberate exception — see *What serialization does*.
4. **A resumed receipt retypes its result.** Written, reloaded, and resumed after the change, the result
   comes back as a resource rather than a bare URI string. Asserting that resume *succeeds* is not enough
   — `retypeStampedResult` swallows an unresolved id and returns, so the failure is invisible unless the
   retyped value itself is asserted.
5. **Rehydration still finds the constructor.** A saved document reloads: the fragment resolves through
   the registry to the exported constructor, which returns the unexported struct behind the interface.
6. **`Unpacker` survives.** Each of `json`, `yaml`, `mem`, `function` rebuilds from a document's content
   section after phase 3 — the check that `UnpackerByTypeID` was actually repaired and not merely
   silenced.
7. **A regular file is not a directory.** After phase 6, a `*regular` fails a `Directory` assertion at
   compile time. Ruling 6's whole purpose, and the one that must not regress.
8. **The structural rule bites.** A deliberately non-conforming fixture — exported resource struct, no
   interface — fails phase 7's assertion.

## Verification

Every phase: `make check`, `make vet` under GOOS windows and linux, `gofmt -l`. The Windows baseline is
zero. Serialization is the risk surface.
