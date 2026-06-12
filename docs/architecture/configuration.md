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

Three foundation types, all in `pkg/devconfig`, with **no domain knowledge**:

- **`Config`** — the bucket. A scoped collection of `Section`s, carried on `op.RuntimeEnvironment.Application`.
- **`Section`** — a named unit of configuration (signing, model, registry, vars, …). It embeds a small identity base,
  mirroring the codebase's `*Base` convention (`ResourceBase`, `OriginBase`).
- **`Setting[T]`** — one typed setting inside a section: its resolved value plus the provenance of which layer set it.

```go
type Section struct { name string }            // identity in its scope; set at construction

type Setting[T any] struct {
    Value T          // resolved value; the overlay writes it (see Resolution)
    from  SourceKind // builtin / defaults / app / env / cli — for diagnostics
}
```

`Setting[T]` marshals as its **bare value** — `config.yaml` reads `signing: { backend: ssh }`, never a nested
`{ value: … }`; `from` is never serialized. The loader writes both fields during the per-key overlay: it stamps `from`
with the layer it is currently applying and restamps when a higher layer sets the same key (see "Per-key application"
under Resolution) — provenance costs nothing beyond the field itself.

### Three levels, no deeper

```
Config                       # the bucket
├── Defaults                 # the common scope — the floor every app inherits
│   ├── Signing              # a Section (owned by pkg/signing)
│   ├── Model
│   └── …
├── Lore                     # a per-app scope
├── Star
└── Writ
```

- **Level 1** `Config` — the bucket.
- **Level 2** scope — `Defaults` plus one per app (`Lore`, `Star`, `Writ`).
- **Level 3** named `Section`s, each a flat struct of `Setting`s — **sections do not nest in sections.**

Flatness is deliberate: a consumer should never dig to find a setting. Star's arbitrary-depth dotted paths flatten to
**dotted names** — `lint.copyright` becomes a flat section *named* `"lint.copyright"` (dots in the key, not nesting).
Star's `Nested` type definitions become **structured setting values** (a setting of type `[]Pattern`), not
sub-sections: the flatness rule constrains section topology, not value shapes.

## Distributed registration

> **Decisions (preamble).** (1) Sections are announced at **import time** (Go path) or **extension-discovery time**
> (data path) into a **process-wide schema registry** — the same mechanism providers use, the fourth member of the
> existing `Announce*` family. (2) The announce verbs are **`devconfig.AnnounceSection`** (Go) and
> **`devconfig.AnnounceSectionSpec`** (data), living in `pkg/devconfig` — dependency direction forces this (see
> below). (3) Providers **piggy-back** on their existing generated announcements; non-provider owners call the same
> verb by hand. (4) **`pkg/op` owns the runtime section** (dry-run, conflict policy, backup suffix). (5) There is
> **no generic announcement bus** (`pkg/announcer` deferred) — the unification is the *idiom*, not a bus. (6) There
> are **two collision policies**: Go path **fatal**, data path **error-returning** (see "Star unification and the two
> announcement paths"). (7) A resolved `Config` is a **build-time snapshot** of the registry. (8) The source axis
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
built **once per application process** and **snapshots the registry at build time**. Sections announced after the
build appear only in `Config`s built later (star builds lazily, after discovery, so its extensions are always in).
Schema global, values in the app's `Config`; one mechanism, test isolation intact. (Star already splits these:
`extensionsConfig.specs` is the schema registry; the `ConfigElement` tree is the resolved values.)

### Cardinality — one `Config` per application process

This was a genuine fork, and it is resolved (2026-06-12): **config is per application, and there is one `Config` per
application process — in the context of a running application, a config singleton.** The process-wide *schema*
registry and the per-process *resolved* `Config` are the two halves; **apps lock into configuration, not
configuration sources** — sources are consulted once, at the build, and the app references the resolved result
thereafter. Every `RuntimeEnvironment` the process creates (including nested planning environments and per-run test
environments) references the process's `Config`; constructing an additional `Config` explicitly remains *possible*
(the type permits it — tests may), but the application convention is one.

