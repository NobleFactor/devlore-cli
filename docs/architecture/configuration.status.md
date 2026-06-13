# Configuration — Status

**Document:** [configuration.md](configuration.md)
**State:** Design draft (2026-06-12).
**Implementation:** Not started. Plan + iterative sequencing:
[`docs/plans/extract-starlark-from-op/phase-8/configuration.md`](../plans/extract-starlark-from-op/phase-8/configuration.md).

## Completion

- [x] Design — distributed-participation model; `devconfig.{Config, Section}` with **plain typed settings**
  (`Setting[T]` withdrawn; the section is the fetch unit — `SectionOf[T]` + owner wrappers; sealed after the build);
  the **kv section variant** as data-path section and starlark travel form (`starlark.Value` + `Mapping` +
  `IterableMapping`; `HasAttrs` dropped; script migration `.get`→indexing detailed); import-time announcement via
  `devconfig.AnnounceSection` (fourth member of the `Announce*` family; reflect.Type-keyed, name-fetched,
  fatal-on-collision; schema registry process-wide; one resolved `Config` per application process, built at startup —
  the *config build*, a runtime event, not `go build`); two-axis ordered-overlay roll-up; per-key overlay with
  sidecar provenance, values instantiated by declared types' own unmarshalers (no read-time conversion); placement
  principle; prior-art synthesis (star / OpenTelemetry / Kubernetes / Go idioms / koanf); the data-path schema is
  **tagged `defaults:`** — each value's YAML tag declares its setting's type (Go `:=`-style), containers untyped.
- [x] `pkg/devconfig` foundation types — `Config`, `Section` (interface) + `SectionBase`, `DataSection` (with the
  `starlark.Value` + `Mapping` + `IterableMapping` faces), `SectionSpec`, `SectionConstructor`, `SettingSourceKind`;
  `Config` accessors (`Section` / `SectionOf[T]` / `Provenance`), `DataSection` reads (`Lookup` / `Get[T]`), and the
  closed `toStarlark` projection. Landed in `pkg/devconfig/config.go` (+ `config_test.go`). **Not yet:** the
  announcement verbs + registry, the loader/build, and `Config`'s own `starlark` face (boundary / star-unification).
- [ ] Section definition — Go-typed path (reflect over a struct) **and** data path (tagged `defaults:` — YAML tags
  declare setting types; untyped containers).
- [ ] Modular loader (koanf) + the staged overlay; `${…}` Converter for variable expansion.
- [ ] Owner-located sections — `signing` (`pkg/signing`), model, registry, op runtime.
- [ ] Unify `cmd/star/config` onto `devconfig`.
- [ ] Retire `internal/config` and the package-global `viper` reads.

## Outstanding / open questions

- Scope-composition home (one shared assembly package vs. per-app).
- Builtin as runtime floor (today the embedded defaults only seed files at install).
- Schema versioning + migration — the held-in-reserve Kubernetes idea.
- Star unification sequencing — shape defined in the doc ("Star unification and the two announcement paths": two
  collision policies, build-time snapshot, project source layer, dotted-name flattening, guarantees G1–G3, sequence
  diagrams for both paths); timing open.

## Discrepancies (design vs. current code)

- `internal/config` is today's model — flat and centralized; this design moves it to `pkg/devconfig` and reshapes it
  into the registry.
- `cmd/star/config` is today's registration system — data-driven and star-only; this design generalizes it to cover
  Go participants and unifies the two.
