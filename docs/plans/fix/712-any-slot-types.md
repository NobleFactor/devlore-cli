---
title: "Slots must carry their type"
issue: https://github.com/NobleFactor/devlore-cli/issues/712
status: in-progress
created: 2026-08-28
updated: 2026-09-01
---

# Plan: Slots must carry their type

## Summary

A value in an `any`-typed slot loses its concrete Go type when the document is written. JSON's encoder emits
the shortest form that round-trips a `float64`, so `42.0` is written `42`; it emits a `[]byte` as a base64
string, so bytes come back as a different value entirely. This plan gives every value in an `any` slot a type
envelope in the document -- MongoDB Extended JSON's shape, a single `$`-prefixed key naming the type --
envelopes non-finite floats wherever they appear, makes an unenveloped value a read error rather than a
guess, and removes a `json.Number` leak that #713 introduced at the same seam.

## Goals

- [ ] A float in an `any` slot reloads as a float; an integer reloads as an integer.
- [ ] A `[]byte` in an `any` slot reloads as the bytes that were written.
- [ ] A `Resource` in an `any` slot reloads as a Resource, not as its URI string.
- [ ] A resource-valued slot binds to the generation it was written against, not the current one.
- [ ] A non-finite float saves and reloads in both codecs, rather than failing in one, at every float
      position rather than only in `any` slots.
- [ ] A value whose field has a declared type is *not* enveloped; the declaration already answers.
- [ ] An integer above 2^53 survives an `any` slot with every digit intact.
- [ ] An unenveloped value in an `any` slot is an error, not a guess.
- [ ] An `any` position nested in a container carries its type at every depth.
- [ ] JSON and YAML agree, in both directions.
- [ ] No decoder-internal type (`json.Number`) is observable to a provider, a guard, or Starlark.
- [ ] The envelope is documented as part of the document format, not left implicit in an encoder.

## Current State

Seven findings. Only the first is in the issue as filed.

### 1. The type is destroyed at save (the filed defect)

`json.Marshal(float64(42))` emits `42`. `yaml.Marshal(float64(42))` emits `42` as well, so this is not a
JSON-specific defect. With no declared type at either end, nothing downstream can recover the distinction.

### 2. `[]byte` is corrupted, not merely retyped

`starlark.Bytes` converts to `[]byte` at `pkg/op/starlarkbridge/converter.go:444`, and `json.Marshal` writes a
`[]byte` as a base64 **string**. Bytes in an `any` slot reload as a `string` whose contents differ from what
was written. This is worse than the numeric case: the value changes, not just its type.

### 3. A `Resource` is mis-identified on reload, in two different ways

`ResourceBase.MarshalJSON` is `json.Marshal(b.uri)` and `ResourceBase.MarshalYAML` returns `b.uri`
(`pkg/op/resource.go:307,333`), so every Resource serializes to its URI. What happens on the way back depends
on whether the slot has a declared type, and neither answer is right:

| Slot | On load | Binds to |
| --- | --- | --- |
| A declared resource type | its `UnmarshalJSON` reaches `Discover(uri)` | the **current** generation |
| `any` | no target type, so the URI stays a `string` | nothing; never resolved |

The **`any` case** is what #712 was filed for: the value reaches the provider as a string indistinguishable
from one the author typed.

The **declared case is worse**, and was not known when this plan was written. `ResourceCatalog.entries` is an
append-only ledger, so every generation of a resource persists under its own id, while `ns` maps a URI to only
the *current* generation and `Discover` resolves through `Current(uri)` (`resource_catalog.go:244`). A URI
therefore does not name a resource -- it names whichever generation is current at the moment of the lookup. A
graph reloaded after a resource has been re-produced binds to a generation it never saw, while the generation
it actually used is still in the ledger under its original id.

This is not confined to dispatch. Three paths re-identify by URI, and the codec one is reached first:

| Path | Location | Reached from |
| --- | --- | --- |
| `discoverResource` | nine providers' `UnmarshalJSON`/`UnmarshalText`/`UnmarshalYAML` | deserialization |
| `resolveDispatchResource` | `method.go:807` | dispatch; one production caller, `method.go:725` |
| `resolveRecordedResource` | `recovery_stack.go:678` | receipt rehydration on resume |

The third is the sharpest: it resolves a **recorded result** -- what a node actually produced on the original
run -- so a resumed run can bind a generation that did not exist when the node ran. A produced resource is a
historical fact with exactly one right entry.

The second and third are both gated on the target or product type implementing [Resource], so neither fires
for an `any` slot at all, which is the gap finding 3's first row describes.