**Caveat — discovery produces new things.** An extension-aware app (star) ends up with configuration it does not know
about until runtime: discovered extensions announce sections after process start, so the app **builds its singleton
after discovery completes** (star's lazy build), and G2 then guarantees every discovered section is in. The exception
is **extensions built into the app** — compiled in, they announce at `init()` like any Go owner and need no deferral.

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
func init() { devconfig.AnnounceSection(reflect.TypeFor[SigningSection](), NewSigningSection) }

// The framework's own settings (pkg/op) — dry-run, conflict policy, backup suffix:
func init() { devconfig.AnnounceSection(reflect.TypeFor[RuntimeSection](), NewRuntimeSection) }

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

| Path | Used by | Schema source | Type |
|---|---|---|---|
| **Go-typed** | providers, subsystems (e.g. `pkg/signing`) | a Go struct; reflect over its fields | compile-time |
| **Data** | star extensions (`extension.yaml`) | a `ConfigSpec` (field→type-name + defaults) | reflection-generated |

Both land as a named `Section` in the same registry and roll up identically. The Go-typed path is the new work; the
data path already exists in `cmd/star/config` and is preserved.

### The factory and its floor

Each section announces a **constructor that builds it pre-floored** — OpenTelemetry's `CreateDefaultConfig()`. The
builtin floor ("the values you get with no `config.yaml`") is therefore a real, typed, constructed value, not an
untyped defaults map. When the registry instantiates a `Config`, it calls each constructor, then the loader overlays
the resolved values.

## Resolution (the roll-up)

A setting resolves on **two axes only**, by **ordered overlay where the last writer wins** — the precedence already
documented in [`2.1-typed-slots.md`](2.1-typed-slots.md) (*"CLI flags → runtime environment → user config files"*):

- **source:** `user config-file < project config-file < env < cli` — the **project layer is app-elected**: star
  elects it (per-project lint/setup config discovered at the git toplevel is star's core use); lore and writ
  currently do not.
- **scope:** `Defaults < <app>`

plus the **builtin floor** beneath both. The load is a staged overlay, each step overwriting only the keys it sets:

```
1. construct sections with builtin floors            (lowest)
2. overlay  user config.yaml  defaults:
3. carry Defaults into the app scope                 (app inherits the resolved defaults)
4. overlay  user config.yaml  <app>:                 (app shadows Defaults)
5. overlay  project config                           (app-elected — star; work-local shadows user)
6. overlay  env  (DEVLORE_* / <APP>_*)
7. overlay  cli flags                                (highest)
```

An app reads **only its own scope plus `Defaults`** — never another app's. Because override happens at overlay time,
there is no per-setting "is-set" bookkeeping and no compile-time decision about which sections are "app-specific" vs
"shared": a section registers **once**, scope-agnostic, and the user places a value under `defaults:` or `<app>:` as
they wish. *Scope is value placement, not schema.*

### The loader is modular

Resolution is fed by a modular loader — OpenTelemetry's **confmap** pattern (Providers fetch sources, Converters
transform, a Resolver merges), realized in Go by **koanf** (Providers: file/env/flags; Parsers: yaml). This replaces
today's package-global `viper` reads. **Variable expansion** (`${VAR}`, the Make-style supplemental layer, below) is a
Converter pass — one well-defined step, not bespoke plumbing.

### Per-key application: provenance and conversion in one pass

The overlay is **per-layer, per-key application** — the loader walks each layer's key→value map and assigns into the
typed sections itself. It is *not* a whole-struct `yaml.Unmarshal` per layer: under that shape the only per-value code
is `Setting[T].UnmarshalYAML`, which cannot know which layer is currently decoding, and provenance would demand diff
passes or smuggled decode context. With the loader as the active party, both problems vanish in one loop:

```
for each layer in [builtin, defaults:, <app>:, project, env, cli]:    // low → high
    for each (key, value) the layer sets:
        section.setting[key].Value = convert(value)    // declared-type-directed
        section.setting[key].from  = layer             // stamp; later layers restamp
```

- **Provenance** is stamped at each assignment and restamped by higher layers — last-writer-wins *with* provenance,
  no bookkeeping beyond the `from` field.
- **Conversion is driven by the declared type.** File layers keep raw values as `*yaml.Node` and call
  `node.Decode(&setting.Value)` — yaml.v3 then converts to the field's `T` for free (scalars, named string types like
  `Backend`, structured values like `[]Pattern`, anything implementing `encoding.TextUnmarshaler`). The env and cli
  layers carry raw strings, converted by the same declared type (`strconv` for scalars; `yaml.Unmarshal` of the string
  for structured values). On the data path the spec's declared type *names* generate the `reflect.Type` (star's
  existing mechanism), and the same decode applies.
- **One key→field table per section type** (reflect once over the struct, matching yaml tags) maps layer keys to
  `Setting` fields — the same reflection the data path's generated types already require, so the machinery is shared,
  not new.

### Not a configuration axis: writ layers

