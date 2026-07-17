---
title: configuration — implementation plan (pkg/devconfig)
status: draft
created: 2026-06-11
updated: 2026-07-16
---

# Configuration — implementation plan

**Design of record: [`docs/architecture/configuration.md`](../../../architecture/configuration.md). The design is
king.** This plan's charter is to bring the code into agreement with that design; once the code agrees, the code
becomes the reference. The architecture doc carries the full model — foundation types, the recursive configuration
tree, the two announcement paths and collision policies, the per-key overlay with the path-keyed set-by sidecar and
declared-type instantiation, owner placement, the star unification shape, guarantees G1–G3, sequence diagrams, and
prior art. This document carries **sequencing and work items only**.

**Revised 2026-07-03.** The prior revision sequenced work against the flat predecessor `Config` (a
`map[string]Section`); the design has since settled the recursive tree, so every work item below targets the tree
directly. Nothing further builds on the predecessor — the reshape (item 1) precedes all other code. The 2026-06-13
sequencing correction stands and is now structural: `Application.Config` (item 2) gates the loader (item 3).

## Iteration loop (user-directed, 2026-06-12)

1. **Baseline** — add `pkg/devconfig`.
2. **Schema** — define config sections for the first owners.
3. **Operations** — importing a package registers its sections for the running app.
4. **Test, debug, refine** the design — return to 2.

## Landed (history)

- **Foundation types — the flat predecessor** (`pkg/devconfig/config.go` + tests): `Section` (interface) +
  `SectionBase`, `DataSection` (with its `starlark.Value` / `Mapping` / `IterableMapping` faces), the concrete
  `Config` (a `map[string]Section` — no `Path()`, no `ConfigBase`) with `Section` / `SectionOf` / `Provenance`,
  `SectionSpec`, `SectionConstructor`, `SettingSourceKind` (the flat set-by). The design names this shape **the
  predecessor**; item 1 reshapes it.
- **The announcement verbs + registry** (`pkg/devconfig/registry.go`): `AnnounceSection` (Go path, fatal) /
  `AnnounceSectionSpec` (data path, error-returning) plus the loader read API `AnnouncedSectionNames` /
  `ConstructorFor` / `SpecFor`.
- **First owner** — `op.RuntimeEnvironmentConfig` (`pkg/op/runtime_environment.go:652`), announced at `init()`
  (`pkg/op/runtime_environment.go:29`) with its builtin floor (`BackupSuffix: ".devlore-backup"`,
  `ConflictPolicy: ConflictStop`); `RuntimeEnvironmentSpec` no longer carries those two fields.
- **Reframed away** — the original item 1, "move `internal/config` → `pkg/devconfig`," is superseded by the design's
  owner placement: the foundation role went to `pkg/devconfig` (done), the typed sections (`LoreConfig`, `WritConfig`,
  `ModelConfig`, `RegistryConfig`) dissolve to their owners (item 4), and `internal/config.Load()` is superseded by
  the loader (item 3). No wholesale move remains.

## Queued work (re-sequenced 2026-07-03; dependency order)

1. **Tree reshape of `pkg/devconfig`** — the flat predecessor becomes the design's recursive tree
   (design §§ "The model" / "The configuration tree"):
   - `Config` becomes the **interface** (`Path()` / `Lookup(name)` / `Sections()`); **`ConfigBase`** is the
     embeddable, unnamed container (named by the struct field that holds it).
   - `Section` / `SectionBase` gain **`Path()`** — dotted, YAML-style, stamped top-down during the loader's typed
     descent (a stored byproduct of parsing; no parent back-pointers).
   - The **path-keyed child-type schema** — container members (dynamic keys inside a `Config`, e.g. brokers) are
     typed by a schema populated at import; a section announces against its **parent handle** and its path derives as
     `parent.Path() + name`. *Blocked by design pins 1 and 2 (below).*
   - The flat `SettingSourceKind` provenance (`pkg/devconfig/config.go:396`) reshapes to the **path-keyed
     `SetBy` / `History` override-chain sidecar** (`path → []Override{step, value}`; last entry wins; diagnostics
     only, never value access).
   - `base` / `profiles` / `applications` are **reserved resolver-level keys** — the hard-coded resolution axis;
     they never enter the schema or the resolved tree.