**Ruled: both are fixed at once** (filed as #735). Fixing the `any` case alone would leave one slot
resolving correctly by id while the declared slot beside it drifts to the current generation -- one
graph giving two different answers about which resource it means.

### 4. A non-finite float cannot be saved as JSON at all

Starlark's `float()` is `strconv.ParseFloat` (`starlark/library.go:435`), which accepts `inf`, `-inf`, and
`nan`; the converter passes a `starlark.Float` through as a `float64` with no finiteness check
(`converter.go:440`). A non-finite float therefore reaches an `any` slot.

The two codecs then disagree completely:

- `encoding/json/encode.go:572` refuses -- `math.IsInf(f, 0) || math.IsNaN(f)` raises an
  `UnsupportedValueError` and the save fails outright.
- `yaml.v3/encode.go:401-405` emits `.inf`, `-.inf`, and `.nan`, and succeeds.

The same graph saves as YAML and fails as JSON, which the "JSON and YAML agree" goal forbids.

Unlike findings 1 through 3 this failure is loud, so nothing is silently corrupted. It is in scope because the
envelope is what fixes it: a string payload carries `Infinity` where a JSON number cannot. This is exactly why
Canonical Extended JSON keeps a wrapper for non-finite doubles while dropping it for finite ones, and it makes
the payload's encoding a correctness question rather than a matter of taste.

### 5. A `json.Number` now leaks into `any` slots -- a live regression from #713

#713 gave the JSON branch `decoder.UseNumber()`, then read the literal against the *declared* type of the
parameter it fills. An `any` parameter has no declared type, so:

- `tryParseSerializedNumber` switches on `target.Kind()`; for `reflect.Interface` no case matches.
- `convertDirect` returns the value untouched through its empty-interface early return.

An `any` slot therefore reloads holding `json.Number` where it previously held `float64`. Two consequences
are confirmed by reading the code:

- **The graph cannot be loaded at all** (filed as #734). The canonical form a checksum covers is built
  from the RELOADED values, so a number returning as a `json.Number` changes the recomputed checksum and [LoadGraph]
  rejects the document as tampered. Phase 1 measured this: an `any` slot holding a string or a bool reloads
  cleanly, while one holding an integer or a float fails outright. This is not a silent type loss -- **every
  graph with a number in an `any` slot is unloadable on `develop` today**, and it is the most severe
  consequence of this regression rather than the subtlest.
- **Truthiness inverts for zero.** `IsTruthy` tries `scalarTruthy`, which has a case for every numeric type
  and for `string` -- but `json.Number` is a *named* type, so `case string:` does not match it and the helper
  reports "not a scalar". Control falls to the reflect switch, whose `Kind()` is `String`, which no case
  lists, so it lands on `default: return true`. `json.Number("0")` is **truthy**; `float64(0)` is falsy. A
  resumed `plan.choose` on a round-tripped zero takes the wrong branch.
- **There is no path back to Starlark.** `starlarkbridge` has no conversion for `json.Number`.

Scope is set by which decoder is used. `LoadGraph` is the only `UseNumber` call, so graph documents are
affected. `recovery_stack.go` uses plain `json.Unmarshal`, so receipt results still reload as `float64`; they
escape finding 5 but remain subject to findings 1 through 4.

### 6. A large integer in an `any` slot loses digits on resume

`recovery_stack.go` decodes with plain `json.Unmarshal` into `Result any` and `Entries []any`. Without
`UseNumber`, `encoding/json` decodes every JSON number reaching an `any` as a `float64`, which represents
integers exactly only to 2^53. An `int64` result of `9007199254740993` reloads as `9007199254740992`.

This is a *value* corruption rather than a type change, it is silent, and it happens on resume -- the path
whose whole job is restoring what already ran.

The envelope fixes it independently of which decoder is used: `{"$int64": "9007199254740993"}` is a string, so
no `float64` ever holds the value. This is the third independent reason for a string payload, alongside
finding 4's non-finite floats and decision 1's tooling normalization.

### 7. The seams are wider than the issue records

`any`-typed fields that are actually serialized:

| File | Field | Reaches |
| --- | --- | --- |
| `pkg/op/node.go:641` | `bindingData.Value` | graph documents |
| `pkg/op/variable.go:94` | `Value` | `plan.Variable` defaults |
| `pkg/op/receipt.go:716` | `Result` | compensation |
| `pkg/op/receipt.go:718` | `Slots` | compensation |
| `pkg/op/recovery_stack.go:198` | `Result`, `Entries` | resume |
| `pkg/op/origin.go:144` | `Annotations` | graph documents |

Receipts and the recovery stack feed compensation and resume. A float that reloads as an integer there
changes a *compensating* call, which is worse than changing a forward one. One shared treatment at every
`any` seam, not a local patch at `bindingData`.

## Work in progress — state as of 2026-09-01

**Branch:** `fix/712-any-slot-types`, in worktree `devlore-cli.712-any-slot-types`. **Committed:**
`b17067b` (Phase 1: the plan and nine failing tests) and `ff33a2f` (the type wrapper, wired at the graph
slot seam). No code changes are outstanding; the scratch tests and the `test_path_*` litter this section
once listed are gone.

**Renamed 2026-09-01** from `feature/any-slot-types`, which named no issue. A plan is now
`docs/plans/<type>/<issue>-<name>.md` and a branch is `<type>/<issue>-<name>`, so the two share a string
([#769](https://github.com/NobleFactor/devlore-cli/issues/769)). `develop` was merged at the same time —
the branch was four behind, and it has already needed one catch-up merge.

### Where this sits among the four threads

This is **thread 2**, resource management, and it is sequenced **after
[#649](https://github.com/NobleFactor/devlore-cli/issues/649)** — the `ConvertFrom` sweep — rather than
before it.

Decision 8 makes an uncataloged resource a hard error at save:

```
op.encodeTypeWrapper: resource "…" is not cataloged
```

and #649 is precisely the defect that mints such resources: every provider's `ConvertFrom` returns a
value whose embedded `op.ResourceBase` stays zero, so `URI()` returns `""` — eight sites across `appnet`,
`git`, `pkg`, `service`, and `file`'s four variants. Landing this plan first would turn a latent defect
into a save-time failure across every provider, found mid-phase. Landing #649 first makes this error path
unreachable by construction rather than by each consumer's good manners.

The thread's order is therefore #644 → #645 → #649 → **#712** → #646 → #647.

[#735](https://github.com/NobleFactor/devlore-cli/issues/735) is the same seam seen from the other side —
a resource-valued slot re-identifying by URI — and belongs to this thread too.

**What does not fold in:** [#734](https://github.com/NobleFactor/devlore-cli/issues/734) is a checksum
mismatch on a *number* in an any slot, and [#758](https://github.com/NobleFactor/devlore-cli/issues/758)
and [#661](https://github.com/NobleFactor/devlore-cli/issues/661) are document-format defects. Those
three remain an `Epic:Serialization` cluster belonging to no thread, which is recorded here rather than
forced into this one.

### The tree is red, deliberately and not

Three executor tests fail: `TestRun_AKindMismatchStopsEvenUnderIgnore`,
`TestRun_ScopedPreflightFailsOnConsumedMissingClaim`, `TestRun_ScopedPreflightVerifiesConsumedClaims`. All
three fail with `op.encodeTypeWrapper: $resource ... has no catalog to name it` — the write side reaches for
`resource.RuntimeEnvironment().ResourceCatalog`, which is the wrong catalog.

Six of the nine Phase 1 tests now pass. Still red: the truthiness guard (task 15), the Resource case, and the
recovery-stack seam.

### What `ff33a2f` carries

| File | What it carries |
| --- | --- |
| `pkg/op/type_wrapper.go` | New. The wrapper codec. Its own tests pass. |
| `pkg/op/type_wrapper_test.go` | New. Round trips over both codecs, all green. |
| `pkg/op/node.go` | `marshalBindings` / `assembleBindings` wrap and unwrap; signatures changed. |
| `pkg/op/graph.go` | `assembleUnits` takes the environment. |
| `pkg/op/binding_test.go` | Call sites updated for the new signatures. |

### The correction to apply first

`encodeResource` asks `resource.RuntimeEnvironment().ResourceCatalog` to `Resolve` the resource into an id.
It should read `resource.ID()`. A cataloged resource carries its own catalog id -- the catalog stamps it at
catalog time -- so the write side needs no catalog at all. Decision 8's "A cataloged resource names itself"
carries the ruling and the six-line replacement. The structural question that stood open here -- how a node
marshaling standalone reaches `graph.resourceCatalog`, given `Node.MarshalYAML() (any, error)` takes no
context -- dissolves: it never needed to.

Decoding is not symmetric. It holds an id and must find what the id names, so it does need a catalog.
`assembleUnits` / `assembleNode` / `assembleBindings` thread `env.ResourceCatalog`; the graph's own unpacked
catalog is probably the right one. Still open.

### What the devlore-test investigation established

Run against real documents rather than reasoned about, and all of it now backed by artifacts:

- **A graph's immediate slots hold literals.** A resource produced by a node travels as a `promise` carrying a
  unit id. Confirmed against a two-node write-then-remove plan.
- **A resource DOES reach an immediate slot** when the author passes a path to a `Resource`-typed parameter:
  plan-time coercion mints the entry, `resources:` gains `id: res-1`, and the slot records the **URI**. That
  document is #735 in one screen — the id the slot should use is eleven lines below it.
- **Passing a path instead of a promise drops the dependency edge.** No `edges:` key is emitted, nothing
  orders the consumer after the producer, and scoped pre-flight then fails the consumer's claim. Not filed;
  see Open Questions.
- **`mode: 420`** appears in a planner-produced document — #711 in the wild.

### Blocked on

- **#738** — `devlore-test` is broken (High/P1). Its `graph.yaml` is empty for any plan whose units return
  nil, the graph document is written to the stream named `receipt`, and no execution trace is persisted. That
  last point blocks observing a resource's `Pending` to `Active` transition, which is a test this plan needs.
- **A missing accessor.** `GraphExecutor` exposes no `RuntimeEnvironment()`, so a test cannot reach the run's
  catalog to assert resource state. Whether that is a gap or an intended restriction is unanswered.

## Requirements

### Requirement 1: Every value in an `any` slot is enveloped

The document stores the type alongside the value. This is unconditional -- it does not depend on whether JSON
could have inferred the type on its own.

### Requirement 2: A non-finite float is enveloped wherever it appears

`Infinity`, `-Infinity`, and `NaN` are enveloped at every float position, declared or not. This is the one
place the envelope is not about an unknown type, so it does not follow the `any` scoping.

### Requirement 3: An unenveloped value is an error

Reading a bare value where an `any` slot expected an envelope is a malformed document. The reader reports it
and refuses; it does not infer a type. Inference is what produced this defect.

### Requirement 4: A number's category survives the round trip

A `float64` reloads as `float64`, an `int64` as an integer, through a graph document, a receipt, and the
recovery stack, in JSON and in YAML.

### Requirement 5: No decoder type escapes the codec

`json.Number` is an artifact of how the document was read. It is resolved to a concrete Go value before any
value reaches a provider, a guard, `IsTruthy`, or `starlarkbridge`.

### Requirement 6: The envelope is documented

A reader of a document can predict what a given value reloads as without reading the encoder.

## Design decision: Extended JSON's shape, only where the schema cannot answer

Three decisions, taken together.

### 1. An envelope rather than syntax

The alternative was to let syntax carry the type -- write an integral float as `42.0` and read a decimal point
as "float". Three reasons it does not hold up:

1. **It only ever addressed numbers.** A declared type carries itself, which is what #711 implements. The only
   place a type is genuinely unknown is an `any` slot, and there *everything* is unknown. A syntactic trick
   that happens to work for numbers leaves every other case unanswered -- findings 2 and 3 among them.
2. **It cannot express finding 2 at all.** Syntax has no way to distinguish a string from a base64 string.
3. **Syntax does not survive tooling.** To a generic JSON parser `42.0` and `42` are the same number. `jq .`,
   a formatter, an editor's format-on-save, and our own canonical marshal (`graph.go:850,865`) are each
   entitled to normalize one to the other. A format whose semantics evaporate when a well-behaved tool
   reformats it is not a format. An envelope is data, so it survives any JSON-preserving transform.

### 2. The shape: one `$`-prefixed key that names the type

Adopted from MongoDB Extended JSON, whose specification calls this a **type wrapper object**: "a JSON value
consisting of an object with one or more `$`-prefixed keys that collectively encode a BSON type and its
corresponding value using only JSON value primitives." The type name *is* the key; there is no separate
`$type` field.

Chosen over the `{"$type": "float", "value": 42}` shape the issue sketches. One key rather than two, which
compounds under the recursion below:

```json
{"$type": "map", "value": {"a": {"$type": "float", "value": 42}}}   // the issue's sketch
{"$map": {"a": {"$float64": "42.0"}}}                                // adopted
```

The type names are Go's, not BSON's. Extended JSON's vocabulary is `$numberInt`, `$numberLong`,
`$numberDouble`, `$binary`, and `$oid`; `$numberLong` for an `int64` would not read to anyone working in this
codebase. The names, alphabetized, are `$bool`, `$bytes`, `$float64`, `$int64`, `$list`, `$map`, `$nil`,
`$resource`, and `$string` -- one per Go type the converter can produce. Integer widths other than `int64` do
not arise from the Starlark path (`converter.go:430` narrows every `starlark.Int` to `int64`); if Go-side code
puts one in an `any` slot, its name follows the same pattern.

The payload is a **string** for numeric types. That is a correctness requirement, not a preference: finding 4
shows a bare JSON number cannot carry `Infinity` at all, and a bare `42.0` can be renormalized to `42` by any
conforming tool. Canonical Extended JSON encodes every numeric payload as a string for both reasons.

### 3. Where envelopes appear: only where the schema cannot answer

MongoDB wraps pervasively because BSON has **no schema** -- every document must be self-describing, since
there is no field definition anywhere to consult. We have field definitions, and #711 already reads a
serialized number against the declared type of the field it fills. Enveloping a declared field would re-encode
information we already hold, and would put a second source of truth in the document that can disagree with the
first.

So an envelope is written in exactly two circumstances, justified differently:

| Circumstance | Why | Where it applies |
| --- | --- | --- |
| A value in an `any` position | The **type** is unknown; no declaration exists there | `any` positions only |
| A non-finite `float32` or `float64` | The **value** is unrepresentable in JSON | every float position |

The second is not an `any`-slot concern and must not be implemented as one. A plain declared `float64` field
holding `+Inf` fails to save today with no `any` involved (finding 4), so the rule applies at every float
position in the document, declared or not.

This lands on MongoDB's two modes applied per *position* instead of per *document*: Canonical behavior where
we have no schema, Relaxed behavior where we do -- bare numbers, keeping a wrapper only for the non-finite
values Relaxed also wraps. We can choose per position precisely because we have the linkage BSON lacks, so
this is better than either mode rather than a compromise between them.

### 4. Within an `any` position, without exception

Every value in an `any` position is enveloped, including those JSON could have determined on its own -- a
string becomes `{"$string": "hi"}`. This is where we deliberately diverge from Extended JSON, which leaves
bare strings, bools, and nulls unwrapped. MongoDB can do that safely because BSON's type set is closed: every
string-ish BSON type has a wrapper, so a bare string is unambiguously a BSON string. Our `any` values include
arbitrary Go types, Resources among them, so that closure does not hold for us.

Uniformity buys three things a partial rule does not:

- **No rule to misremember.** There is no table of which types are bare.
- **No reserved-key collision.** An author's own map is always the payload *of* an envelope, so it can never
  be mistaken *for* one. A partial rule would need an escape hatch for exactly this.
- **A checkable invariant.** A validator can assert that every `any` position holds an envelope. Under a
  partial rule a bare value is legal, so a missing envelope is undetectable -- which is Requirement 3's whole
  point.

The cost is verbosity: a string or a bool in an `any` position carries an envelope it did not strictly need.

### 5. Precision, exactly stated

Go's own round trip is exact for floats. `encoding/json` formats them with
`strconv.AppendFloat(b, f, fmt, -1, bits)` (`encode.go:591`), and precision `-1` "uses the smallest number of
digits necessary such that ParseFloat will return f exactly" (`strconv/ftoa.go:55`). A `float64` and a
`float32` survive save and reload bit-for-bit. A declared integer field is written with all its digits and
read back into the same width exactly.

| Path | Exact? |
| --- | --- |
| A `float32` or `float64`, any position | yes -- shortest round trip |
| A declared integer field, our own reader | yes -- full digits, same width |
| An integer in an `any` slot, plain `json.Unmarshal` | **no** -- becomes a `float64`, lossy above 2^53 |
| A declared `int64` read by an external JSON tool | **no** -- JSON numbers are doubles to most consumers |

Row three is finding 6, and the envelope removes it: a string payload never passes through a `float64`.

Row four we accept. A declared `int64` is written as a bare JSON number, so a JavaScript-based tool that
reformatted the document would round it past 2^53. Canonical Extended JSON avoids this by making every numeric
payload a string everywhere. The alternative taxes every integer in every document to defend against a tool we
do not ship, and the declared type means our own reader is never confused. This narrows decision 1's "syntax
does not survive tooling" argument to `any` positions, where it still holds in full, and it is accepted here
explicitly rather than left to be discovered later.

### 6. Containers: the envelope is recursive

An `any` position nested inside a container is still an `any` position, and containers are where most of them
live: `receipt.Slots`, `receipt.Annotations`, `plan.Plan`'s `args` and `kwargs`, `template.RenderText`'s
`data`, and everything `naturalDict` and `naturalSequence` produce. A Starlark dict reaching an `any` slot is
a `map[string]any` whose every value is exactly as untyped as a top-level `any`. So the envelope applies at
every depth:

```json
{"$map": {"a": {"$float64": "42.0"}}}
```

Enveloping the container alone would round-trip the map and lose every element inside it, which is the same
defect one level down.

A container whose element type is *declared* is not enveloped internally. A `map[string]string` carries its
values' type in the field itself, so its elements stay bare. Only `any` positions are enveloped, which keeps
the verbosity proportional to how much of the document is genuinely untyped.

The cost is real and worth stating: a nested structure envelopes at every level, so a dict of lists of numbers
is heavily wrapped. A homogeneous shorthand -- enveloping once at the container as `list<int>` when every
element shares a type -- would cut it, at the price of a second form to parse and a rule for when each
applies. Not adopted here. It is a compression of a correct format, available later if verbosity becomes a
real complaint, and it does not change what the format means.

### 7. Where the resolution happens: two authorities, and no guessing

The governing rule: **we serialize with full knowledge of the field type, so we deserialize with full
knowledge of the field type.** There is no third state in which the reader has to work out what a value is by
looking at it. If the reader is ever reduced to inspecting a literal's shape, the writer failed to record
something it knew, and the fix belongs on the write side.

Two authorities can answer "what type is this?", and each is consulted where it lives:

| Value | Who answers | Resolved at |
| --- | --- | --- |
| Enveloped (an `any` position) | The **document** -- the envelope names the type | the decoder, in `graph.go` |
| Bare literal (a declared field) | The **parameter** -- only the field knows | `readAgainstField`, in `node.go` |

The decoder is the right home for the envelope and the wrong home for everything else.
[readAgainstField]'s own doc comment states the constraint that decides this: "Neither document can answer
what the value IS; only the parameter can." Resolving a bare literal at the decoder would guess ahead of the
declared type, which is precisely what #713 introduced `UseNumber` to stop doing. Resolving an envelope at
`readAgainstField` would be equally wrong in the other direction, since the envelope is the document
answering and no parameter needs consulting.

Resolving envelopes at the decoder is also what repairs finding 5's checksum failure at the root: the
reloaded value is a concrete `float64` again, so the canonical form matches what the in-memory graph
produced.

A second, independent reason the envelope must resolve at the decoder: `readAgainstField` handles **numbers
only, deliberately.** Its comment records why -- handing every value to [Convert] reaches the
resource-construction step, which would build a Resource from a URI string at LOAD time, forbidden by §5.6 of
docs/plans/resource-construction.md, and would move the checksum. The envelope's Resource case therefore
cannot be reached by widening `isDecodedNumber`. It has to resolve where no [Convert] call is involved.

#### No guessing means the leaks become errors

`json.Number` escapes `readAgainstField` through four routes, each ending in `return value` unchanged. Under
the rule above, none of them may quietly hand a decoder artifact forward:

| Route | Under this plan |
| --- | --- |
| `method == nil` -- no action, so no declared type | an error: nothing to read against |
| `!declared \|\| parameter.Type == nil` | an error, same reason |
| [Convert] returns an error | an error, raised at load |
| An `any` parameter | cannot arise: enveloped, and resolved at the decoder |

All four pass the value through unchanged today, which is how a `json.Number` reaches the runtime at all.

Row 3 is a deliberate behavior change worth calling out. Today the failure is swallowed so it surfaces at
dispatch "naming the parameter and its type rather than here naming a slot the author never wrote." That
deferral was harmless when the value passed through as an ordinary number; it is not harmless when what
passes through is a `json.Number`. The error moves to load time, and the load-time message must name the
slot, the parameter, and the declared type so nothing is lost by the move.

### 8. Resource identity is the catalog id, not the URI

A graph slot can hold a Resource even though nothing in a graph has executed. An author's Starlark calls a
provider constructor at plan time, that constructor mints the entry through `Discover`, and
[marshalBindings] writes the Resource into `immediateData.Value`. Plan-time construction is not execution.

The id is `res-N` from a monotonic counter (`resource_catalog.go:825`) -- assigned by order rather than
derived from the resource. It is stable where it has to be: `LedgerEntrySnapshot` carries each entry's `ID`,
`ResourceLedgerSnapshot.NextID` is serialized, and `Rehydrate` restores both. The graph is immutable and its
ledger is saved to the trace, so an id written into a graph names the same entry every time that graph loads.

That the id is stable *relative to its ledger* rather than globally is the point, not a limitation. A URI is
globally meaningful and therefore cannot distinguish generations; an id names one entry in one ledger, which
is precisely what a slot needs to say.

Only the id is stored. Recording the URI beside it would put a second source of truth in the document that can
disagree with the first -- the same objection that keeps a declared field un-enveloped in decision 3.

The two questions are orthogonal, which is what lets "slots need the same treatment" hold without disturbing
the scoping rule:

| Question | Answer | Applies to |
| --- | --- | --- |
| What **type** is this value? | the wrapper | `any` positions, plus non-finite floats |
| Which **resource** is this? | the catalog id | every resource-valued slot, declared or `any` |

A declared `*file.resource` slot needs no wrapper -- the field already states the type -- but its payload
changes from URI to id. An `any` slot needs both: `{"$resource": "res-7"}`.

The substitution cannot live in `ResourceBase.MarshalJSON`, which serializes resources everywhere including
`LedgerEntrySnapshot.URI`, where the URI is stored deliberately and must remain. It belongs at the slot seam,
`immediateData.Value` -- the same place the wrapper lives.

#### The decoder binds a resource; state carries resolution

**Ruled: the decoder binds a Resource.** `{"$resource": "res-7"}` resolves through
`ResourceCatalog.Lookup(id)` and the slot holds the Resource as an **interface value** -- [Resource] is an
interface and `immediateData.Value` is `any`, so what the slot stores is an interface value whose dynamic type
is the concrete resource. There is no separate reference type, and none is needed:

| Catalog state | Means |
| --- | --- |
| `Pending` | unresolved -- claimed, not yet observed or produced |
| `Active` or `Gone` | resolved -- observed, and either present or destroyed |

Resolution is a property of the catalog entry, not of the slot's Go type. A reference type was considered and
is NOT adopted: it would encode in the type system a distinction the state machine already carries, leaving
two sources of truth about whether a resource has been observed.

This is why binding at load is safe despite load never verifying existence (below). Binding yields a
reference to a `Pending` entry, which is exactly what a plan whose producing node has not run should hold. The
`Pending` to `Active`/`Gone` transition happens later, driven by pre-flight, and the slot's reference
follows the entry rather than duplicating its state.

A resource-valued slot always has an id to store. A `Pending` entry exists from plan time onward, so the
question "what if there is no entry yet?" does not arise:

| What the author wrote | At plan time | The slot holds |
| --- | --- | --- |
| `file.path("foo")` | the constructor mints a `Pending` entry | the id |
| `"foo"` into a resource-typed slot | coercion through `Discover` mints one | the id |
| `"foo"` into an `any` slot | nothing; the author never said it was a resource | the string |

The third row is correct rather than a gap. A bare string in an `any` position is a string, and treating it as
a resource would be the reader inferring what the writer never said -- decision 7's rule, in another costume.

#### A cataloged resource names itself

**Ruled 2026-08-30.** `encodeResource` needs no catalog. A resource that has been cataloged already carries
its catalog id, and the write side asks the resource for it:

```go
func encodeResource(resource Resource) (map[string]any, error) {
	id := resource.ID()
	if id == "" {
		return nil, fmt.Errorf("op.encodeTypeWrapper: %s %q is not cataloged", typeNameResource, resource.URI())
	}
	return map[string]any{typeNameResource: id}, nil
}
```

**The catalog is the stamper, not the namer.** [ResourceBase] holds `id` and `producerID`, and its doc
comment says who owns them: "stamped by the [ResourceCatalog] when the resource is cataloged; they are not a
concern of the resource itself" (`resource.go:144-149`). The stamping is literal -- `base.id = id` at
`resource_catalog.go:828` and `:936`. `ID()` returns that field (`resource.go:218`). So identity is settled
once, at catalog time, and every later reader just reads it.

**What was wrong before.** The code asked `resource.RuntimeEnvironment().ResourceCatalog` to `Resolve` the
resource back into an id. Two defects in one line. The environment's catalog is nil by design at that point
-- [plan.Provider.Assemble] nils it at `provider.go:204` once [NewGraph] has taken ownership -- which is what
the three executor tests were reporting. And `Resolve` *mutates*: it catalogs a resource that is not yet
present. A serializer that interns into a catalog is writing where it was asked to read.

The chain that reaches the resource is real -- a node holds slots, a slot holds a resource -- and it ends
there. There is no next hop to reach for.

**Rejected: stamping each node with its graph's catalog.** Considered on 2026-08-29 and wrong. It re-plumbs a
lookup that does not need to exist, adds a field to every node to answer a question the resource already
answers, and leaves the mutating `Resolve` call on the write path. No node changes; no `graph.go` changes.

**What it fixes.** The three executor tests failing on `$resource ... has no catalog to name it`
(`TestRun_AKindMismatchStopsEvenUnderIgnore`, `TestRun_ScopedPreflightFailsOnConsumedMissingClaim`,
`TestRun_ScopedPreflightVerifiesConsumedClaims`), plus
`TestLoadGraph_AResourceInAnAnySlotReloadsAsAResource`. All four are that one defect.

**The read side is not symmetric.** Decoding holds an id and must find the resource it names, so it does need
a catalog -- the graph's own, reconstructed by `unpackCatalog` from the document's `resources` section, not
the ambient environment's. `assembleUnits` / `assembleNode` / `assembleBindings` thread `env.ResourceCatalog`
today. Which catalog they should thread is still open; that the write side never needed one does not settle
it.

#### What load must not do: existence belongs to pre-flight

Ruled previously, in docs/architecture/4-resource-management.md §3 (the claims taxonomy, 2026-08-22), and
binding on this work:

> **Pending** -- the URI is claimed but not yet observed or produced. Plan-time entries are born here.

> `Discover` performs none [no I/O], and existence belongs to pre-flight alone.

The scenario that makes this concrete: a graph has two nodes, the first produces file resource `foo`, and the
second consumes it through an immediate slot. Save, then load. At load `foo` has a ledger entry in `Pending`
and **is not on disk, correctly** -- nothing has executed. A load that verified existence would reject a
perfectly good plan for describing work it has not done yet. So resolution at load resolves *identity* and
never touches the filesystem.

The three cases must stay distinct. Conflating any two of them breaks the scenario above:

| Case | State | Detected at | Answer |
| --- | --- | --- | --- |
| The id is not in the ledger | -- | run time | fail the whole run |
| In the ledger, not yet produced | `Pending` | load | **normal**, not an error |
| In the ledger, destroyed | `Gone` | run time | the consumer's [MissingResourcePolicy] |

`Gone` is a formal state meaning *was there, then destroyed*, and `Discover` errors on it ("resource is
known-gone"). It is NOT "absent from disk". A not-yet-produced resource is `Pending`; if one were ever born
`Gone`, the two-node graph above would fail to load. The `Pending` to `Active`/`Gone` transition is driven by
the executor's pre-flight resolve pass through `VerifyExistence`, and by nothing else.

When a consumer does find a resource missing at run time, [MissingResourcePolicy] governs: `Stop` or `Ignore`,
`Stop` winning aggregation across the consumers of one entry, with a warning emitted under every policy.

### 9. The wrapper is in the canonical form, and that is what makes reload stable

Measured rather than assumed. `Graph.canonicalForm` marshals the live `*Node` values
(`graph.go:830-851`), `Node.MarshalYAML` returns `n.marshalData()` (`node.go:203`), and `marshalData` fills
its slots through [marshalBindings]. Wrapping in [marshalBindings] therefore puts the wrapper into the
canonical form as well as the document.

Three consequences, all wanted:

- **The checksum changes** for every graph holding an immediate slot value. Greenfield, so no document needs
  to keep validating; the Migration section already states this.
- **The canonical form becomes codec-independent.** It now carries explicit types, so a graph written as json
  and the same graph written as yaml canonicalize identically even for values the two codecs disagree about.
  That is what the "JSON and YAML agree" goal has been asking for.
- **It fixes #734 at the root.** A reloaded `{"$float64": "42"}` resolves to `float64(42)`, which re-wraps to
  the identical bytes, so the recomputed checksum matches the stored one. The regression exists precisely
  because a `json.Number` re-canonicalized differently from the `float64` that was written.

The last point is the load-bearing one: the wrapper is not merely tolerated by the checksum, it is what makes
save and reload agree at all.

### Out of scope, and why

`converter.go:446` collapses `*starlark.List`, `starlark.Tuple`, and `*starlark.Set` into a single
`naturalSequence` call, so all three arrive in Go as `[]any`. That loss happens at the Starlark-to-Go
boundary, before serialization. The envelope records what Go holds; it cannot restore a distinction Go never
received. Restoring Starlark-level identity is a defect in a different layer, filed as #730.

## Implementation Phases

### Phase 1: Prove every finding with a failing test -- COMPLETE

Landed in `pkg/op/any_slot_types_test.go`. Every row was run and seen red for the reason it names; the one
characterization test was run and seen green.

- [x] A `float64` in an `any` binding reloads as `float64` -- red on finding 1.
- [x] A `[]byte` in an `any` binding reloads as those bytes -- red on finding 2.
- [x] A `Resource` in an `any` binding reloads as a Resource -- red on finding 3, reaching its assertion:
      the value came back as the URI `string`.
- [x] A graph holding `float("inf")` in an `any` slot saves as JSON -- red on finding 4 with
      `json: unsupported value: +Inf`, an error rather than a wrong value.
- [x] A value in an `any` binding is never a `json.Number` after `LoadGraph` -- red on finding 5.
- [x] An `int64` of 9007199254740993 survives a recovery-stack round trip -- red on finding 6, reloading as
      9007199254740992.
- [x] `IsTruthy(json.Number("0"))` is false -- red today, the narrowest statement of the inversion.
- [x] Confirm by test, not by assertion, that `yaml.Marshal(float64(42))` emits `42`. **Confirmed green**, so
      finding 1 is codec-independent and the write side needs fixing in both.

Two things Phase 1 changed about the plan itself:

- **A precondition test was added.** Four value assertions were initially red on a checksum mismatch rather
  than on the defect they name -- they never reached their assertion, which is the vacuous-test failure mode.
  `TestLoadGraph_AGraphWithAValueInAnAnySlotLoads` now runs first and isolates the cause by kind: a string and
  a bool reload cleanly, an integer and a float do not. That measurement is what finding 5 now records.
- **The recovery-stack assertion was narrowed.** Comparing whole documents made the test red on a differing
  `transaction_id` as well, so it would have passed for the wrong reason once precision was fixed. It now
  compares only the result's literal digits, read with `UseNumber` so the probe does not re-run the very
  float64 conversion it exists to detect.

### Phase 2: Resolve at the two authorities

- [ ] Resolve an enveloped value at the decoder in `graph.go`, so no envelope survives `LoadGraph`. This is
      what makes a graph with a number in an `any` slot loadable again (finding 5).
- [ ] Leave bare-literal resolution in `readAgainstField`, reading against the declared type. It must not move
      to the decoder; that would guess ahead of the field and regress #711.
- [ ] Turn each of the four pass-through routes into an error rather than a guess, including the [Convert]
      failure that is deferred to dispatch today. The load-time message names the slot, the parameter, and the
      declared type.
- [ ] Give `scalarTruthy` a `json.Number` case as defense in depth, so the truthiness inversion cannot recur
      if a new decoder path appears. The resolution above is the fix; this is the guard rail.

### Phase 3: Write the envelope

- [ ] Envelope every value in an `any` slot, in JSON and in YAML, emitting the same shape in both so the two
      documents stay structurally isomorphic.
- [ ] Recurse into `[]any`, `map[string]any`, and arrays, enveloping each element.
- [ ] Leave containers with a declared element type bare inside.
- [ ] Write a resource-valued slot as its catalog id, at both seams at once: wrapped for an `any`
      position, bare for a declared resource type.
- [ ] Resolve it by `ResourceCatalog.Lookup(id)`, never `Discover(uri)`, so a slot binds to the
      generation it was written against. A ledger miss fails the whole run.
- [ ] Resolve identity ONLY, binding the Resource pointer. Load never verifies existence -- a
      `Pending` entry is the normal case for a plan whose producing node has not run.
- [ ] Retire the URI paths for slots: `Discover(uri)` on unmarshal, and `resolveDispatchResource`.
- [ ] Encode a non-finite float in the envelope payload, so JSON can carry what it cannot express as a
      bare number. Apply this at **every** float position, declared or not -- not only in `any` slots.
- [ ] Leave a value with a declared type bare. Enveloping it would duplicate what the field already says.
- [ ] Apply it at every seam in the finding-7 table, not only `bindingData`.

### Phase 4: Refuse an unenveloped value

- [ ] A bare value in an `any` slot is a read error naming the slot and what was found.
- [ ] An envelope naming a type the reader does not know is a read error, not a fallback.

### Phase 5: Document the format and close the loop

- [ ] State the envelope in the document-format documentation.
- [ ] Re-check the checksum and canonical-form question: Phase 3 changes the bytes of every document holding
      an `any` slot. Whether it changes the *canonical* form is the open question below.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 0 | A graph with a number in an `any` slot loads at all | unit | Reverting Phase 2's resolution |
| 1 | A `float64` in an `any` binding reloads as `float64` | unit | Reverting Phase 3's envelope |
| 2 | An integer in an `any` binding reloads as an integer | unit | The reader resolves every envelope to float |
| 3 | A `[]byte` reloads as those bytes, not as a base64 string | unit | `[]byte` left un-enveloped |
| 4 | `LoadGraph` never leaves a `json.Number` in an `any` slot | unit | Reverting Phase 2's resolution |
| 5 | `IsTruthy(json.Number("0"))` is false | unit | Removing the `json.Number` case from `scalarTruthy` |
| 6 | A bare value in an `any` slot is refused | unit | Phase 4 reverted to inference |
| 7 | A map whose own key is the reserved key round-trips intact | unit | The envelope is applied selectively |
| 8 | `plan.Variable(default=1.0)` survives save/reload with its type | e2e | Either side reverted |
| 9 | JSON and YAML agree on the same graph, both directions | unit | Enveloping in one codec only |
| 10 | A receipt's `Result` keeps its type across compensation | scenario | Phase 3 applied to `bindingData` alone |
| 11 | A `map[string]any` round-trips every element type | unit | The envelope stops at the top level |
| 12 | A `map[string]string` keeps bare elements | unit | The envelope reaches declared element types |
| 13 | A `[]any` nested in a `map[string]any` round-trips at depth | unit | Recursion stops at one level |
| 14 | A `Resource` in an `any` slot reloads as a Resource | unit | The Resource wrapper is dropped |
| 15 | `inf`, `-inf`, and `nan` round-trip as JSON | unit | The payload reverts to a bare number |
| 16 | `inf` round-trips identically in JSON and YAML | unit | Only one codec is taught the form |
| 17 | A declared `float64` field holding `+Inf` saves | unit | Non-finite handling scoped to `any` |
| 18 | A declared `fs.FileMode` field is written bare | unit | The envelope leaks onto declared types |
| 19 | `int64` 9007199254740993 survives an `any` slot | unit | The payload reverts to a bare number |
| 20 | A large `int64` survives a recovery-stack resume | scenario | `recovery_stack` left un-enveloped |
| 21 | A bare literal with no declared field is a load error | unit | The route guesses from syntax |
| 22 | No envelope survives `LoadGraph` | unit | Envelope resolution moved off the decoder |

**Write the failing test first.** Every row is red before its phase and must be seen red once.

**Not covered, as a decision rather than an oversight:**

- Tuple-vs-list-vs-set in an `any` slot. All three become `[]any` at `converter.go:446`, before
  serialization. The envelope cannot restore what Go never received. Filed as #730.
- `float32`-vs-`float64`. The converter produces `float64` for every `starlark.Float`, so a `float32` never
  reaches an `any` slot from Starlark.
- Old documents. Greenfield, per the governing principle: documents written before this change need no
  accommodation, and that is a stated decision rather than an assumption.

## Migration Path

None. Per the governing principle there are no legacy documents to accommodate. Documents written before this
change already read back wrong; Phase 4 makes them fail loudly instead of quietly, which is the intent.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/type_wrapper.go` | Create | The wrapper codec (**landed**, green) |
| `pkg/op/convert.go` | Modify | Resolve an enveloped value at the `any` seam |
| `pkg/op/truthiness.go` | Modify | `json.Number` case in `scalarTruthy` as a guard rail |
| `pkg/op/node.go` | Modify | Envelope `bindingData.Value` |
| `pkg/op/variable.go` | Modify | Envelope `Value` |
| `pkg/op/receipt.go` | Modify | Envelope `Result` and `Slots` |
| `pkg/op/recovery_stack.go` | Modify | Envelope `Result` and `Entries` |
| `pkg/op/origin.go` | Modify | Envelope `Annotations` |
| `pkg/op/graph.go` | Modify | Extend the `UseNumber` comment to state the envelope rule |
| `pkg/op/method.go` | Modify | Retire `resolveDispatchResource`'s URI lookup for slots |
| `pkg/op/provider/*/resource*.go` | Modify | Retire `Discover(uri)` on unmarshal for slot values |
| `pkg/op/any_slot_types_test.go` | Create | Phase 1: one failing test per finding (**landed**) |
| `pkg/op/type_wrapper_test.go` | Create | Wrapper round trips, both codecs (**landed**, green) |
| _every float position_ | Modify | Non-finite floats envelope regardless of declaration (Requirement 2) |

## Related Documents

- Issue #712 -- this plan
- Issue #711 / PR #713 -- declared-type numbers; the source of finding 5
- Issue #709 -- the conversion category rule
- docs/architecture/4-resource-management.md §3 -- the claims taxonomy; `Pending`, `Gone`, and the
  rule that existence belongs to pre-flight
- Issue #735 -- a resource-valued slot re-identifies by URI, binding to the current generation
- Issue #734 -- a graph with a number in an `any` slot cannot be loaded; fixed by this plan's Phase 2
- Issue #730 -- tuple, list, and set collapse to `[]any` at the conversion boundary
- Issue #443 -- Epic: Serialization and the single codec

## Open Questions
