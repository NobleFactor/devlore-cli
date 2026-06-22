# Configuration

> **Status:** design (draft). The implementation plan and sequencing live in
> [`docs/plans/extract-starlark-from-op/phase-8/configuration.md`](../plans/extract-starlark-from-op/phase-8/configuration.md).

## Thesis

Configuration in devlore is a **distributed-participation problem**, not a static struct. Independent participants —
providers, subsystems, and star extensions — each **own** a slice of the configuration surface. None of them should
have to know about the others, and the framework should not have to know about any of them in particular. So the model
is: **participants announce their configuration schema; a registry assembles the announcements; values roll up through a
fixed precedence.**

That shape is not invented here. It is the same shape Kubernetes uses for API types and OpenTelemetry uses for
collector components, and devlore already uses it for providers (`op.ReceiverRegistry`) and for star extensions
(`cmd/star/config`). **The resemblance to Kubernetes is not accidental** — both are exercises in *modeling distributed
systems*: independent participants that register themselves into a shared, versioned model, with deterministic
resolution rules. This document applies that lens to configuration.

The foundation package is **`pkg/devconfig`** (named `devconfig`, not `config`, because the bare name is already
contended — `internal/config`, `cmd/star/config`, and the AWS SDK's `config` all coexist).

## The model

The foundation lives in `pkg/devconfig`, with **no domain knowledge**:

- **`Config`** — the **interface** for a *container* of named sections; **`ConfigBase`** is the embeddable that
  implements it. A `Config` carries `Path()` but **no name** — it is held as a struct field, named by the field that
  holds it. The resolved root is carried on `op.RuntimeEnvironment.Application`. Because a section's field may itself be
  a `Config`, the model is a **tree** (see [The configuration tree](#the-configuration-tree)).
- **`Section`** — the **interface** every section satisfies (`Name() string`, `Path() string`); **`SectionBase`** is
  the embeddable, named identity, mirroring the codebase's `*Base` convention (`ResourceBase`, `OriginBase`).
- **Go-typed sections** — a plain struct embedding `SectionBase`. Its scalar/typed fields are the **settings**; its
  `Config`-typed fields are the **sub-trees** (e.g. a provider's `Brokers`). Settings and sub-trees are distinct named
  fields, so serialization is ordinary struct marshaling. There is no per-setting wrapper (the `Setting[T]` struct and
  its later accessor function were **withdrawn**); the **section is the fetch unit**.
- **`DataSection`** *(working name)* — the data-path / Starlark section: a kv bag the runtime-discovered star extensions
  take and the form any section crosses into Starlark as. Reduced role — structural containers are `ConfigBase`, not
  `DataSection`.

```go
type Section interface {
    Name() string
    Path() string                              // dotted, YAML-style: "providers.elevator.brokers.ssh"
}
type Config interface {
    Path() string                              // unnamed — the holding field names it
    Lookup(name string) (Section, bool)
    Sections() []Section
}
type SectionBase struct{ /* name, path */ }     // embeddable; implements Section (named)
type ConfigBase  struct{ /* path, members */ }  // embeddable; implements Config (unnamed)

// a leaf section — settings as plain typed fields (pkg/signing):
type SigningConfig struct {
    devconfig.SectionBase
    Backend        Backend
    Key            string
    AllowedSigners string
}

// a branch section — settings plus a Config-typed sub-tree field:
type ElevatorConfig struct {
    devconfig.SectionBase
    Brokers devconfig.ConfigBase               // the broker sub-tree
}
```

> **Landed vs. design.** The shipped `pkg/devconfig` is still the flat predecessor — `Config` is a concrete
> `map[string]Section` with no `Path()` and no `ConfigBase`. The interface-plus-`ConfigBase` tree above is the target.

Three consequences, each deliberate:

- **No conversion at read time.** Every value is instantiated when the configuration is resolved by its declared type's own
  unmarshaler (`UnmarshalYAML` / `encoding.TextUnmarshaler`); by the time a consumer fetches, the value already has
  the declared type. Read-time conversion would reintroduce live-source semantics through the back door.
- **The section is the fetch unit.** A Go consumer fetches the whole section —
  `devconfig.SectionOf[*SigningConfig](cfg)` (type→name resolved through the registry; they were announced
  together), usually wrapped by the owner as `signing.ConfigFrom(cfg)` — and reads fields directly. One assert at
  the section boundary, zero per-setting machinery.
- **Sections are sealed after resolution.** The fetch returns the registered instance — a pointer into the process
  singleton — and mutation after resolution is a bug (sealed by convention, like the graph). Copy-per-fetch was
  rejected: it buys little, and shallow copies lie about maps and slices.

**Where each setting came from** is **not** stored on settings: the `Config` keeps a path-keyed **sidecar** —
`path → []Override`, each `Override` recording the overlay `Step` (source × layer) that set the value and the value
itself — appended during the overlay. Two diagnostic reads (`config explain`), never value access: `SetBy(path)` gives
the winning `Step` (the last writer), `History(path)` the full override chain.

### The configuration tree

> **In design (2026-06-17).** This supersedes the earlier flat "three levels, no deeper" model. The shape below — the
> recursive `Config`/`ConfigBase` + `Section`/`SectionBase` tree, dotted `Path()`, and the `base` / `profiles` /
> `applications` layers — is settled. One point remains open (end of section).

Configuration is a **recursive tree** built from two pairs (see [The model](#the-model)): a **`Section`** (named, via
`SectionBase`) holds settings and `Config`-typed sub-tree fields; a **`Config`** (a container, via `ConfigBase`, unnamed)
holds named sections. The recursion alternates: section → `Config` field → sections → `Config` field → …

It is one structure seen two ways: the **layers** (`base` → `profiles.<stage>` → `applications.<app>`) and the
**containment** within a layer (`provider` → `broker` → `service`).

```
Config  (root)
├── base            ─► providers ─► elevator ─► brokers ─► services
├── profiles        ─► development · integration · staging · release
│                       (each a partial overlay of the base section tree)
└── applications    ─► lore · star · writ
                        (each an overlay of base, + an optional nested profiles)
```

- **`base`** (was `Defaults`) — the foundational layer every profile and app inherits.
- **`profiles`** (was the `environments` overlay axis) — deployment stages, now structural; strictness scales up toward
  production (`release` tightens what `development` relaxes).
- **`applications`** — one layer per app; an app may carry its own `profiles` overlay for a stage-specific value, and
  otherwise stays the simple `base → profile → app` case.

**Addressing.** Both `SectionBase` and `ConfigBase` expose `Path()` — a **dotted, YAML-style path** that mirrors the
YAML key nesting exactly: `providers.elevator.brokers.ssh.services.step-ca.endpoint`. Whether a segment is a struct
field or a `Config` member is **type-level only** — resolved from the type as the loader descends, never written into
the path string (consistent with YAML, where both are just nested keys). The path is **stamped top-down during that
typed descent**, so `Path()` is a stored byproduct of parsing — no parent back-pointers.

**Sections nest, and that nesting is the provider → broker → service hierarchy** — the flat model could not host it
without an untyped aggregate. Settings stay typed struct fields, so depth lives in the *tree*, never in `map[string]any`.

Resolution traverses and overlays the layers, last writer wins:

    base ⊕ profiles.<active> ⊕ applications.<app> ⊕ applications.<app>.profiles.<active>

The active profile is selected from outside the tree (`--profile` / `DEVLORE_PROFILE`), once per process; each layer
contributes only the keys it changes, applied per-key over the resolved tree.

**Resolution mechanics (settled).** Container members (the dynamic keys inside a `Config` — `ssh`, `sudo`) are typed by
a **path-keyed schema** populated at import: a section is announced against its **`parent` handle** and its path is
derived (`parent.Path() + name`); the loader instantiates the right typed member at each `Config`, while a section's
struct fields stay reflection-driven (an unknown member key is a loud error). The layers overlay by **deep merge**
(low→high; a setting is last-writer-wins, a container is a recursive union, a partial layer inherits every key it does
not set), and the merge **collapses** to a single resolved tree for the active profile + app. `base` / `profiles` /
`applications` are **reserved resolver-level keys** — the hard-coded resolution axis — that never enter the schema or
the resolved tree; everything inside a layer is the uniform, schema-typed section tree.

## Distributed registration

> **Decisions (preamble).** (1) Sections are announced at **import time** (Go path) or **extension-discovery time**
> (data path) into a **process-wide schema registry** — the same mechanism providers use, the fourth member of the
> existing `Announce*` family. (2) The announce verbs are **`devconfig.AnnounceSection`** (Go) and
> **`devconfig.AnnounceSectionSpec`** (data), living in `pkg/devconfig` — dependency direction forces this (see
> below). (3) Providers **piggy-back** on their existing generated announcements; non-provider owners call the same
> verb by hand. (4) **`pkg/op` owns the runtime section** (dry-run, conflict policy, backup suffix). (5) There is
> **no generic announcement bus** (`pkg/announcer` deferred) — the unification is the *idiom*, not a bus. (6) There
> are **two collision policies**: Go path **fatal**, data path **error-returning** (see "Star unification and the two
> announcement paths"). (7) A resolved `Config` is a **snapshot of the registry taken at resolution** — the runtime
> construction of the `Config` at process startup, not a compile step (defined under "Cardinality"). (8) The
> source axis
> carries an app-elected **project config** layer (star elects it). (9) There is **one resolved `Config` per
> application process** — a singleton in the context of the running app; every `RuntimeEnvironment` the process
> creates references it.

### Announcement timing, and why the shared registry is safe

A participant **announces** a section the way an op provider announces itself. Announcement is keyed by
`reflect.Type`; configuration is fetched by `Name`. On the Go path a name collision is **fatal at announce time** (the
Go `Must` idiom — fail fast, at startup, never silently); on the data path it is an **error returned to the caller**
(user-supplied extensions must fail with a diagnostic, not a panic — see the collision policies below).

The process-wide registry is safe because it holds only **inert schema** — `name → factory`, written during `init()`
(Go path) and extension discovery (data path), **never values**. This is what `sql.Register`, `gob.Register`, and
Kubernetes' `SchemeBuilder` do globally without harm. The `prometheus` global-registerer trap (import-order panics,
broken test isolation) afflicts globals holding *values/state* — and values never live here: the resolved `Config` is
constructed **once per application process** and **snapshots the registry at resolution**. Sections announced after
resolution appear only in `Config`s resolved later (star resolves lazily, after discovery, so its extensions are in).
Schema global, values in the app's `Config`; one mechanism, test isolation intact. (Star already splits these:
`extensionsConfig.specs` is the schema registry; the `ConfigElement` tree is the resolved values.)

### Cardinality — one `Config` per application process

This was a genuine fork, and it is resolved (2026-06-12): **config is per application, and there is one `Config` per
application process — in the context of a running application, a config singleton.** The process-wide *schema*
registry and the per-process *resolved* `Config` are the two halves, and the `Config` is **owned by the
`Application`** — `Application.Config` (a `*devconfig.Config`), reached by framework code as
`RuntimeEnvironment.Application.Config`.

**Resolution is a runtime event.** Throughout this document, *resolving* the configuration is the one-time
construction of the resolved `Config` at **process startup** — after CLI parsing and, for an extension-aware app,
after extension discovery. Nothing is compiled; the loader reads the sources and rolls values up. Compile time
contributes only which `init()` announcements are linked into the binary; construction, overlay, validation, and
snapshot all happen when the process starts.

Resolution consults exactly five **sources**, each **once**:

1. **builtin** — the announced section constructors (the compiled-in floors), called to produce pre-floored sections;
2. **the user config file** (`~/.config/devlore/config.yaml`) — read once; one source contributing two overlay
   layers, its `defaults:` scope and then its `<app>:` scope;
3. **the project config file** — app-elected (star); read once when elected;
4. **environment variables** (`DEVLORE_*` / `<APP>_*`) — snapshotted once;
5. **CLI flags** — the parsed pflag set, read once.

The builtin floor alone makes every section **present and looked-up from day one.** Because each owner announces its
section at `init()`, `Application.Config` always resolves at least the floor, so a consumer can read settings —
`devconfig.SectionOf[*RuntimeEnvironmentConfig](cfg)` then `.DryRun` — before any file/env/cli source exists. Adding
those sources later enriches the *same* lookup; it never changes how a consumer reaches a value. **Consumers read
settings through `Application.Config`, never from per-call spec fields** (this is why `RuntimeEnvironmentSpec` no
longer carries `ConflictPolicy` / `BackupSuffix`).

**Apps lock into configuration, not configuration sources.** Once resolved, no consumer returns to a source: nobody
re-reads `config.yaml` mid-run, nobody calls `os.Getenv` at a decision point — the resolved `Config` is the only
thing anyone touches. This is a deliberate break from today's viper behavior, where sources stay *live*
(`viper.GetBool` consults merged state at call time, and `AutomaticEnv` re-reads the environment on every `Get`):
under this model, editing `config.yaml` or exporting a variable after resolution changes nothing in the running app.
It is also what keeps the set-by record honest — the sidecar records which one-time consultation won a key, and
that answer cannot drift.

Every `RuntimeEnvironment` the process creates (including nested planning environments and per-run test
environments) references the process's `Config`; constructing an additional `Config` explicitly remains *possible*
(the type permits it — tests may), but the application convention is one.

**Caveat — discovery produces new things.** An extension-aware app (star) ends up with configuration it does not know
about until runtime: discovered extensions announce sections after process start, so the app **resolves its singleton
after discovery completes** (star's lazy resolution), and G2 then guarantees every discovered section is in. The
exception is **extensions compiled into the app** — they announce at `init()` like any Go owner and need no deferral.

### The announcement family

Announcement is already a house idiom, not an invention of this design. `pkg/op/receiver_registry.go` carries three
verbs — `AnnounceProvider`, `AnnounceResource`, `AnnounceType` — each called from a generated `init()` in the
provider's `gen` package and pulled into the process by the inventory import aggregator
(`pkg/op/inventory/inventory.gen.go`); a duplicate announcement is asserted fatal (`"already announced"`). Resources
use the same two-phase announce/init lifecycle ([`4.3-resource-registration.md`](4.3-resource-registration.md)).
**`devconfig.AnnounceSection` is the fourth member of this family** — same keying, same timing, same collision policy,
same generated-`init()` + inventory ride.

### The API

The verb lives in **`pkg/devconfig`, not `pkg/op`** — forced by dependency direction: `pkg/op` imports
`pkg/application` (the `RuntimeEnvironment.Application` field), so application-level owners could never import op to
announce. Everyone can import the leaf:

```go
// pkg/devconfig — the two announce verbs, one per definition path.

// Go path (compiled-in owners). Keyed by sectionType; fetched by the constructed Section's
// Name(). A duplicate is a programmer error — fatal at announce time, both claimants named.
func AnnounceSection(sectionType reflect.Type, construct SectionConstructor)

// Data path (runtime-discovered schema — star extensions). The spec is user-supplied data;
// a duplicate is a user error — returned, never fatal. First writer keeps the name.
func AnnounceSectionSpec(spec SectionSpec) error
```

Use cases, concretely:

```go
// A provider with config — one generated line beside the announcement it already has
// (pkg/op/provider/<name>/gen/provider.gen.go):
func init() {
    op.AnnounceProvider(reflect.TypeFor[provider.Provider](), …)                         // today
    devconfig.AnnounceSection(reflect.TypeFor[provider.Section](), provider.NewSection)  // new
}

// A subsystem (pkg/signing) — hand-written, identical shape:
func init() { devconfig.AnnounceSection(reflect.TypeFor[SigningConfig](), NewSigningConfig) }

// The framework's own settings (pkg/op) — dry-run, conflict policy, backup suffix:
func init() { devconfig.AnnounceSection(reflect.TypeFor[RuntimeEnvironmentConfig](), NewRuntimeEnvironmentConfig) }

// A star extension (data path) — at extension-discovery time, from the extension's declared
// ConfigSchema; error-returning, never fatal (user data):
//     if err := devconfig.AnnounceSectionSpec(ext.ToSectionSpec()); err != nil {
//         // diagnostic naming the extension and the name's holder; extension disabled
//     }
```

Piggy-backing means riding the **same import event and codegen** — not widening `AnnounceProvider`'s signature; op
stays out of the middle, and a provider's single import brings both its receivers and its config schema.

Two deliberate non-moves:

- **`pkg/application` announces nothing.** It is the *carrier* of the resolved `Config`, not an owner of settings; the
  runtime section's consumers (the executor, `DryRun` checks) are op-side, so `pkg/op` owns it.
- **No `pkg/announcer` generic bus.** The unification that matters is the idiom — one verb shape, `reflect.Type`
  keying, name fetching, generated-`init()` + inventory, fatal collision — now shared by four announcement kinds. A
  literal bus would relocate each kind's typed registry without deleting anything. If a future kind needs shared
  two-phase machinery, extracting it then is mechanical.

### Two definition paths, one registry

| Path | Used by | Schema source | Section shape |
|---|---|---|---|
| **Go-typed** | providers, subsystems (e.g. `pkg/signing`) | a Go struct; reflect over its fields | the owner's typed struct |
| **Data** | star extensions (`extension.yaml`) | a `SectionSpec` — tagged `defaults:`, each value's YAML tag declaring its type | the **kv section variant** (typed key/value pairs) |

Both land as a named `Section` in the same registry and roll up **identically** — same overlay, same set-by
sidecar, same sealed singleton; only the storage differs (declared fields vs. typed kv pairs). The data path
**retires star's `reflect.StructOf` type generation**: a spec-built section *is* the kv variant (see "The starlark
travel form" under Star unification). The Go-typed path is the new work; the data path's schema moves to **tagged
defaults** ("The data-path schema" below) — the old `fields:` type-name table folds into `defaults:`, each value's
tag declaring its type.

### The data-path schema — tagged defaults declare types

On the data path an extension does **not** declare a parallel `field → type-name` table. It writes a single
`defaults:` block, and **each default value's YAML tag declares its setting's type** — the schema *is* the floor. This
is Go's `:=` applied to configuration: the value names the type.

```yaml
# extension.yaml — the lint.copyright section
config:
  path: lint.copyright
  defaults:
    enabled: false                 # !!bool  → bool
    license: auto                  # !!str   → string
    holder: !!str                  # declared at the zero value "" (no useful default yet)
    version: !!str 1.0             # explicit tag: keep "1.0" a string, not a float
    exclude:                       # !!seq   → []any            (untyped container)
      - "**/testdata/**"
      - "**/vendor/**"
    patterns:                      # !!map   → map[string]any   (untyped container)
      go: { match: "…", replace: "…" }
```

The loader decodes `defaults:` into a `yaml.Node`, reads each value node's resolved `Tag`, maps it to a Go type, and
instantiates the floor — the same decode the Go path runs, with the type supplied by the tag rather than a struct
field.

**Three declaration forms, mirroring Go.** Most settings take the first — natural YAML, the parser's implicit
resolution supplying the tag. The bare-tag form declares a typed setting whose useful default is its zero value. The
explicit-tag form is the escape hatch, written only where inference would pick wrong (`!!str 1.0`, `!!float 1`):

| Go | YAML | meaning |
|---|---|---|
| `enabled := false` | `enabled: false` | declare + infer type from the value |
| `var holder string` | `holder: !!str` | declare at the zero value, type named |
| `string("1.0")` | `version: !!str 1.0` | explicit tag overrides inference (coercion) |

**The tag → Go type vocabulary** — the YAML 1.2 core schema plus yaml.v3's two widely-supported extensions. `!!` is
YAML's secondary tag handle, expanding to the global type namespace `tag:yaml.org,2002:`, so `!!str` is the standard
YAML string type, not an application tag (a single `!` would be a local, app-defined one):

| YAML tag | Go type | in core 1.2? |
|---|---|---|
| `!!null` | the setting's zero value | yes |
| `!!bool` | `bool` | yes |
| `!!int` | `int` / `int64` | yes |
| `!!float` | `float64` | yes |
| `!!str` | `string` | yes |
| `!!seq` | `[]any` | yes |
| `!!map` | `map[string]any` | yes |
| `!!timestamp` | `time.Time` | no — yaml.v3 ext |
| `!!binary` | `[]byte` | no — yaml.v3 ext |

The YAML 1.1 carry-overs `!!omap` / `!!set` / `!!pairs` / `!!merge` have no clean Go target and stay unsupported.

**Containers are untyped — `!!seq` → `[]any`, `!!map` → `map[string]any`, always.** We do *not* infer a homogeneous
element type from a default's contents. The reason is empirical: list settings are very often **empty by default**
(`exclude: []`), and an empty sequence carries nothing to infer from — standard YAML cannot spell "empty seq *of
string*." Inferring element types from the non-empty cases would then be right sometimes and wrong others (the empty
case, the mixed case), and a schema rule that holds only sometimes is worse than none. So we do what YAML itself does:
containers stay untyped, and a consumer asserts element types at the point of use — a typed key lookup in Go, ordinary
indexing or iteration in starlark. This keeps the `:=` analogy honest: it holds exactly where YAML's own typing holds,
and stops where YAML's stops.

The old `fields:` (`name → type-name`) table is **subsumed** — every setting appears once, in `defaults:`, typed by
its tag. The former `type:` (the generated Go struct name) becomes **informational**: no struct is generated, and the
kv section variant stores the tagged values directly.

### The factory and its floor

Each section announces a **constructor that returns it pre-floored** — OpenTelemetry's `CreateDefaultConfig()`. The
builtin floor ("the values you get with no `config.yaml`") is therefore a real, typed, constructed value, not an
untyped defaults map: `NewRuntimeEnvironmentConfig()` sets `BackupSuffix: ".devlore-backup"` and
`ConflictPolicy: ConflictStop` directly — the floor is a *compiled-in* default set in code, not defaulting logic
scattered at the point of use. When resolution instantiates a `Config`, it calls each constructor, then the loader
overlays the resolved values.

**The floor is not the `defaults:` scope.** "Default" names two different layers; do not conflate them. The **floor**
(`SourceBuiltin`) is the *compiled-in* default — a Go section's constructor, or a data extension's own `extension.yaml`
`defaults:` block — and it sits beneath every source. The **`defaults:` scope** (`SourceDefaults`) is a *user-authored*
block in `config.yaml`, shared across apps, that overlays **on top of** the floor. The floor ships in the binary; the
`defaults:` scope is configuration the user writes. (A data extension's floor is also spelled `defaults:`, but that is
the extension's *schema* in `extension.yaml` — not the user's `config.yaml` scope.)

## Resolution (the roll-up)

A setting resolves by **ordered overlay where the last writer wins**, across two orthogonal dimensions — the **source**
a value comes from and the **layer** of the tree it sits in (the precedence already documented in
[`2.1-typed-slots.md`](2.1-typed-slots.md): *"CLI flags → runtime environment → user config files"*):

- **source:** `user config-file < project config-file < env < cli` — the **project layer is app-elected**: star
  elects it (per-project lint/setup config discovered at the git toplevel is star's core use); lore and writ
  currently do not.
- **layer:** `base < profiles.<active> < applications.<app> < applications.<app>.profiles.<active>` — the tree layers
  from ["The configuration tree"](#the-configuration-tree). `base` is the floor every app and profile inherits; the
  active profile (`--profile` / `DEVLORE_PROFILE`) overlays it; the application overlays that; an application's own
  profile overlay, if present, wins last. It is **application-dominant**: a profile refines `base`, but an application's
  value still overrides a profile value of the same key. No active profile → only `base` and the application layers
  apply. See ["Profiles"](#profiles--deployment-stage-overlays) below.

plus the **builtin floor** (`SourceBuiltin`) beneath all — the compiled-in default, not the `base` layer above. The load is a staged overlay that **walks matching paths down the tree**, each step overwriting only the keys it sets:

```
1. construct sections with builtin floors                  (lowest)
2. overlay  base
3. overlay  profiles.<active>                              (active stage refines base)
4. overlay  applications.<app>                             (running app shadows base + profile)
5. overlay  applications.<app>.profiles.<active>           (app's own stage overlay, if present — wins)
6. overlay  project config  (its base, then its profiles.<active>)   (app-elected — star)
7. overlay  env  (DEVLORE_* / <APP>_*)
8. overlay  cli flags                                      (highest)
```

Because each layer is itself a tree, the overlay is **per-key down matching paths**:
`base.providers.elevator.brokers.ssh.default_ttl` is overwritten by
`profiles.release.providers.elevator.brokers.ssh.default_ttl`, and a key set in one layer but not another is inherited.

An app reads **only `base` plus its own application layer** (and the active profile) — never another app's. Because
override happens at overlay time, there is no per-setting "is-set" bookkeeping and no compile-time decision about which
sections are "app-specific" vs "shared": a section registers **once**, layer-agnostic, and the user places a value under
`base`, a profile, or `applications.<app>` as they wish. *Layer is value placement, not schema.*

### Profiles — deployment-stage overlays

`profiles` is the renamed environment axis, now a structural layer in [the tree](#the-configuration-tree) and named for
the deployment pipeline. Each profile overlays `base`; an application overlays the resolved `base + profile`; and an
application may carry its own `profiles` overlay for a stage-specific value. The resolved `Config` a consumer reads has
the active profile already folded in.

```yaml
base:
  registry_url: "https://registry.local"
  llm_provider: "openai"
  max_retries: 3

profiles:
  development: { registry_url: "https://localhost:8080", debug_mode: true }
  staging:     { registry_url: "https://staging.internal", debug_mode: false }
  release:     { registry_url: "https://production.live", debug_mode: false, max_retries: 5 }

applications:
  lore:
    llm_provider: "anthropic"        # app override, every profile
  star: {}                           # inherits resolved base + profile
```

Resolving for `lore` in `release` (per-key, up the chain):

| key | value | won from |
|-----|-------|----------|
| `registry_url` | `https://production.live` | `profiles.release` |
| `llm_provider` | `anthropic` | `applications.lore` |
| `max_retries` | `5` | `profiles.release` (over `base` 3) |
| `debug_mode` | `false` | `profiles.release` |

This carries the promote-with-zero-edits story: the same signed artifact resolves differently under `--profile
development` and `--profile release` because only the profile layer differs. The axis is general — any section may carry
profile overlays. For the elevator, where the overlay reaches deep into a provider → broker → service sub-tree, see
[the elevator case study](#case-study-the-elevator-section).

### The loader is modular

Resolution is fed by a modular loader — OpenTelemetry's **confmap** pattern (Providers fetch sources, Converters
transform, a Resolver merges), realized in Go by **koanf** (Providers: file/env/flags; Parsers: yaml). This replaces
today's package-global `viper` reads. **Variable expansion** (`${VAR}`, the Make-style supplemental layer, below) is a
Converter pass — one well-defined step, not bespoke plumbing.

### Per-key application: set-by and conversion in one pass

The overlay is **per-layer, per-key application** — the loader walks each layer's key→value map and assigns into the
sections itself. It is *not* a whole-struct `yaml.Unmarshal` per layer: under that shape no per-value code can know
which layer is currently decoding, and the set-by record would demand diff passes or smuggled decode context. With the
loader as the active party, both problems vanish in one loop:

```
for each layer in [builtin, base, profiles.<active>, applications.<app>, applications.<app>.profiles.<active>, project, env, cli]:  // low → high
    for each (path, value) the layer sets:                // path walks the section tree
        decode value into the section's field at path     // the declared type's own unmarshaler
        sidecar[path] = append(sidecar[path], {step, value})  // set-by: append the override; last entry wins
```

- **The set-by chain** appends an `{step, value}` entry at each assignment; the last entry is the winner — `SetBy(path)`
  reads it, `History(path)` reads the whole chain. No bookkeeping on the sections themselves.
- **Instantiation is the declared type's own unmarshaler.** File layers keep raw values as `*yaml.Node` and call
  `node.Decode(&field)` — invoking the field type's `UnmarshalYAML` / `encoding.TextUnmarshaler` (scalars, named
  string types like `Backend`, structured values like `[]Pattern`). The env and cli layers carry raw strings,
  instantiated through the same declared type (`strconv` for scalars; `yaml.Unmarshal` of the string for structured
  values). On the data path the spec's declared type *names* select the kv variant's value types, and the same decode
  applies. **There is no read-time conversion anywhere.**
- **One key→field table per Go section type** (reflect once over the struct, matching yaml tags) maps layer keys to
  fields; the kv variant needs none — its keys *are* its storage. Unknown keys in a layer (a typo'd setting name) are
  detected here and reported.

### Not a configuration axis: writ layers

Writ's `base/team/personal` (and `system/home`) **layers are a packaging concern** — they decide *where writ pulls
packages and files from* ([`2.4-hermeticity-guarantees.md`](2.4-hermeticity-guarantees.md)). They contribute **zero**
configuration. Configuration never reads from those repos and never rolls up across them; the layer-tree overlay is a
separate mechanism and is **off-limits** to the config engine.

## Case study: the elevator section

The elevator (privilege-elevation) provider is the worked example, because it exercises every part of the model at
once: a provider whose configuration is a sub-tree of **brokers**, each fronting a set of backing **services**, with
per-stage variance supplied by profiles. The full elevation design is in
[`6.1-privilege-elevation.md`](6.1-privilege-elevation.md); here we show only its configuration shape.

### The sub-tree

Under `base` sits a `providers` container, and under it the `elevator` section. Its child `Config` holds two brokers —
`ssh` (mints short-lived SSH certificates; the identity-assumption strategy) and `sudo` (acquires a privileged host
context; the process-spawn strategy). Each broker's child `Config` holds the services it fronts.

```
base
└── providers
    └── elevator          ElevatorSection
        └── brokers
            ├── ssh        SshBroker  ─► services: step-ca, vault-ssh
            └── sudo       SudoBroker ─► services: local
```

### The typed sections

The provider's `elevator.Config` embeds `SectionBase` and **wires** the brokers it uses; each broker owns its config
type in its own package, announced *with* the broker (see
[Pluggable brokers](3.2-projected-provider-api.md#pluggable-brokers--the-provider-is-the-invoker)):

```go
// pkg/op/provider/elevator — wires the brokers it uses; defines none of their config:
type Config struct {
    devconfig.SectionBase
    Brokers devconfig.ConfigBase
}
func NewConfig() devconfig.Section {
    c := &Config{SectionBase: devconfig.NewSectionBase("elevator")}
    c.Brokers = op.WireBrokers(reflect.TypeFor[*ssh.Broker](), reflect.TypeFor[*sudo.Broker]())
    return c
}

// pkg/op/broker/ssh — the broker owns its config schema (and its Services sub-tree):
type Config struct {
    devconfig.SectionBase
    DefaultTTL time.Duration        `yaml:"default_ttl"`
    Failover   []string             `yaml:"failover"`
    Services   devconfig.ConfigBase `yaml:"services"`
}

// pkg/op/broker/ssh — a service config is a leaf the broker owns:
type StepCAConfig struct {
    devconfig.SectionBase
    Endpoint, Provisioner, CAFingerprint string
}
```

### Across the layers

`base` carries the floor; `profiles` tighten it per stage; an application overrides only where it must:

```yaml
base:
  providers:
    elevator:
      brokers:
        ssh:
          default_ttl: 15m
          failover: [step-ca, vault-ssh]
          step-ca: { endpoint: "https://ca.local", provisioner: "devlore" }
        sudo:
          non_interactive: true

profiles:
  release:
    providers:
      elevator:
        brokers:
          ssh: { default_ttl: 5m }          # production tightens cert lifetime

applications:
  star: {}                                  # inherits resolved base + profile
```

### Resolving `star` in `release`

| path | value | won from |
|------|-------|----------|
| `…elevator.brokers.ssh.default_ttl` | `5m` | `profiles.release` (over `base` 15m) |
| `…elevator.brokers.ssh.failover` | `[step-ca, vault-ssh]` | `base` |
| `…elevator.brokers.ssh.step-ca.endpoint` | `https://ca.local` | `base` |
| `…elevator.brokers.sudo.non_interactive` | `true` | `base` |

The overlay walked the **same path** through three trees (`base`, `profiles.release`, `applications.star`), and the
profile contributed a single key deep in the sub-tree — `default_ttl` — leaving the rest of the broker inherited. This
is the "dynamic aggregate section" problem dissolving: brokers and services are typed sections in the tree, not entries
in an untyped per-provider map.

## Validation

Each section may implement `Validate() error` — OpenTelemetry's `component.ConfigValidator`. Validation runs **after**
the roll-up, so it sees resolved values, and fails fast with a precise message (`signing.backend: unknown "kms2"`)
rather than surfacing deep inside an execution.

## Variables — supplemental

Variables are the **Make-style** supplemental layer: `FOO = bar`, overridable from the command line and environment,
referenced as `$FOO` throughout the runtime environment. They are a **`Vars` `Section`** the user authors (today
`WritConfig.Vars`), resolved by the same roll-up and expanded by the loader's Converter pass. Variables are *not* a
parallel system — the variable resolver becomes a thin reader over the one rolled-up config.

## Where sections live

> **A `devconfig.Section` lives with the subsystem that defines its schema and consumes it — never centralized.**

- **`pkg/devconfig`** — foundation only (`Config`, `Section`, `SectionBase`, `DataSection`); generic over `Section`;
  imports no domain.
- **Owner packages** define their own sections, importing only `pkg/devconfig`: `SigningConfig` → `pkg/signing`,
  `ModelConfig`/`RegistryConfig` → their subsystems, an execution/runtime section → `pkg/op` (the *only* sections op
  defines — its own).
- **Scope composition** (`Defaults` + per-app scopes) lives in the **app / assembly layer** — not `pkg/devconfig`
  (leaf) and not `pkg/op` (must not import domains).
- **Typed accessor** — the generic fetch is `devconfig.SectionOf[T](cfg)` (type→name via the registry); each owner
  wraps it so consumers never type-assert by hand: `signing.ConfigFrom(cfg) (*SigningConfig, bool)`.

```
pkg/devconfig                      (leaf: Config / Section / SectionBase / DataSection)
   ▲            ▲
pkg/signing    pkg/op              (define their own sections; import devconfig)
   ▼                               ▼
app / assembly  ── compose scopes; apps declare the sections they carry
```

`pkg/op` carries `devconfig.Config` on `Application` and reads it **generically** — it never needs the concrete
`SigningConfig`, so it never imports `pkg/signing`. `pkg/signing` imports `pkg/op` (to sign graphs) and
`pkg/devconfig`; no cycle.

## Star unification and the two announcement paths

Star is not an integration risk bolted on later — it is the **second of the two announcement paths**, and its
requirements shaped the design above. This section records what star already practices, what it demanded, the
guarantees that fall out, and both paths worked end to end.

### What star already practices (migration tailwinds)

- **Schema/values split** — `extensionsConfig.specs` vs. the `ConfigElement` tree is exactly the registry/resolved
  split this design formalizes.
- **Reference, not owned** — extensions hold `config *config.Config // not owned`; under devconfig they hold the
  resolved `Config` the same way.
- **Lazy resolution** — star resolves its config after `DiscoverAndLoad`, which is what makes discovery-time
  announcement safe.
- **A hack retires** — `Application.Overrides["config"]` exists only because star's config cannot ride `Application`
  properly; with `devconfig.Config` on `Application`, the one real `Overrides` user disappears.

### What star demanded of the design (folded in above)

1. **A defined freeze point** — extensions announce at discovery time, after `init()`, so the registry accepts late
   announcements and each resolved `Config` is a snapshot taken at resolution.
2. **Two collision policies** — fatal for compiled-in code, error for user-installed data (below).
3. **The project config source** — star merges a project-level `star/config.yaml` (git-toplevel) over user config;
   the source axis carries that app-elected layer.
4. **Dotted names, flat sections** — `lint.copyright` is a flat section named `"lint.copyright"`; star's `Nested`
   type definitions become structured setting values.
5. **A starlark travel form** — an object that travels well between Go and starlark, carrying a section's settings as
   key/value pairs (below).

### Guarantees

- **G1 — framework names cannot be hijacked.** Go `init()` announcements strictly precede extension discovery, so
  compiled-in sections (`signing`, the op runtime section, …) always claim their names first; an extension claiming a
  taken name gets an error, never the name.
- **G2 — a `Config` is a snapshot taken at resolution.** Membership is fixed at resolution; sections announced later appear only in
  `Config`s built later. Star resolves lazily, after discovery, so its extensions are always in.
- **G3 — collisions never corrupt.** First writer keeps the name. Go-path duplicate: the process dies at startup with
  both claimants named. Data-path duplicate: the extension is reported and disabled; the process continues.

### The two collision policies, by use case

**Go path — fatal.** Example: `pkg/signing` announces `"signing"`. A duplicate means two *compiled-in* packages claim
the same section — a bug only a code change can fix, best surfaced at the earliest moment, in every environment,
deterministically. Crash at startup, naming both claimants.

**Data path — error.** Example: a user installs two extensions that both claim `"lint.copyright"` — or one that
claims `"signing"`, which the framework already holds (G1). The claimant is user *data*, not devlore code: star
reports a diagnostic naming the extension and the name's holder, disables the extension, and continues. The user
fixes it by uninstalling or renaming. A process must never panic over installable content.

### The starlark travel form — a sealed Mapping, not HasAttrs

Star scripts are configuration consumers without Go types in scope, so sections cross the boundary as the **kv
section variant**: a `devconfig` section whose settings are **typed key/value pairs**. One type, two roles:

- **It *is* the data-path section.** A spec-built section stores `setting name → typed value` directly — no
  `reflect.StructOf` type generation. The spec's `Defaults` apply as the builtin layer of the same overlay.
- **It is the travel form of any Go-typed section.** When a `SigningConfig` is handed to a script, it projects
  through the same interface — resolved lazily against the section's key→field table (the adapter gist,
  `pkg/op/provider/plan/adapter.go`): one source of truth, no copied snapshot to drift.

**Sealed interface choice (2026-06-12): `starlark.Value` + `starlark.Mapping` + `starlark.IterableMapping` —
`starlark.HasAttrs` is deliberately dropped.**

```go
_ starlark.Value           // String / Type / Freeze (no-op — sealed) / Truth / Hash
_ starlark.Mapping         // section["enabled"]; unknown key → loud error (a schema typo)
_ starlark.IterableMapping // Items() — scripts can enumerate settings
```

The reasoning: `HasAttrs` carried sugar, not load. Dot-chaining (`section.enabled`) was a second access idiom; and
the genuinely method-shaped thing scripts do today — `config.get(path, default)` — exists only because star's config
can be *missing keys*, forcing read-time defaults. Under devconfig, **floors make that obsolete**: every declared
setting is present in a built `Config` by construction, so a missing key is a **schema typo, and erroring loudly is
correct**. With read-time defaults gone, one access idiom suffices — indexing — and the root `Config` speaks the same
`Mapping` (dotted section names already forced index syntax there: `config["lint.copyright"]`).

**Branch sections index into their children.** Once sections nest ([the tree](#the-configuration-tree)), indexing a
branch section returns its child section — itself a sealed `Mapping` — so navigation is the same one idiom all the way
down: `config["base"]["providers"]["elevator"]["brokers"]["ssh"]["default_ttl"]`. Indexing a leaf returns a setting
value. The flatness rationale that motivated dropping `HasAttrs` is unaffected — there is still exactly one access
idiom; the tree simply makes it recurse.

Value projection splits by layer. `devconfig`'s own projection is **small and closed** — scalars, lists, and string
maps — and stays in the leaf (it never imports the bridge). A **struct-valued setting** is a Go value, so it crosses as
a `starlarkbridge.goReceiver` built by the **reflection framework** (`marshalReflect`, fed by the receiver registry that
`op.AnnounceType` populates) — its real fields and methods, not a flattened dict. That projection lives at the bridge
layer, where config reaches Starlark anyway, and needs no new machinery: a *section* is a sealed `Mapping`; a Go *value*
reached through it is a receiver, exactly as Go values cross elsewhere.

### Script migration — `.get` to indexing

Today's starlark-facing config is `ConfigValue`, a `starlark.HasAttrs` (`cmd/star/config/starlark.go`), and extension
scripts read through `get`-style calls. Both change at star unification:

```python
# before — method access, read-time default
cfg = config.get
path = ctx.config.get("test.tool_path", "build/devlore-test")

# after — index access; the floor guarantees presence, an unknown key errors loudly
section = config["test"]
path = section["tool_path"]
```

- Call sites like `cmd/star/extensions/com.noblefactor.star.LintCopyright/commands/lint-copyright.star`
  (`config.get`) and `star/extensions/com.noblefactor.devlore.Test/commands/run.star` (`ctx.config.get(path,
  default)`) migrate to indexing. **Read-time defaults are dropped** — the value a script would have defaulted to
  belongs in the extension's declared `Defaults` (the builtin floor), where every consumer sees it and
  `config explain` can attribute it.
- `ConfigValue` (HasAttrs) and `generateConfigType` (`reflect.StructOf`) **retire**; the kv variant replaces both.
- Extensions receive their section **pre-scoped** (star's existing `ResolveConfig` delivery); root navigation is
  index-style.
- If transition sugar proves wanted, a `get` builtin can be added back later — an add-back decision, deliberately not
  planned.

### Sequence — Go path (`pkg/signing`, a lore session)

```
pkg/signing.init()       devconfig registry        lore bootstrap           loader              consumer
       │                         │                       │                    │              (pkg/signing)
       │ AnnounceSection(        │                       │                    │                    │
       │   TypeFor[SigningConfig],                      │                    │                    │
       │   NewSigningConfig)    │                       │                    │                    │
       │────────────────────────▶│                       │                    │                    │
       │                         │ "signing" free?       │                    │                    │
       │                         │  yes → store factory  │                    │                    │
       │                         │  no  → FATAL, both    │                    │                    │
       │                         │        claimants named│                    │                    │
       │      … all init()s complete; main() begins …    │                    │                    │
       │                         │                       │                    │                    │
       │                         │    Resolve("lore")    │                    │                    │
       │                         │◀──────────────────────│                    │                    │
       │                         │ snapshot factories    │                    │                    │
       │                         │──────────────────────▶│                    │                    │
       │                         │                       │ construct floors   │                    │
       │                         │                       │───────────────────▶│                    │
       │                         │                       │ overlay: user defaults: → lore: →       │
       │                         │                       │          env → cli │                    │
       │                         │                       │◀───────────────────│                    │
       │                         │                       │ Validate() each section                 │
       │                         │                       │ → Application.Config                    │
       │                         │                       │────────────────────────────────────────▶│
       │                         │                       │                    │ SectionFrom(cfg)   │
       │                         │                       │                    │ → *SigningConfig  │
```

### Sequence — data path (star extension `lint.copyright`)

```
star main()             discovery              devconfig registry       star Config (lazy)     extension
(init() done: Go sections hold their names)           │                       │             (lint.copyright)
       │                     │                        │                       │                    │
       │ DiscoverAndLoad()   │                        │                       │                    │
       │────────────────────▶│                        │                       │                    │
       │                     │ read extension.yaml:   │                       │                    │
       │                     │ SectionSpec{path: "lint.copyright",            │                    │
       │                     │              tagged defaults}                  │                    │
       │                     │ AnnounceSectionSpec(spec)                      │                    │
       │                     │───────────────────────▶│                       │                    │
       │                     │                        │ "lint.copyright" free?│                    │
       │                     │                        │  yes → store kv-      │                    │
       │                     │                        │        variant factory│                    │
       │                     │                        │  no  → error returned:│                    │
       │                     │◀───────────────────────│        star prints diagnostic, disables    │
       │                     │                        │        the extension; process continues    │
       │ first Config() use  │                        │                       │                    │
       │────────────────────────────────────────────────────────────────────▶│                    │
       │                     │                        │ snapshot factories    │                    │
       │                     │                        │──────────────────────▶│                    │
       │                     │                        │                       │ overlay: user star │
       │                     │                        │                       │ config → PROJECT   │
       │                     │                        │                       │ star config → env  │
       │                     │                        │                       │ → cli; Validate()  │
       │                     │                        │                       │───────────────────▶│
       │                     │                        │                       │ config             │
       │                     │                        │                       │ ["lint.copyright"] │
```

## Prior art

This design is an explicit synthesis. The specifics, because they earned their place:

### Star extensions (`cmd/star/config`) — the in-house base

The closest model is devlore's own. A star `Extension` declares `Config *ConfigSchema` (`cmd/star/star/extension.go`)
— a data schema of field names, type names, and defaults — registered at a dotted path via `RegisterExtension`
(`cmd/star/config/root.go`), which reflection-generates the typed instance. Star already gives us: announce-by-name,
defaults-in-schema, an on-demand accessor, the **schema-specs vs. resolved-values split**, and the
**reference-not-owned instance**. `pkg/devconfig` *generalizes* star to cover Go-typed participants too; star's config
is **unified onto** `devconfig`, not paralleled by it.

### OpenTelemetry Collector — the close companion

We are OTel-shaped almost point-for-point, and OTel is the system to read alongside this one:

- **Factory + `CreateDefaultConfig()`** → our section-announces-a-constructor-with-its-floor.
- **`component.ConfigValidator.Validate()`** → our per-section validation.
- **`confmap`** (Providers + Converters + Resolver) → our modular loader (koanf), including `${…}` expansion.
- **component-by-id** → our section-by-name; instance-based; no version codec.

### Kubernetes — shared distributed-systems DNA

Kubernetes contributes the *registration skeleton*: types register into a `runtime.Scheme` mapping **reflect.Type ↔
name** (GroupVersionKind) via `SchemeBuilder.Register` in `init()` / `AddToScheme`, and the Scheme is an **injectable
instance**. We borrow the skeleton and the instance discipline. We deliberately do **not** borrow its defining
machinery — **multi-version API conversion and codecs** — which is enormous and unnecessary for CLI configuration. The
one Kubernetes idea held in reserve is **schema versioning + forward migration**: a future hook so an old `config.yaml`
upgrades cleanly when a section's shape changes. The deeper kinship is intentional: Kubernetes and devlore are both
modeling distributed systems, and convergent design follows from convergent problems.

### Go registration idioms

`sql.Register`, `gob.Register`, `prometheus.MustRegister`: register-by-name, **panic on duplicate**. We adopt the
panic. The `prometheus` *global* default registerer is the cautionary tale that justifies our **instance, not global**
decision — its global causes import-order panics and breaks test isolation.

### koanf

An instance-based, low-dependency Viper alternative with modular Providers (file/env/flags) and Parsers (yaml). It is
the Go realization of confmap and the loader that retires today's `viper` globals.

## Refinements adopted

From the comparison, the concrete refinements layered onto star's proven registration spine:

1. **Typed default constructor** (OTel) — a real typed floor per section, not an untyped defaults map (Go path).
2. **Per-section `Validate()`** (OTel) — validate resolved values, fail fast.
3. **Modular confmap-style loader** (OTel / koanf) — Providers + Converters, separate from the schema registry.
4. **`${…}` expansion as a Converter** (OTel) — the clean home for Make-style variable interpolation.
5. **Schema global, values per-session** (Kubernetes / star / Go `Register` idioms) — the import-time schema registry
   holds only inert factories; resolved `Config`s are per-`Application`. The anti-`prometheus` line is drawn at
   *values*, not at the registry.
6. **Schema versioning + migration** (Kubernetes) — a *future* hook, flagged, not built.

## Relationship to today's code

- **`op.ReceiverRegistry`** — the in-house precedent for distributed, reflect.Type-keyed, import-time registration;
  `devconfig`'s registry mirrors it (and a provider may announce its config section as part of its announcement).
- **`internal/config`** — the established typed model (`Config`, `LoreConfig`, `WritConfig`, `ModelConfig`,
  `RegistryConfig`) being moved to `pkg/devconfig` and reshaped into the registry.
- **`cmd/star/config`** — the extension-config system being **unified onto** `devconfig` rather than bolted alongside
  it; "provider as extension" makes the config-participant abstraction the same for both.

## Open questions

- **Scope-composition home** — one shared assembly package vs. each app composing its own scope structs.
- **Builtin as runtime floor** — today the embedded `schema.*DefaultConfig` only *seeds files at install*; the target
  constructs the floor at runtime, demoting `config.yaml` to overrides.
- **Schema versioning** — when (not whether) to add the Kubernetes-style migration hook.
- **Star unification sequencing** — the shape is defined ("Star unification and the two announcement paths"); *when*
  to execute the fold remains open.