2. **`Application.Config` + minimal builtin resolution** — the former "step 1," retargeted at the tree; delivers
   "builtin as runtime floor":
   1. **`devconfig.Resolve()`** — snapshot the registry's announced **Go-path floors** into the resolved tree root
      (call each `ConstructorFor`; all-`SourceBuiltin` sidecar). Data-path floor construction (`SpecFor` →
      `DataSection`) is a flagged TODO until star sections exist.
   2. **`Application.Config`** — rename the existing `Config map[string]any` (`pkg/application/application.go:52`,
      the variable-resolver source) to `ConfigValues`, updating its readers (`pkg/op/variable_resolver.go:169`,
      `pkg/op/provider/plan/provider.go:468`, `pkg/op/runtime_environment.go:588`, the devlore-test setters); add
      the resolved-tree field, populated by `NewApplication` via `devconfig.Resolve()`. The one-`Config`-per-process
      singleton convention is enforced at the loader (item 3).
   3. **`NewRuntimeEnvironment` sources the runtime settings from `Application.Config`** — populate
      `re.BackupSuffix` via `devconfig.SectionOf[*RuntimeEnvironmentConfig]` rather than the floor constructor
      directly. Consumers read settings through `Application.Config`, never from per-call spec fields.

   **Consumer-migration facts (established 2026-06-13; references re-verified 2026-07-03):**
   - **`BackupSuffix`** — the one reader is `file.Provider.Backup`, reading
     `RuntimeEnvironment().BackupSuffix` at `pkg/op/provider/file/provider.go:94`, in another session's hands;
     coordinate the switch there.
   - **`ConflictPolicy`** — SUPERSEDED 2026-07-16 by step 49
     ([steps/49-conflict-policy-enforcement.md](steps/49-conflict-policy-enforcement.md)): the unread
     `RuntimeEnvironment.ConflictPolicy` field (`pkg/op/runtime_environment.go:60`) still **deletes**, but the
     SECTION SETTING gains its first real consumer — the file provider's write seam reads it live ({stop, skip,
     replace}; the enum collapses, replace always archives) — and writ's `--conflict` flag feeds the cli layer.
     Both halves land in step 49; the cli feed **waits for the loader** like DryRun.
   - **`DryRun`** — migration **waits for the loader**: it needs the CLI-flag overlay (the builtin floor alone
     cannot reflect `--dry-run`), so it stays on `Application.DryRun()` / `Flags` until item 3. Consumers today:
     `pkg/op/action_types.go:59`, `pkg/op/action_types.go:116`, `pkg/op/action_types.go:172`,
     `pkg/op/runtime_environment.go:620`.
   - **`policies.Transition`** — the executor's floor fallback constructs the section fresh
     (`NewPoliciesConfig().Transition`, `pkg/op/graph_executor.go:699`) instead of reading a resolved config —
     correct while builtin is the only source, wrong the moment the roll-up exists. Switch it to the
     `Application.Config` read (`devconfig.SectionOf[*PoliciesConfig]`) in item 2.3's sweep.

   **Open in item 2:** the `ConfigValues` name; whether the `RuntimeEnvironment.ConflictPolicy` deletion belongs in
   this item or its own cleanup.
3. **The loader** (design §§ "Resolution (the roll-up)" / "The loader is modular" / "Per-key application").
   koanf-backed providers realizing the confmap pattern; the five sources, each consulted **once** (builtin
   constructors, user `config.yaml`, app-elected project config, environment variables, CLI flags); the staged
   per-key overlay across **source × layer** (`base` < `profiles.<active>` < `applications.<app>` <
   `applications.<app>.profiles.<active>`, application-dominant, deep merge down matching paths); the loader as the
   active party — each value instantiated by its **declared type's own unmarshaler** (no read-time conversion
   anywhere), each assignment appended to the `SetBy` / `History` sidecar; unknown keys reported loudly; `${VAR}`
   expansion as a Converter pass; the one-resolved-`Config`-per-application-process singleton; apps lock into
   configuration, not sources (no live re-reads — the deliberate break from viper).
   **Validation + preflight (settled):** after the roll-up the loader walks the resolved tree calling per-section
   `Validate()` (recursive, path-qualified errors, self-contained to a section's own values + sub-tree); cross-tree /
   graph↔config checks are the consuming provider's **run-start preflight** (an unresolved reference refuses the
   run), never the config layer's. *Blocked by design pins 3 and 4 (below).*
