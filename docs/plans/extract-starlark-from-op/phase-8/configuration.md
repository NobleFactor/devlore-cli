---
title: configuration — implementation plan (pkg/devconfig)
status: draft
created: 2026-06-11
updated: 2026-06-13
---

# Configuration — implementation plan

**Design of record: [`docs/architecture/configuration.md`](../../../architecture/configuration.md).** The design
evolved past this plan's first draft — compile-time scope *embedding* was superseded by **distributed registration**
(sections announce into a process-wide schema registry; resolved `Config`s snapshot at resolution) — and the architecture
doc carries the full model: foundation types, the two announcement paths and collision policies, the per-key overlay
with loader-stamped provenance and declared-type conversion, owner placement, the star unification shape, guarantees,
sequence diagrams, and prior art. This document carries **sequencing and work items only**.

## Iteration loop (user-directed, 2026-06-12)

1. **Baseline** — add `pkg/devconfig`.
2. **Schema** — define config sections for the first owners.
3. **Operations** — importing a package registers its sections for the running app.
4. **Test, debug, refine** the design — return to 2.

## Queued work

1. **Move `internal/config` → `pkg/devconfig`.** It is infrastructure, consumed across the ecosystem. Named
   `devconfig`, not `config` — the bare name is contended (`internal/config`, `cmd/star/config` — star already
   aliases its import to `cfg` — and the AWS SDK's `config` arriving with signing/KMS). The struct is finessed as we
   go.
2. **Foundation types + announcement.** `Config` (keyed by section name) / `Section` / **plain typed settings** (the
   `Setting[T]` wrapper and its accessor-function successor were withdrawn — the **section is the fetch unit**:
   `devconfig.SectionOf[T]` + owner wrappers); the **`DataSection`** (the *kv section variant* — typed key/value pairs; `starlark.Value` +
   `Mapping` + `IterableMapping` — `HasAttrs` dropped) as the data-path section and starlark travel form;
   `AnnounceSection` (Go path, fatal on collision) and `AnnounceSectionSpec` (data path, error-returning); the
   data-path schema is tagged `defaults:` (each value's YAML tag declares its setting's type, Go `:=`-style; untyped
   containers); the process-wide schema registry; one resolved `Config` per application process, resolved at startup
   (a runtime event, not a compile step), snapshotting the registry; sections **sealed after resolution**.
   **Status — foundation types landed in `pkg/devconfig/config.go` (+ tests):** `Section` (interface) + `SectionBase`,
   `DataSection` (with its `starlark.Value` / `Mapping` / `IterableMapping` faces), `Config` (+ `Section` /
   `SectionOf` / `Provenance`), `SectionSpec`, `SectionConstructor`, `SettingSourceKind`. The announcement verbs +
   registry also landed (`pkg/devconfig/registry.go`): `AnnounceSection` (fatal) / `AnnounceSectionSpec` (error) plus
   the loader read API `AnnouncedSectionNames` / `ConstructorFor` / `SpecFor`; first owner, `op.RuntimeEnvironmentConfig`
   (read live via `Application.Config` — builtin floor now, resolved sources later). **Remaining:** the loader (item 3).
   **Design settled (see [`configuration.md`](../../../architecture/configuration.md)):** the model is a **recursive
   tree** — `Config`/`ConfigBase` (container) + `Section`/`SectionBase` (named), dotted `Path()`, a path-keyed
   child-type schema, deep-merge overlay collapsing to one resolved tree, and `base`/`profiles`/`applications` as the
   reserved resolution axis. The landed flat `Config` (a map of sections) is the predecessor.
3. **The loader.** koanf-backed providers (user `config.yaml`, app-elected project config, env, cli); the staged
   per-key overlay (three axes — source / scope / environment); provenance in the per-section sidecar
   (`devconfig.Provenance`); values instantiated by their
   declared types' own unmarshalers — no read-time conversion; `${VAR}` expansion as a Converter pass.
4. **Owner-located sections** (first wave): `pkg/op` — the runtime section (dry-run, conflict policy, backup suffix)
   **landed as `RuntimeEnvironmentConfig` (`pkg/op/runtime_environment.go`), announced at init() and read live via
   `Application.Config`; its floor sets `BackupSuffix: ".devlore-backup"` / `ConflictPolicy: ConflictStop`, and
   `RuntimeEnvironmentSpec` no longer carries those two fields**; `pkg/application` announces nothing — it carries the
   resolved `Config`; `pkg/signing` — `SigningConfig`
   (see [`signing-options.md`](signing-options.md)); the registry section — owner to be extracted from `internal/`
   (working name `pkg/devregistry`); the model/LLM section likewise; and the **elevator** provider's config section
   — a **provider section with a broker sub-tree** (`providers.elevator` → `brokers` → a section per broker, each
   fronting its services), realized through the recursive `Config` / `ConfigSection` tree and the
   `base` / `profiles` / `applications` layers — **not** a flat `offers` + `brokers` section, and **not** an in-section
   `environments:` map. Brokers **self-announce globally** (like resources — `op.AnnounceBroker` ≈ `op.AnnounceResource`,
   each owning its contract and config schema on a common `op.BrokerBase`); the elevator provider **wires** the brokers
   it uses (`op.WireBrokers`) and, as the **invoker**, allocates a **fresh instance per wired broker** at construction
   via `op.BrokerRegistry[elevation.Broker]` (full model —
   [Projected Provider API → Pluggable brokers](../../../architecture/3.2-projected-provider-api.md#pluggable-brokers--the-provider-is-the-invoker)).
   See the worked shape in
   [configuration.md → the elevator case study](../../../architecture/configuration.md#case-study-the-elevator-section)
   and the full elevation design in [`6.1-privilege-elevation.md`](../../../architecture/6.1-privilege-elevation.md)).
5. **`Application` carries `devconfig.Config`.** The variable resolver becomes a thin reader over the rolled-up
   config (`Vars` as the supplemental Make-style section); retire the op-side flat source maps
   (`pkg/application/application.go:47`) and the package-global `viper` reads.
6. **Star unification.** Shape defined (architecture doc: two paths, G1–G3, project source layer, dotted-name
   flattening, the travel form, and the script migration `.get` → indexing); **timing open** — its own work item,
   not part of the first iterations. **Travel form settled:** a lazy reflection adapter projects any section as the
   sealed `Mapping` (uniform across the root `Config` / typed sections / `ConfigBase` / `DataSection`); a struct-valued
   setting crosses as a `goReceiver` through the existing reflection framework.

## Next — step 1: `Application.Config` + minimal builtin resolution (proposed 2026-06-13; not yet approved)

**Sequencing correction:** the `Application.Config` field (the core of item 5) **gates the loader (item 3)** — the
loader's output is a `*devconfig.Config` that has to land on `Application.Config`, so item 5's field-change precedes
item 3 despite the numbering. The proposed minimal first step, ahead of the full source overlay:

1. **`devconfig.Resolve() *Config`** — snapshot the registry's announced **Go-path floors** into a `*Config` (call each
   `ConstructorFor`; all-`SourceBuiltin` provenance). Data-path floor construction (`SpecFor` → `DataSection`) is a
   flagged TODO until star sections exist.
2. **`Application.Config *devconfig.Config`** — rename the existing `Config map[string]any` (the variable-resolver
   source) to `ConfigValues`, updating its readers (`pkg/op/variable_resolver.go:169`,
   `pkg/op/provider/plan/provider.go:425`, the devlore-test setters); add `Config *devconfig.Config`, populated by
   `NewApplication` via `devconfig.Resolve()`. The one-`Config`-per-process singleton is deferred to the loader.
3. **`NewRuntimeEnvironment` sources the runtime settings from `Application.Config`** — populate `re.BackupSuffix` from
   `devconfig.SectionOf[*RuntimeEnvironmentConfig](app.Config)` rather than the floor constructor directly.

**Consumer-migration facts (established 2026-06-13):**

- **`BackupSuffix`** — the one reader is `file.Provider.Backup` (`pkg/op/provider/file/provider.go:93`), in another
  session's hands; coordinate the switch to `Application.Config` there.
- **`ConflictPolicy`** — `RuntimeEnvironment.ConflictPolicy` (`pkg/op/runtime_environment.go:60`) has **zero readers**
  across `pkg/op` and `cmd/`; it is plumbed-but-unconsumed, so **delete it** rather than migrate.
- **`DryRun`** — migration **waits for the loader**: it needs the CLI-flag overlay (the builtin floor alone cannot
  reflect `--dry-run`), so it stays on `Application.DryRun()` / `Flags` until item 3. (Consumers today:
  `pkg/op/action_types.go:59,116,172`, `pkg/op/runtime_environment.go:599`.)

**Open in step 1:** the `ConfigValues` name; whether the `RuntimeEnvironment.ConflictPolicy` deletion belongs in this
step or its own cleanup.

## The model today (facts that stay true)

- `internal/config/config.go:33` — the established typed model (`Config`, `LoreConfig`, `WritConfig`, `ModelConfig`,
  `RegistryConfig`); `Load()` at `internal/config/config.go:65`; precedence already cli > env > file at
  `internal/config/config.go:56`.
- `internal/config/writ.go:13` — `WritConfig.Vars`: variables are already a supplemental field inside config.
- Section-level builtin floors exist as `WithDefaults()` (`internal/config/model.go:23`,
  `internal/config/registry.go:30`) — they fold into the announced constructors (the OTel `CreateDefaultConfig`
  shape).
- `cmd/star/config` — the in-house registration prior art (`ConfigSpec`, `RegisterExtension`, accessor), to be
  unified onto `devconfig`.
- The embedded `schema.*DefaultConfig` only seeds files at install (`internal/cli/selfinstall.go:466`); the target
  constructs the builtin floor at runtime.

## Open questions (tracked in the architecture doc)

- **Resolved-`Config` cardinality — RESOLVED 2026-06-12:** one `Config` per application process (a running-app
  singleton); apps lock into configuration, not sources; extension-aware apps resolve after discovery (built-in
  extensions announce at `init()`). See the architecture doc's "Cardinality" section.
- Builtin as runtime floor; schema versioning/migration hook; star unification timing; scope-composition home.

## Related

- [`docs/architecture/configuration.md`](../../../architecture/configuration.md) — the design of record.
- [`signing-options.md`](signing-options.md) — `SigningConfig`, the first non-op owner section.
- [`graph-signing.md`](graph-signing.md) — the signing mechanism whose config rides this model.
