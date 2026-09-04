---
title: "Sealed provider resources: every announced resource type is an interface"
issue: https://github.com/NobleFactor/devlore-cli/issues/625
status: in-progress
created: 2026-08-23
updated: 2026-09-04
---

# Plan: Sealed provider resources — every announced resource type is an interface


## Where we are (2026-09-04)

This plan is **thread 2 of four**, worked after the CLI output conventions
([#740](https://github.com/NobleFactor/devlore-cli/issues/740),
[cli-output-conventions.md](740-cli-output-conventions.md)) and before the writ lifecycle surface
([#762](https://github.com/NobleFactor/devlore-cli/issues/762)) and unified configuration
([#441](https://github.com/NobleFactor/devlore-cli/issues/441)).

**Seven of ten phases have landed.** Phases 1-7 are complete: the framework repairs and `service`, the
generator inspecting the implementation, `git` and `appnet`, `json` and `yaml`, `mem` and `function`, `pkg`,
and `file`. Parts 1 and 2 are done: every announced resource type is an interface, and the rule holds with no
exceptions. What remains is closure.

| Phase | Subject | Issue |
| --- | --- | --- |
| 8 | sweep `ConvertFrom` / `CanConvertFrom` | [#649](https://github.com/NobleFactor/devlore-cli/issues/649) |
| 9 | the rule becomes structurally enforceable | [#646](https://github.com/NobleFactor/devlore-cli/issues/646) |
| 10 | closure — the design record states the contract | [#647](https://github.com/NobleFactor/devlore-cli/issues/647) |

Every remaining phase already has an issue. **#649 is next.**

### The thread's other open work

Not part of this plan, and tracked separately under `Epic:ResourceModel`:

- [#635](https://github.com/NobleFactor/devlore-cli/issues/635) — a unit test and a functional test for
  every provider method, one pull request per provider.
- [#597](https://github.com/NobleFactor/devlore-cli/issues/597) — design: the RuntimeEnvironment holds a
  named set of roots, `(root-name, rel)` identity.
- [#735](https://github.com/NobleFactor/devlore-cli/issues/735) — a resource-valued slot re-identifies by
  URI, binding to the current generation. High severity, P1.

### The four items outstanding on the status document

[4-resource-management.status.md](../../architecture/4-resource-management.status.md) records the
construction campaign as **converged** — design and tree agree, no surviving divergence row — and lists
four items that outlive it:

1. **The staged per-type `Resolve`/`Exists` rollout.** `file` is proven and kind-honest; the other eight
   resource-bearing providers await their per-type step. Phases 6 and 7 of this plan touch `pkg` and
   `file`, so the rollout and the sealing overlap and should be sequenced together rather than twice.
2. **Remote-execution filesystem abstraction** (open question §10.1). No owner, no thread.
3. **Run-start claiming for variable-fed resource slots**, ruled 2026-08-22 and explicitly sequenced after
   the resource-construction campaign. The interim posture is a plan-time refusal of plain variables into
   resource-typed slots, the reserved gather `item` frame excepted. That interim is live, so this is not
   blocking anything today.
4. **Judgment scenario 2** — relocate the tree, reconcile at the new root. It is the direct payoff of rel
   identity and stays a recorded prediction **until there is a drivable reconcile surface**, which is
   thread 3's phase 2 ([#762](https://github.com/NobleFactor/devlore-cli/issues/762)). This is a
   cross-thread dependency running thread 3 → thread 2, and it is the reason to expect a return to this
   thread after #762 lands rather than to treat it as finished when phase 10 closes.

**The process this thread is worked under** is
[noblefactor-ops `development-process.md`](https://github.com/NobleFactor/noblefactor-ops/blob/develop/docs/guides/development-process.md):
one open worktree at a time, every issue in it resolved before a pull request, issues logged on discovery
with their resolution site decided at that moment, and every commit updating every document it touches.

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

`tryHydrateStruct` ([pkg/op/convert.go:441](../../../pkg/op/convert.go)) is generic map→struct hydration
inside `op.Convert`. Its guard admits any conversion whose source is a string-keyed map and whose target
has concrete kind `reflect.Struct`. A resource slot declaring `*service.Resource` satisfies that today.
So an author who supplies a dict where a resource is expected gets `reflect.New(concrete).Elem()` — a
freshly minted `service.Resource` with its exported fields filled from the map.

The embedded `op.ResourceBase` has only unexported fields, so it stays zero. `URI()` returns `""`. The
value is not merely unclaimed, it is **identity-less** — and it reaches
[pkg/op/provider/service/provider.go:50](../../../pkg/op/provider/service/provider.go), which reads
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
[pkg/op/provider/service/resource.go:305](../../../pkg/op/provider/service/resource.go) — returns
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
([pkg/op/resource.go:529](../../../pkg/op/resource.go)). Holding to ruling 2's fragment clause would have
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

1. **The two `PointerTo` promotions** — [pkg/op/helpers.go:258](../../../pkg/op/helpers.go) and
   [pkg/op/receiver_type.go:357](../../../pkg/op/receiver_type.go). Both read *"if not a pointer, make it
   one, so pointer-receiver methods are visible."* On an interface that yields `*Resource` — a pointer to
   interface, whose method set is **empty**. Providers that announce method metadata fail loudly at init
   (`parseParameters` → `MethodByName` → `"method Equal: not found on type service.Resource"`).
   `file.AnyKind`, which announces `nil`, fails **silently** with zero methods, because `parseParameters`
   returns an empty but non-nil map and the announced path then matches nothing.

2. **`NewMethod` cannot consume an interface method at all** —
   [pkg/op/method.go:178](../../../pkg/op/method.go). Per `reflect`, an interface type's `Method` has
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
  [pkg/op/receiver_registry.go:960](../../../pkg/op/receiver_registry.go) — which must key on the concrete
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

Checked and *not* a cost: `plannerForType` ([pkg/op/planner.go:147](../../../pkg/op/planner.go)) does the
same nil-interface probe but is only ever called with `metadata.Planner` — never a resource type. And
`deriveMethodParams` ([pkg/op/receiver_type.go:702](../../../pkg/op/receiver_type.go)) already filters
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
([pkg/op/receipt.go:438](../../../pkg/op/receipt.go)). `ProductTypeByID` resolves it through an index built
from **every** action method's **declared** result type
([pkg/op/receiver_registry.go:542](../../../pkg/op/receiver_registry.go)). The two need not agree, and today
they agree only by coincidence:

- `file.Move`, `Remove`, `RemoveAll`, and `Backup` already return the `Resource` interface, so they record
  a concrete dynamic id — `*…/file.Regular`, say. That id resolves because `file.Copy` independently
  *declares* `*Regular` and contributes the key. `*Directory` comes from `Mkdir`, `*SymbolicLink` from
  `Link`. Every kind those four can produce happens to be some other method's declared return.

**Sealing removes the coincidence.** The dynamic type becomes `*…/file.regular`, an unexported struct no
method declares, so nothing contributes the key and the lookup misses. And the miss is **silent** —
`retypeStampedResult` ([pkg/op/recovery_stack.go:647](../../../pkg/op/recovery_stack.go)) reads:

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
| 1 | 4 | #643 | `json`, `yaml` |
| 1 | 5 | #662 | `mem` and `function` — coupled by a cross-package embed |
| 1 | 6 | #644 | `pkg` — the widest footprint, in-package |
| 2 | 7 | #645 | `file`'s four variants, discriminated by `kind()` |
| — | 8 | #649 | sweep `ConvertFrom` / `CanConvertFrom` — dead once every provider is sealed |
| — | 9 | #646 | the rule becomes structurally enforceable |
| — | 10 | #647 | closure — the design record states the contract |

Supersedes #626–#630, which were filed against the original phase shape and are closed as superseded.

Phases are grouped by risk, not by footprint. The `Unpacker` four share one failure mode and are proved
together; `pkg` is alone because it has the widest footprint — 46 field reads, every one in-package, as the
phase-6 sizing found; the "7 files" it was filed with were importers of its action-name constants — and the open question about
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

**This lands before the provider sweep** ([#658](https://github.com/NobleFactor/devlore-cli/issues/658)). Otherwise
every
remaining phase inherits the same silent risk and needs a manual per-provider review — five chances to miss
one, with no signal when you do. It also narrows phase 8 to asserting the shape rather than policing dispatch.

What it does not settle: whether a method *belongs* on the interface stays a design question per provider. It
stops being a silent regression and becomes an ordinary question about the public contract.

**Landed 2026-08-24 (#658).** The detection needed no type query: only an interface has zero receiver methods,
so emptiness at the announced name is the signal, and the implementation's name follows from ruling 3. A struct
with no methods falls back to itself, which is correct either way. `service`'s three entries returned with no
hand-editing of `*.gen.go`, and no other provider's announcement moved — verified by comparing the metadata
shape of all twelve resource announcements before and after.

#### Phase 3 — `git` and `appnet` — status: complete

One external file and two respectively; neither implements `Unpacker`. Mechanical application of the
phase-1 template.

**Landed 2026-08-24 (#642).** Not the mechanical pass the plan expected. Four findings:

1. **The registry name collided, and it is a framework fix, not a provider one.** `receiverName` special-cases
   the type named `Resource` into `<pkg>.Resource`, which is what keeps providers distinct. A sealed
   implementation is named `resource`, falls through to the default arm, and yields the bare `resource` for
   *every* provider — so the second one announced panics at init with `"resource" already announced`. Identity
   now governs the registry name as well as the type id; the implementation governs only reflection. Sealing
   one provider could never have surfaced this; it took a second.
2. **Open question 2 is answered here, not at `pkg`.** Both resources carry exported fields the provider reads
   (`git`: `SourcePath`, `Ref`, `HEAD`; `appnet`: `SourceURL`), so the interface gains accessors. No external
   consumer needed them — every real read is in-package, and the two out-of-package mentions are doc comments.
3. **A codec cannot set unexported fields.** `git` decoded through a `type alias Resource` embed, which
   silently leaves `ref` and `head` at zero once the fields are unexported. Both the JSON and YAML paths now
   decode into an explicit local struct. Same keys, same shape, but the alias trick is not compatible with
   sealing.
4. **`git`'s document form is asymmetric, and was already.** It has no `MarshalJSON`, so the promoted base
   marshaler writes the URI alone — nothing ever writes the `ref`/`head` keys its unmarshaler carefully
   preserves. Behavior preserved exactly here; the asymmetry predates this feature and wants its own issue.

#### Phase 4 — `json` and `yaml` — status: complete

These share the failure mode phase 1 repaired, so they are the proof that the repair holds. `mem` is the
content-addressed store, where identity *is* the digest and the fragment is doubly load-bearing;
`function` carries the Starlark-callable path, where bytecode travels in the document's content section.
Each needs its content-section round-trip pinned, not assumed.

**Landed 2026-08-24 (#643).** The mechanical pass the plan kept predicting and had not yet got: both
providers took the template unchanged, with no framework repair and no surprise. Two things worth recording.

**`mem` had to move out before it started.** `function.Resource` embeds `mem.Resource` cross-package, and Go
cannot embed an unexported type from another package, so sealing `mem` breaks `function` at compile time.
Found by surveying before transforming rather than by the compiler afterwards. It is the only cross-package
resource embed in the tree — `json` and `yaml` embed `op.ResourceBase` alone.

**The `Unpacker` pin needed rewriting to be worth having.** The first version derived its expected set from
`ProviderType()` — the very value a regression corrupts. A reverted repair would make the type stop
implementing `op.Unpacker`, the derived loop would skip it, and the test would pass while content
rehydration was broken. The set is now named explicitly, so a fifth unpacker has to be added deliberately.

#### Phase 5 — `mem` and `function` — status: complete

**`mem` moved here from phase 4 (USER, 2026-08-24): the two cannot be sealed separately.**
`function.Resource` embeds `mem.Resource` cross-package, and Go cannot embed an unexported type from
another package — so sealing `mem` breaks `function` at compile time. Verified as isolated rather than a
pattern: it is the **only** cross-package resource embed in the tree.

The agreed shape has `function.resource` embed the `mem.Resource` *interface* rather than the struct. That
is legal and preserves the method set, but it is a real semantic change: `function` goes from *is-a*
`mem.Resource` by value to *holds-a* one, so storage, zero-value behaviour, and any reliance on the
embedded struct being addressable all shift.


Split from phase 4 (USER, 2026-08-24) because it does not fit the template the earlier phases established.

Six exported fields against `service`'s one, four of them carrying no codec tag. It holds a compiled
`*starlark.Function` whose bytecode travels in the document's content section, so its `Unpack` is the most
load-bearing of the four — rehydration reconstitutes an executable, not just an identity. And `Compiled` /
`CompilerVersion` are runtime state rather than identity, which raises a question about what belongs on a
sealed interface that no other provider raises.

It also carries **the first out-of-package consumer any phase has had to change**:
`pkg/op/provider/plan/lifecycle_api_test.go:774` asserts `.(*function.Resource)` and becomes
`.(function.Resource)`. The other four external references are doc comments.

**Landed 2026-08-25 (#662).** The embed came out without the `mem` mixin constructor the ruling
anticipated — because phase 4's groundwork had already moved the content-address path formula into `op`.
`function` now embeds `op.ResourceBase` like every other provider and reaches its pack through
`op.ContentAddressedPath` / `op.ContentAddressedReader`. Four findings:

1. **`function` inherited nine methods and genuinely used two.** `SourcePath` and the `Hash` field. `Pack`
   and `Unpack` never delegated — they build their own transport envelope — and `sourceBytes` already
   opened its own mmap. The other seven are the per-provider set every sealed provider writes for itself,
   so supplying them was joining the convention rather than paying a cost.
2. **The first out-of-package consumer any phase has had.** `pkg/op/provider/plan/lifecycle_api_test.go`
   calls `ConvertTo` on a transported function, so `function.Resource` declares `CanConvertTo`/`ConvertTo`
   — the first sealed interface to reach past `op.Resource` plus accessors. Every earlier provider's
   consumers turned out to be in-package or doc comments.
3. **`function`'s six exported fields had no external readers at all**, so they went unexported with no
   interface methods. The transport envelope's fields stay exported — that struct is the wire form.
4. **Running the lint gate before pushing caught four issues** a `vet` + `test` check could not see: a
   cyclomatic-complexity ceiling breach in `ConvertTo`, three receiver-naming inconsistencies, and three
   misspellings. The first phase where that ran before the push rather than after CI.

#### Phase 6 — `pkg` — status: complete

Also resolved in this worktree: [#796](https://github.com/NobleFactor/devlore-cli/issues/796) -- the five checked-in package manifests, found on the Windows test.

**Sized 2026-09-04 (#644), before transforming anything.** The "seven external files, the widest surface"
was wrong in the way phase 5's "six exported fields" turned out to be: the surface is wide inside the
package and almost nothing outside it touches the struct.

- **The struct.** `op.ResourceBase` plus three exported fields, `Name`, `Type`, `Version`. Identity is the
  purl, location-keyed, so all three are identity-bearing or requested state: they become interface
  methods `Name()`, `Type()`, `Version()` on a sealed `Resource` over an unexported `resource`, the shape
  every earlier phase set.
- **Outside the package**, two non-generated files import it, lore's `builder.go` and writ's
  `deploy/report.go`, and both use the action-name constants `pkg.Install`, `pkg.Remove`, `pkg.Upgrade`.
  Not one reads a field. Every mention of `pkg.Resource` elsewhere is a doc comment in `pkg/platform`.
  The generated tests read `Name()` and `Type()` of action and receiver metadata, not of the resource.
  **No consumer changes.**
- **Inside the package**, 20 field reads in code and 26 in tests. The code sites are `helpers.go`, which
  builds a purl from the three fields; `provider.go`'s query paths, which read `Name` and `Type`; and one
  **write**: after an install the provider replaces `Type` with the manager's resolved purl type
  (`provider.go:393`, `resource.Type = resolvedType`). That is the one thing sealing has to keep honest.
  The write stays in-package on the struct, reached through the same `.(*resource)` assertion the other
  providers use to reach their struct, and gets a method with a name that says it is a resolution, not a
  free setter.
- **Method signatures.** `Install`, `Remove`, `Upgrade` take `[]*Resource`; `Installed`, `NotInstalled`,
  `Observe`, `VersionGTE` take `*Resource`; `NewReceipt` takes one. All become the interface, and the
  generated files regenerate from the implementation, as phase 2 arranged.
- **Tests.** In-package, so they may construct `&resource{}` directly where a test needs a specific
  shape, and go through `NewResource`/`DiscoverResource` where it needs a catalog entry.

Steps, each a commit only if it needs to be one:

1. `resource.go`: the interface, the struct, `sealedResource`, the three accessors, the resolution method,
   the two constructors returning the interface, the `init` registration pair, the interface guards.
2. `provider.go`, `helpers.go`, `receipt.go`, `observation.go`: the interface in every signature; field
   reads become accessor calls; the write becomes the resolution method.
3. `make generate` regenerates the gen files; the four fragments stay byte-identical (ruling 4).
4. Tests follow; `make check`.

**Landed 2026-09-04 (#644).** The sizing held; one commit. What the transformation showed:

- **The generated files did not change by a byte**, and no generator work was needed: the announcement
  already named `provider.Resource`, which is now the interface, and phase 2's generator reads the
  implementation for the receiver methods. Ruling 4 cost nothing here.
- **The write** is `(*resource).resolveType`, reached from `buildStack` through `resolved(r Resource)
  *resource`, an in-package helper that asserts to the struct and states, via `assert.True`, that a
  foreign implementation is a framework bug. `receiptResource` asserts the same way. The URI is untouched
  by the resolution, so the catalog key stays the purl the user asked for.
- **`DiscoverResource` split**: the exported form returns the interface; an unexported `discoverResource`
  returns the struct for the three unmarshalers, which copy into a receiver they already hold.
- **Tests** reach `ReachabilityURI` and `Equal` through a `concrete(t, r)` helper, the pattern phase 1 set
  in `service`: neither is on [op.Resource], so the sealed interface does not expose them, and widening
  the contract for a test would be the wrong fix. Everything else the tests read is on the interface.
- **No consumer changed**, as sized: the tree built and `make check` passed with edits confined to the
  seven files in `pkg/op/provider/pkg`.

### Part 2 — `file`

#### Phase 7 — `file`'s four variants become interfaces — status: complete

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

**Sized 2026-09-04 (#645), before transforming anything.** Read against develop at 1fc3ea9c, after phase 6.

- **The shape today.** `file.Resource` is already sealed, from #616's first phase: `op.Resource` plus
  `Path()` plus the unexported marker. The four variants are exported structs that each embed the one
  unexported base, `resource` in `resource_base.go`, and add no fields of their own. The base carries the
  package's one exported field, `SourcePath`, read nowhere outside the package. So there are no
  behavioral fields to promote; the work is the four type declarations and everything that names them.
- **Constructors.** `NewRegular`/`DiscoverRegular`, `NewDirectory`/`DiscoverDirectory`, and
  `NewSymbolicLink`/`DiscoverSymbolicLink` return the struct pointer; each returns its variant interface.
  `DiscoverAnyKind` already returns `Resource` and is untouched.
- **Registration.** One mint exists today, `Resource → *AnyKind` in `planspace.go`. The phase adds the four
  implementation registrations and the four interface→struct mints the plan names. The kind resolver on
  any-kind is reached in the catalog by an interface assertion, and `Supersede` takes `op.Resource` values,
  so the concrete-type change is invisible to both; the re-pinning the plan warns about lives in
  `kind_resolution_test.go` and `planner_test.go`, which model the any-kind shape "without the filesystem"
  and are read against the interface forms.
- **Outside the package**, the pointer type is held in three consumer packages and one test: `encryption`
  (`DecryptSopsFile` and `EncryptFile` take and return `*file.Regular`; two compensations and `receipt.go`
  assert it), `archive` (`Extract` takes `source *file.Regular`), `starcode` (a tree-walk reducer asserts
  `entry.(*file.Directory)`), and `encryption/provider_test.go`. Everything else naming a variant outside
  the package is a doc comment, in five files. **Ruled 2026-09-04 (USER): this worktree edits those
  sites.** A pointer type becomes the interface of the same name; the change is mechanical; the phase cannot
  compile without it, and splitting it off would leave the tree red between two PRs.
- **Inside the package**, the pointer forms appear roughly 140 times in code and 34 in tests, many in
  error strings that keep their text. The methods outside the interfaces — `BindRoot`, `IsDir`,
  `MismatchesKind`, `String`, `Equal`, `ConvertTo`, the unmarshalers — stay on the base struct; in-package
  callers reach them through the struct, tests through a `concrete` helper as `pkg` and `service` do.
  `BindRoot` is reached by the executor through `op.RootBinder`, an interface assertion, so it is unaffected.
- **Generated files.** The four announcements name `provider.AnyKind`, `provider.Regular`,
  `provider.Directory`, `provider.SymbolicLink`, the names the interfaces keep. Expected byte-identical, as
  `pkg`'s were.
- **The discriminator** is `kind() Regular` and its three siblings on the unexported structs, per ruling 6;
  the base's `sealedResource()` stays where it is. #645's body still uses the pre-rename `Any`/`any`; the
  names settled above govern.

Steps, each a commit only if it needs to be one:

1. `regular.go`, `directory.go`, `symbolic_link.go`, `any_kind.go`: the interface, the unexported struct,
   `kind()`, the constructors returning the interface; `planspace.go`: the registrations.
2. In-package readers to the interfaces; the `concrete` helper in the tests.
3. The consumer sites, under the ruling: `encryption` (five sites and the test), `archive` (one),
   `starcode` (one).
4. `make generate`; the four fragments stay byte-identical (ruling 4).
5. Tests follow, the kind-resolution and planner tests read against the interface types; `make check`.

**Landed 2026-09-04 (#645).** One commit after the sizing. Where the transformation and the sizing disagreed:

- **The sizing was wrong on the field.** `SourcePath` is read outside the package — eight sites in `encryption`,
  three in `archive` — and two more test files in `plan` held `*file.Regular`. The survey's filters hid them
  (an enumeration error, the capped-grep kind). All became `Path()`, which the base interface already had, under
  the consumer-edit ruling. `archive` also called `IsDir()` on a `Directory` after `Exists()`; a `Directory`'s
  `Exists` is kind-honest, so the second check was unreachable and the two folded into one refusal.
- **The framework change landed in `convert.go`, not the catalog.** The plan-time interconvertibility probes
  (`sourceSideAdvertises`, `targetSideAdvertises`) inspect a type's method set, and an interface type has none
  of the struct's converter methods, so binding a `file.Directory` output to a `string` slot was refused at
  validation — three tests caught it. `probeTypeFor` resolves a registered sealed interface to
  `*implementation` before probing. This is the third framework path an interface-typed resource breaks, after
  #641's two; it had not bitten earlier because no graph binds a `service` or `pkg` output to a string slot.
- **Kind resolution and supersession needed no change.** The catalog reaches the resolver by interface
  assertion. `internEntry`'s probe for an unasserted entry asserts `AnyKind`, the interface, and the any-kind
  tests assert the ledger holds `Regular` and `AnyKind` by interface — the re-pinning the plan asked for.
- **Registrations.** Four implementations and five mints, in `resource.go`'s `init`; the base's mint moved
  there from `planspace.go` to sit beside them. The plan-path normalizers register the interfaces, because the
  parameter type is what the planner looks up.
- **Constructors return an explicit nil on error.** The earlier phases return a typed nil through
  `return discoverResource(...)` — logged as [#807](https://github.com/NobleFactor/devlore-cli/issues/807).
  The cross-kind collision error now prints the struct names via `%T` — logged as
  [#808](https://github.com/NobleFactor/devlore-cli/issues/808), for #649's sweep.
- **Tests** reach struct-only members (`MismatchesKind`, `Equal`, `ReachabilityURI`) through a generic
  `concrete[S]` helper; two test helpers were renamed to make room for the unexported `discoverDirectory` and
  `discoverSymbolicLink`.
- **Generated files** are byte-identical to develop for `file`, `encryption`, `archive`, and `starcode`.

### Closure

#### Phase 8 — sweep `ConvertFrom` / `CanConvertFrom` — status: pending

[#649](https://github.com/NobleFactor/devlore-cli/issues/649), scheduled here rather than per-phase (USER,
2026-08-25). Sealing is what makes a resource's `TargetConverter` pair unreachable: `op.Convert` step 7
probes `reflect.New(target)`, and on an interface that yields a pointer-to-interface with an empty method
set. Once every provider is sealed, all sixteen methods are dead **at once**, so the sweep is pure
dead-code removal rather than a behaviour change threaded through five separate PRs.

**Sixteen methods, eight pairs, five providers** — `appnet`, `git`, `service` (dead since their phases),
`pkg` (phase 6), and `file`'s four variants plus its base (phase 7). Nothing else goes.

The framework seam **stays**: `op.TargetConverter`, `tryTargetConverter`, `targetSideAdvertises`, and the
cached type are all live for non-resource targets — `(*OrderingEdge).CanConvertFrom` is a real production
implementor with nothing to do with resources, and `TestConvert_TargetConverter` keeps exercising step 7.

Two things make this safe rather than merely tidy, and both are evidence rather than argument:

1. **The contract already declares itself optional for resources.** `pkg/op/interfaces.go:65` says
   implementers *"commonly omit `TargetConverter` when their natural source is content bytes or a parsed
   structure."* Four resources — `json`, `yaml`, `mem`, `function` — have never implemented it.
2. **The plan-time worry is answered.** `CanConvertFrom` is `typesAreInterconvertible`'s only bridge for
   `string ↔ Resource`, and removing it was expected to turn a tolerated slot collision into a hard one.
   Sealing already removed it for five providers, and the suite, the scenarios, and three platforms stayed
   green. Nothing depended on it.

Distinct from [#661](https://github.com/NobleFactor/devlore-cli/issues/661), which is the opposite shape: a
documented override contract that **zero** implementors honour.

#### Phase 9 — the rule becomes structurally enforceable — status: pending

1. `4.3-resource-registration.md` states the shape as **the contract** for adding a provider resource.
2. A test asserts it structurally: every announced resource type is an interface, and the struct behind
   it is unexported. A future provider that exports its struct fails the suite rather than a review.

Without this the rule is a convention, and conventions decay.

#### Phase 10 — closure — status: pending

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
7. **A regular file is not a directory.** After phase 7, a `*regular` fails a `Directory` assertion at
   compile time. Ruling 6's whole purpose, and the one that must not regress.
8. **The structural rule bites.** A deliberately non-conforming fixture — exported resource struct, no
   interface — fails phase 8's assertion.

## Verification

Every phase: `make check`, `make vet` under GOOS windows and linux, `gofmt -l`. The Windows baseline is
zero. Serialization is the risk surface.