4. **Owner-located sections** (first wave): `pkg/op` — the runtime section, **landed** (see "Landed"); `pkg/op` —
   the **policies section** (`PoliciesConfig`, path `policies`: `Retry op.RetryPolicy` — the subgraph-combinator
   default, step 35 — + `Transition TransitionPolicy` {degraded/execution_failed/compensation_failed →
   continue/pause/stop; continue illegal for compensation_failed}, floors continue/stop/stop; settled 2026-07-06,
   design in
   [compensation-failure-contract.md §"TransitionPolicy — Q3 settled"](compensation-failure-contract.md));
   `pkg/signing` — `SigningConfig` (see [`signing-options.md`](signing-options.md)); the registry section — owner to
   be extracted
   from `internal/` (working name `pkg/devregistry`), absorbing `internal/config/registry.go`; the model/LLM section
   likewise, absorbing `internal/config/model.go`; the lore/writ app sections dissolving `internal/config/lore.go` /
   `internal/config/writ.go` to their apps; and the **elevation** provider's config section — a **provider section
   with a broker sub-tree** (`providers.elevation` → `brokers` → a section per broker, each fronting its services),
   realized through the recursive `Config` / `ConfigBase` tree and the `base` / `profiles` / `applications` layers —
   **not** a flat `offers` + `brokers` section, and **not** an in-section `environments:` map. A **broker is any type
   fulfilling the provider's broker interface** (no base, no registration, no fixed home); the elevation provider
   builds a **router** — itself a broker — that **allocates and configures its sub-brokers from the resolved config**
   and delegates (the recursion bottoms out at leaf sub-brokers, the former "services"). `op` supplies only the
   interface and typed config tree — no `op.AnnounceBroker`, no `op.WireBrokers`, no global registry, no
   `op.BrokerRegistry` trait; the `pkg/platform` `compositeManager` is the worked router precedent (full model —
   [Projected Provider API → Pluggable
   brokers](../../../architecture/3.2-projected-provider-api.md#pluggable-brokers--provider-owned-routers)).
   See the worked shape in
   [the elevation case study](../../../architecture/configuration.md#case-study-the-elevation-section)
   and the full elevation design in [`6.1-privilege-elevation.md`](../../../architecture/6.1-privilege-elevation.md).
   Elevation **policy** (as distinct from the config shape, which is settled) is phase-8 step 38
   ([steps/38-elevation-policy.md](steps/38-elevation-policy.md)).
5. **Variables + retirement of the flat sources.** The variable resolver becomes a thin reader over the rolled-up
   config (`Vars` as the supplemental Make-style section, resolved by the same roll-up, expanded by the loader's
   Converter pass — design § "Variables — supplemental"); retire `Application.ConfigValues` and the package-global
   `viper` reads (`internal/cli/viper.go`, `internal/cli/root.go`, and the cmd-side readers); star's
   `Application.Overrides["config"]` hack retires with item 6.
6. **Star unification.** Shape defined (architecture doc: two paths, G1–G3, project source layer, dotted-name
   flattening, the travel form, and the script migration `.get` → indexing); **timing open** — its own work item,
   not part of the first iterations. **Travel form settled:** a lazy reflection adapter projects any section as the
   sealed `Mapping` (uniform across the root `Config` / typed sections / `ConfigBase` / `DataSection`); a
   struct-valued setting crosses as a `goReceiver` through the existing reflection framework.

7. **Config intake close-out — every outstanding setting lands in a section, or is ruled out.** The framework
   pieces (items 1–3: the tree, `Application.Config`, the loader with its env + cli sources) unblock a queue of
   settings accumulated while they were missing. This item empties that queue; nothing may keep reading viper,
   `Application.Flags`, or a freshly-constructed floor once it closes. The sections and their owners:
   1. **`runtime`** (`pkg/op`, announced): the `DryRun` cli feed (consumer-migration facts above); the
      `ConflictPolicy` provider read + writ `--conflict` cli feed (step 49 owns both halves); the `BackupSuffix`
      consumer switch (coordinate with the session that owns `file.Provider.Backup`).
   2. **`policies`** (`pkg/op`, announced): the executor's floor fallback becomes the live resolved-config read
      (fact above); the step-41 `transition_policy=` kwarg tier is unaffected (per-unit beats config).
   3. **`writ`** (new; `cmd/writ` owns, dissolving `internal/config/writ.go` per item 4): today's viper keys
      `writ.dry-run`, `writ.verbose`, `writ.repo`, `writ.vars` — read by `cmd/writ/writ/config.go`'s parse
      functions. `writ.vars` becomes the render-data vars source; `writ.repo` the single-source root.
   4. **`lore`** (new; `cmd/lore` owns, dissolving `internal/config/lore.go`): `lore.dry-run`, `lore.verbose`.
   5. **`model`** (new; owner absorbs `internal/config/model.go` per item 4): `provider` / `endpoint` / `model` /
      `api_key`, unifying the three hand-rolled sources documented in `internal/model/config.go` (viper
      `lore.model.*`, `DEVLORE_MODEL_*` env, `--model-*` flags) into the loader's source overlay. Shared by lore
      and writ migrate's AI analysis.
   6. **The registry section** (owner extracted to `pkg/devregistry` per item 4, absorbing
      `internal/config/registry.go`) — lore's package-registry location.
   7. **`signing`** (`pkg/signing` when step 46 builds it; shape in [signing-options.md](signing-options.md)) —
      the scheme and identity/key source are step 46 design questions; the section lands with the signer.
   8. **`providers.elevation`** (the broker sub-tree, item 4 / step 38) — the worked recursive-tree case.

   **Ruled OUT of config** (boundaries to keep loud): sops **encryption** settings stay in file-anchored
   `.sops.yaml` (git-style upward discovery per [sops-config-discovery.md](sops-config-discovery.md)) — never the
   roll-up — and **decryption is config-free** (ambient identities); the store's locations (`GraphsDir()` /
   `ReceiptsDir()` / the run index under the XDG state home) are convention, not configuration; writ's layer
   tree (`WritLayersDir()`) is packaging, not configuration (the settled config-vs-layers separation).

## Design pins — questions the design of record must answer before the item they block

The design is king; these are the points where it is silent. Each blocks the work item named; none blocks the items
before it.

1. **The parent-handle announce form** *(blocks item 1)* — the tree section says a child section "is announced
   against its **parent handle**," but `AnnounceSection` (`pkg/devconfig/registry.go:139`) takes
   `(reflect.Type, SectionConstructor)` with no parent parameter, and no parent *instance* exists at `init()` time.
   What is the handle concretely — a type, a path string, a registration token returned by the parent's announce?


2. **Child-type schema keying** *(blocks item 1)* — are container members typed per **container path** ("children of
   `providers.elevation.brokers` are `elevation.BrokerConfig`") or per **full member path**? If per container, can
   one container hold heterogeneous section types (the leaf sub-brokers — `step-ca` vs. `local` — read as distinct
   unexported concrete types under one `Services` container)?
3. **Environment/CLI key mapping** *(blocks item 3)* — the convention mapping `DEVLORE_*` / `<APP>_*` variable names
   and pflag names onto dotted tree paths (what addresses `providers.elevation.brokers.ssh.default_ttl`?) is
   unspecified.
4. **Dotted-flat data sections under the tree** *(blocks item 3; touches item 6)* — star demanded "dotted names,
   flat sections" (`lint.copyright` as one flat section named `"lint.copyright"`), which predates the tree. Under the
   tree, is `lint.copyright` still a single root-level key, or `lint` → `copyright` nesting? The readings yield
   different YAML files and different `config[...]` navigation.

Also for the design owner: the marker "One point remains open (end of section)" at
[`configuration.md` § The configuration tree](../../../architecture/configuration.md#the-configuration-tree) no
longer points at an identifiable open point — either it resolved (remove the marker) or the point was lost (restore
it).

## The model today (facts that stay true; references re-verified 2026-07-03)

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
  constructs the builtin floor at runtime — delivered by item 2.

## Open questions (tracked in the architecture doc)

- **Resolved-`Config` cardinality — RESOLVED 2026-06-12:** one `Config` per application process (a running-app
  singleton); apps lock into configuration, not sources; extension-aware apps resolve after discovery (built-in
  extensions announce at `init()`). See the architecture doc's "Cardinality" section.
- **Builtin as runtime floor — delivered by item 2** (the doc's open should close when item 2 lands).
- **Scope-composition home** — one shared assembly package vs. each app composing its own scope structs.
- **Schema versioning** — when (not whether) to add the Kubernetes-style migration hook; deferred by design.
- **Star unification timing** — the shape is defined; when to execute the fold is item 6's own decision.

## Related

- [`docs/architecture/configuration.md`](../../../architecture/configuration.md) — the design of record.
- [`signing-options.md`](signing-options.md) — `SigningConfig`, the first non-op owner section.
- [`graph-signing.md`](graph-signing.md) — the signing mechanism whose config rides this model.
- [`steps/38-elevation-policy.md`](steps/38-elevation-policy.md) — elevation policy, deliberately separate from this
  plan (the config model is settled; the policy is not).
