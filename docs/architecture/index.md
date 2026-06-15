# Architecture Documentation

Internal engineering documentation for devlore-cli developers.

Each architecture document has a companion `*.status.md` file tracking completion, PR links, document discrepancies, and outstanding work.

## Documents

### 1. System Model

- [System Model](1-system-model.md) ([status](1-system-model.status.md)) — Hosts, deployments, dependency taxonomy, receipt graph as system model, distributed orchestration vision

### Configuration

- [Configuration](configuration.md) ([status](configuration.status.md)) — Distributed config participation: `devconfig.{Config, Section, Setting}`, schema announcement and the section registry, the two-axis roll-up, owner-located sections, and prior art (star / OpenTelemetry / Kubernetes)

### 2. Execution Graph

- [Execution Graph](2-execution-graph.md) ([status](2-execution-graph.status.md)) — Unified graph design for all lifecycle commands
  - [Typed Slots](2.1-typed-slots.md) ([status](2.1-typed-slots.status.md)) — Slot model, resolution chain, providers, generated code
  - [Phase Execution](2.2-phase-execution.md) ([status](2.2-phase-execution.status.md)) — Saga pattern, phases, retry/rollback, compensation
  - [Orchestration Primitives](2.3-orchestration-primitives.md) ([status](2.3-orchestration-primitives.status.md)) — Gather, Choose, WaitUntil, Complete, Degraded, Fatal, Elevate, SlotProxy, lifecycle hooks
  - [Hermeticity Guarantees](2.4-hermeticity-guarantees.md) ([status](2.4-hermeticity-guarantees.status.md)) — Git-native input snapshots, scope separation (System/Home graphs), layer hermeticity
  - [Lifecycle Pipeline Construction](2.5-lifecycle-pipeline-construction.md) ([status](2.5-lifecycle-pipeline-construction.status.md)) — How commands build per-OS lifecycle-pipeline graphs from packages: phase-script→subgraph harvest, Origin/provenance, build/save/load/run portability
  - [Execution Policies: Retry, Elevation, and Error Handling](2.6-execution-policies.md) ([status](2.6-execution-policies.status.md)) — The per-unit policy triplet, its floor-plus-inherit resolution at plan time, the maturity gradient (retry live / error-action observation-only / elevation dormant), and elevation's requirement/offer/provision/realization model

### 3. Providers

- [Action Namespaces](3-operation-namespaces.md) ([status](3-operation-namespaces.status.md)) — How to add new action namespaces to the execution engine
  - [Provider Loading and Lifetime](3.1-provider-loading.md) ([status](3.1-provider-loading.status.md)) — Module loading via `@devlore//`, provider lifetime model
  - [Projected Provider API](3.2-projected-provider-api.md) ([status](3.2-projected-provider-api.status.md)) — Reflection-based Go struct to Starlark bindings
  - [Static Starlark Value Codegen](3.3-static-starlark-codegen.md) — Generated starlark.Value implementations replacing reflection (draft)
  - [Platform and Package Management](3.4-platform-package-managers.md) ([status](3.4-platform-package-managers.status.md)) — `op.Platform`, the Composite `op.PackageManager` router, purl routing, the driver catalog, and state-query verification (design)

### 4. Resources

- [Resource Management](4-resource-management.md) ([status](4-resource-management.status.md)) — URI-based resource tracking, lineage, shadowing, tombstone unification
  - [Resource Identity](4.1-resource-identity.md) ([status](4.1-resource-identity.status.md)) — URI schemes, opaque vs hierarchical, interface simplification
  - [Memory Resources](4.2-mem-resource.md) ([status](4.2-mem-resource.status.md)) — `mem:` scheme, callable serialization lifecycle
  - [Resource Registration](4.3-resource-registration.md) ([status](4.3-resource-registration.status.md)) — Two-phase announce/init lifecycle, lazy constructor registration, callable generalization
  - [Root-Path Triad](4.4-root-path-triad.md) ([status](4.4-root-path-triad.status.md)) — Root interface, Path struct, RecoverySite, OS-enforced I/O confinement

### 5. Operational Integrity

- [Receipt Integrity](5-receipt-integrity.md) ([status](5-receipt-integrity.status.md)) — Checksum and signature verification for receipts
  - [Reconciliation](5.1-reconciliation.md) ([status](5.1-reconciliation.status.md)) — Audit, reconciliation, and recovery in the execution graph
  - [Recovery Serialization](5.2-recovery-serialization.md) ([status](5.2-recovery-serialization.status.md)) — Recovery stack serialization and restart (planned)
  - [Recovery Site](5.3-recovery-site.md) ([status](5.3-recovery-site.status.md)) — Shared recovery service for file archival (zero-copy rename) and data archival (byte serialization)

### 6. Execution Topology

- [Execution Topology](6-execution-topology.md) ([status](6-execution-topology.status.md)) — Elevation, remote execution, telemetry (planned)
  - [Privilege Elevation: The Elevator Provider](6.1-privilege-elevation.md) ([status](6.1-privilege-elevation.status.md)) — The elevator provider: graph/config/runtime split, the two strategies (ProcessSpawn / IdentityAssumption), the token-provider mechanism, the config outline, and failure routing

### 7. Knowledge and LLM

- [Registry Knowledge Base](7-registry-knowledge.md) ([status](7-registry-knowledge.status.md)) — Knowledge domain structure in devlore-registry
  - [LLM Integration](7.1-llm-integration.md) ([status](7.1-llm-integration.status.md)) — Provider abstraction and prompt loading for AI-assisted commands
  - [E2E Testing](7.2-e2e-testing.md) ([status](7.2-e2e-testing.status.md)) — Multi-provider testing strategy for LLM commands

### 8. Rust Migration

- [Rust Migration](8-rust-migration.md) — Architecture decisions for porting devlore-cli to Rust (draft — long-term)

### 9. Star

- [Star Extensions](star-extensions.md) — Extension model, discovery, loading, runtime types (`star.Extension`, `star.Command`)
  - Star Configuration — Config hierarchy, schema, on-demand resolution (planned)
  - Star WASM Receivers — WASM sandboxing, capabilities, host module (planned)

## Overview

The devlore-cli consists of three tools that share a common execution engine:

- **writ** — File tree management (dotfiles, configs, templates)
- **lore** — Package management (software installation, configuration)
- **star** — Starlark-powered operations tool (linting, setup, hooks, project automation)

All three tools consume the shared provider framework in `pkg/op/` and the starlark runtime. Writ and lore produce execution graphs processed by the engine in `internal/execution/`. Star uses a lightweight command model where extensions define commands in Starlark and providers supply the bindings.
