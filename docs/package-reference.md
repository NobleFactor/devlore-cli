# devlore-cli Package Reference

> **Status:** rewritten 2026-07-22 (phase-8 step 51, slice 7) from the live tree as a **one-line-per-package
> roster** (the former per-symbol element listing described the deleted pre-`op` layout and is not maintainable by
> hand — symbol-level reference is `go doc`'s job). Disposition (step-51 decision (c)): compact and
> hand-maintained; a generator is a possible future charter. The layered map is
> [package-hierarchy.md](package-hierarchy.md); design is the architecture set.

## pkg — the public framework

| Package | Purpose |
|---|---|
| `pkg/op` | the engine: sealed `Graph`, `GraphExecutor`, receipts + recovery, `ResourceCatalog`, run-state machine, control plane, hooks |
| `pkg/op/starlarkbridge` | the Starlark↔Go projection: `goReceiver`, the Converter, the Invoker |
| `pkg/op/controlhttp` | the HTTP/2 (h2c) wire listener for the control plane (architecture 2.7) |
| `pkg/op/inventory` | generated blank-import roster of announced providers |
| `pkg/op/provider/*` | the provider catalog — 18 action providers + the `elevator` stub ([3.5](architecture/3.5-provider-catalog.md), design docs 3.5.1–3.5.16) |
| `pkg/application` | the per-tool `Application` handle the framework carries on its runtime environment |
| `pkg/assert` | uniform vocabulary for invariant checks |
| `pkg/devconfig` | the domain-free unified configuration model ([configuration.md](architecture/configuration.md)) |
| `pkg/fsroot` | the confined filesystem root: `Root`, `Path`, OS-enforced I/O confinement ([4.4](architecture/4.4-root-path-triad.md)) |
| `pkg/gitignore` | gitignore-aware file filtering over go-git |
| `pkg/iox` | standalone I/O utilities, op-free |
| `pkg/platform` | the platform capability + Composite package-manager router, op-free ([3.4](architecture/3.4-platform-package-managers.md)) |
| `pkg/process` | the single bridge between `os/exec` and narration/result emission |
| `pkg/result` | the primary output channel: `Pipeline` (filter → formatter → sink) |
| `pkg/signing` | signing + verification of graphs and traces, ssh-ed25519 ([5](architecture/5-receipt-integrity.md)) |
| `pkg/sink` | the byte-out endpoint contract (`Stdout` / `Stderr` / `Discard`) |
| `pkg/sops` | SOPS decryption/encryption over getsops |
| `pkg/status` | the human narration side-channel: `Narrator` ([2.8](architecture/2.8-eventing-infrastructure.md)) |

## internal — shared application infrastructure

| Package | Purpose |
|---|---|
| `internal/cli` | shared CLI infrastructure: the graph/trace store (`WriteGraph`/`WriteTrace`), the NDJSON run index, narrator bootstrap (`SetUI`/`UI`) |
| `internal/config` | centralized ecosystem configuration |
| `internal/console` | interactive terminal UI for guided workflows |
| `internal/credentials` | OS-native credential storage |
| `internal/document` | structured YAML/JSON document I/O |
| `internal/e2e` | the end-to-end LLM test harness ([7.2](architecture/7.2-e2e-testing.md)) |
| `internal/lorepackage` | the lore package model + resolution constants |
| `internal/manifest` | packages-manifest loading and validation |
| `internal/model` | the LLM provider abstraction: anthropic / gemini / groq / ollama / openai, `EnsureProvider` ([7.1](architecture/7.1-llm-integration.md)) |
| `internal/pwsh` | persistent PowerShell session for lore |
| `internal/registry` | devlore-registry transport |
| `internal/tools/docgen` | Docker-style CLI reference generation |

## cmd — the binaries

| Package | Purpose |
|---|---|
| `cmd/writ/writ` | the writ CLI; subpackages per command family: adopt, decommission, deploy, identity, migrate, readback, segment, snapshot, status, tree, upgrade, verify |
| `cmd/lore/lore` | the lore CLI runtime (builder over the shared plan provider) + `onboard/` |
| `cmd/star` | the star CLI + code generator: `cli/`, `config/`, `inventory/`, tool providers under `provider/` (commands, config, goast(+doctaxonomy), lint, setup, shellcheck, staranalysis, starcode, starcomplexity, starindex, starstats) |
| `cmd/devlore-test/devloretest` | the fixture harness CLI driving the `.star` corpus |
| `cmd/docgen`, `cmd/indexgen` | documentation and index generators |