Writ's `base/team/personal` (and `system/home`) **layers are a packaging concern** — they decide *where writ pulls
packages and files from* ([`2.4-hermeticity-guarantees.md`](2.4-hermeticity-guarantees.md)). They contribute **zero**
configuration. Configuration never reads from those repos and never rolls up across them; the layer-tree overlay is a
separate mechanism and is **off-limits** to the config engine.

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

- **`pkg/devconfig`** — foundation only (`Config`/`Section`/`Setting`); generic over `Section`; imports no domain.
- **Owner packages** define their own sections, importing only `pkg/devconfig`: `SigningSection` → `pkg/signing`,
  `ModelSection`/`RegistrySection` → their subsystems, an execution/runtime section → `pkg/op` (the *only* sections op
  defines — its own).
- **Scope composition** (`Defaults` + per-app scopes) lives in the **app / assembly layer** — not `pkg/devconfig`
  (leaf) and not `pkg/op` (must not import domains).
- **Typed accessor** — each owner exposes a keyed getter so consumers never type-assert by hand:
  `signing.SectionFrom(cfg) (*SigningSection, bool)`.

```
pkg/devconfig                      (leaf: Config / Section / Setting)
   ▲            ▲
pkg/signing    pkg/op              (define their own sections; import devconfig)
   ▼                               ▼
app / assembly  ── compose scopes; apps declare the sections they carry
```

`pkg/op` carries `devconfig.Config` on `Application` and reads it **generically** — it never needs the concrete
`SigningSection`, so it never imports `pkg/signing`. `pkg/signing` imports `pkg/op` (to sign graphs) and
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
- **Lazy resolution** — star builds its config after `DiscoverAndLoad`, which is what makes discovery-time
  announcement safe.
- **A hack retires** — `Application.Overrides["config"]` exists only because star's config cannot ride `Application`
  properly; with `devconfig.Config` on `Application`, the one real `Overrides` user disappears.

### What star demanded of the design (folded in above)

1. **A defined freeze point** — extensions announce at discovery time, after `init()`, so the registry accepts late
   announcements and each resolved `Config` is a build-time snapshot.
2. **Two collision policies** — fatal for compiled-in code, error for user-installed data (below).
3. **The project config source** — star merges a project-level `star/config.yaml` (git-toplevel) over user config;
   the source axis carries that app-elected layer.
4. **Dotted names, flat sections** — `lint.copyright` is a flat section named `"lint.copyright"`; star's `Nested`
   type definitions become structured setting values.

### Guarantees

- **G1 — framework names cannot be hijacked.** Go `init()` announcements strictly precede extension discovery, so
  compiled-in sections (`signing`, the op runtime section, …) always claim their names first; an extension claiming a
  taken name gets an error, never the name.
- **G2 — a `Config` is a build-time snapshot.** Membership is fixed at build; sections announced later appear only in
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

### Sequence — Go path (`pkg/signing`, a lore session)

```
pkg/signing.init()       devconfig registry        lore bootstrap           loader              consumer
       │                         │                       │                    │              (pkg/signing)
       │ AnnounceSection(        │                       │                    │                    │
       │   TypeFor[SigningSection],                      │                    │                    │
       │   NewSigningSection)    │                       │                    │                    │
       │────────────────────────▶│                       │                    │                    │
       │                         │ "signing" free?       │                    │                    │
       │                         │  yes → store factory  │                    │                    │
       │                         │  no  → FATAL, both    │                    │                    │
       │                         │        claimants named│                    │                    │
       │      … all init()s complete; main() begins …    │                    │                    │
       │                         │                       │                    │                    │
       │                         │     Build("lore")     │                    │                    │
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
       │                         │                       │                    │ → *SigningSection  │
```

### Sequence — data path (star extension `lint.copyright`)

```
star main()             discovery              devconfig registry       star Config (lazy)     extension
(init() done: Go sections hold their names)           │                       │             (lint.copyright)
       │                     │                        │                       │                    │
       │ DiscoverAndLoad()   │                        │                       │                    │
       │────────────────────▶│                        │                       │                    │
       │                     │ read extension.yaml:   │                       │                    │
       │                     │ ConfigSchema{path: "lint.copyright",           │                    │
       │                     │              fields, defaults}                 │                    │
       │                     │ AnnounceSectionSpec(spec)                      │                    │
       │                     │───────────────────────▶│                       │                    │
       │                     │                        │ "lint.copyright" free?│                    │
       │                     │                        │  yes → generate type, │                    │
       │                     │                        │        store factory  │                    │
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
       │                     │                        │                       │ accessor           │
       │                     │                        │                       │ ("lint.copyright") │
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
