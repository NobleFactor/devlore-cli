# Configuration — Status

**Document:** [configuration.md](configuration.md)
**State:** Design draft (2026-06-12).
**Implementation:** Not started. Plan + iterative sequencing:
[`docs/plans/extract-starlark-from-op/phase-8/configuration.md`](../plans/extract-starlark-from-op/phase-8/configuration.md).

## Completion

- [x] Design — distributed-participation model; `devconfig.{Config, Section}` with **plain typed settings**
  (`Setting[T]` withdrawn; the section is the fetch unit — `SectionOf[T]` + owner wrappers; sealed after resolution);
  the **kv section variant** as data-path section and starlark travel form (`starlark.Value` + `Mapping` +
  `IterableMapping`; `HasAttrs` dropped; script migration `.get`→indexing detailed); import-time announcement via
  `devconfig.AnnounceSection` (fourth member of the `Announce*` family; reflect.Type-keyed, name-fetched,
  fatal-on-collision; schema registry process-wide; one resolved `Config` per application process, built at startup —
  resolution, a runtime event, not a compile step); two-axis ordered-overlay roll-up; per-key overlay with
  sidecar provenance, values instantiated by declared types' own unmarshalers (no read-time conversion); placement
  principle; prior-art synthesis (star / OpenTelemetry / Kubernetes / Go idioms / koanf); the data-path schema is
  **tagged `defaults:`** — each value's YAML tag declares its setting's type (Go `:=`-style), containers untyped.
- [x] `pkg/devconfig` foundation types — `Config`, `Section` (interface) + `SectionBase`, `DataSection` (with the
  `starlark.Value` + `Mapping` + `IterableMapping` faces), `SectionSpec`, `SectionConstructor`, `SettingSourceKind`;
  `Config` accessors (`Section` / `SectionOf[T]` / `Provenance`), `DataSection` reads (`Lookup` / `Get[T]`), and the
  closed `toStarlark` projection. Landed in `pkg/devconfig/config.go` (+ `config_test.go`). **Not yet:** the
  loader and `Config`'s own `starlark` face (boundary / star-unification).
- [x] Announcement — `AnnounceSection` (Go path, fatal on collision) / `AnnounceSectionSpec` (data path, error) over
  the process-wide schema registry; loader read API `AnnouncedSectionNames` / `ConstructorFor` / `SpecFor`. Landed in
  `pkg/devconfig/registry.go` (+ `registry_test.go`). First owner: `op.RuntimeEnvironmentConfig`
  (`pkg/op/runtime_environment.go`), read live via `Application.Config` (builtin floor).
- [ ] Section definition — Go-typed path (reflect over a struct) **and** data path (tagged `defaults:` — YAML tags
  declare setting types; untyped containers).
- [ ] Modular loader (koanf) + the staged overlay; `${…}` Converter for variable expansion.
- [ ] Owner-located sections — `op` runtime **landed** (`RuntimeEnvironmentConfig`); `signing`
  (`pkg/signing`, not yet created), model, registry pending.
- [ ] Unify `cmd/star/config` onto `devconfig`.
- [ ] Retire `internal/config` and the package-global `viper` reads.

## Outstanding / open questions

- Scope-composition home (one shared assembly package vs. per-app).
- Builtin as runtime floor (today the embedded defaults only seed files at install).
- Schema versioning + migration — the held-in-reserve Kubernetes idea.
- Star unification sequencing — shape defined in the doc ("Star unification and the two announcement paths": two
  collision policies, snapshot taken at resolution, project source layer, dotted-name flattening, guarantees G1–G3, sequence
  diagrams for both paths); timing open.

## Discrepancies (design vs. current code)

- `internal/config` is today's model — flat and centralized; this design moves it to `pkg/devconfig` and reshapes it
  into the registry.
- `cmd/star/config` is today's registration system — data-driven and star-only; this design generalizes it to cover
  Go participants and unifies the two.
