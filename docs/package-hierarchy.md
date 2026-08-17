# Package Hierarchy

> **Status:** rewritten 2026-07-22 (phase-8 step 51, slice 7) from the live tree. The former inventory described
> the pre-`op` layout (`internal/execution`, `internal/writ/*`, `internal/lore`, `internal/starlark`) — all
> deleted. Disposition (step-51 decision (c)): **compact and hand-maintained**; generating it is a possible future
> charter if churn warrants. Per-package one-liners live in [package-reference.md](package-reference.md); the
> architecture set ([docs/architecture/index.md](architecture/index.md)) owns design.

## The three layers

```
cmd/        the binaries — thin commands over the framework
  │ imports
  ▼
internal/   shared application infrastructure (CLI store, config, model, registry, …)
  │ imports
  ▼
pkg/        the public framework — the op engine, providers, and op-free capabilities
```

The dependency rule: `pkg/` never imports `internal/` or `cmd/`; `internal/` imports `pkg/`; each binary under
`cmd/` imports both. Within `pkg/`, `op` is the hub — the op-free capability packages (`fsroot`, `platform`,
`status`, `result`, `sink`, `process`, `sops`, `signing`, `devconfig`, …) stand alone beneath it.

## The tree

```
cmd/
  writ/writ/            the writ CLI: adopt, decommission, deploy, identity, migrate, readback,
                        segment, snapshot, status, tree, upgrade, verify
  lore/lore/            the lore CLI runtime + onboard/
  star/                 the star CLI: cli/, config/, inventory/, and the tool providers under provider/
                        (commands, config, goast, lint, setup, shellcheck, staranalysis,
                        starcode, starcomplexity, starindex, starstats)
  devlore-test/         the fixture harness CLI (devloretest/)
  devlore-docs/         the CLI reference generator (published to devlore.noblefactor.com)
  devlore-index/        the index generator
  devlore-inventory/    the op inventory generator (blank-import files)
  internal/             shared command infrastructure — importable only from cmd/...
    cli/                the graph/trace store, run index, narrator bootstrap
    config/             centralized ecosystem configuration
    devlore/            the locations the tools share, named over pkg/xdg
    e2e/                end-to-end LLM test harness
    lorepackage/        lore package model + resolution
    model/              LLM provider abstraction (anthropic, gemini, groq, ollama, openai)

internal/
  console/              interactive terminal UI for guided workflows
  credentials/          OS-native credential storage
  document/             structured YAML/JSON document I/O
  manifest/             packages-manifest loading and validation
  registry/             devlore-registry transport

pkg/
  op/                   the engine: sealed Graph, GraphExecutor, receipts, catalog, run-state machine,
                        control plane; starlarkbridge/ (the projection), server/ (the wire listener),
                        inventory/ (generated announcement roster)
  op/provider/          the 18-provider catalog (see architecture/3.5-provider-catalog.md) + elevator stub
  application/          the per-tool Application handle
  assert/               invariant-check vocabulary
  devconfig/            the unified configuration model
  fsroot/               the confined filesystem root + Path
  gitignore/            gitignore-aware filtering
  iox/                  standalone I/O utilities
  platform/             op-free platform capability + the Composite package-manager router
  process/              the os/exec ↔ narration/result bridge
  result/               the primary structured-output channel
  signing/              graph/trace signing + verification (ssh-ed25519)
  sink/                 the byte-out endpoint contract
  sops/                 SOPS decryption/encryption over getsops
  status/               the human narration side-channel (Narrator)
```
