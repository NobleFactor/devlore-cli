# Configuration — Status

**Document:** [configuration.md](configuration.md)
**State:** Design draft (2026-06-12).
**Implementation:** Not started. Plan + iterative sequencing:
[`docs/plans/extract-starlark-from-op/phase-8/configuration.md`](../plans/extract-starlark-from-op/phase-8/configuration.md).

## Completion

- [x] Design — distributed-participation model; `devconfig.{Config, Section, Setting}`; import-time announcement via
  `devconfig.AnnounceSection` (fourth member of the `Announce*` family; reflect.Type-keyed, name-fetched,
  fatal-on-collision; schema registry process-wide, resolved values per-`Application`); two-axis ordered-overlay
  roll-up; per-key overlay with loader-stamped provenance and declared-type conversion; placement principle;
  prior-art synthesis (star / OpenTelemetry / Kubernetes / Go idioms / koanf).
- [ ] `pkg/devconfig` foundation types (`Config` / `Section` / `Setting`).
- [ ] Section definition — Go-typed path (reflect over a struct) **and** data path (`ConfigSpec`).
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
