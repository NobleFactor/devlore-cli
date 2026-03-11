# Architecture Documentation

Internal engineering documentation for devlore-cli developers.

Each architecture document has a companion `*.status.md` file tracking completion, PR links, document discrepancies, and outstanding work.

## Documents

### 1. System Model

- [System Model](1-system-model.md) ([status](1-system-model.status.md)) — Hosts, deployments, dependency taxonomy, receipt graph as system model, distributed orchestration vision

### 2. Execution Graph

- [Execution Graph](2-execution-graph.md) ([status](2-execution-graph.status.md)) — Unified graph design for all lifecycle commands
  - [Typed Slots](2.1-typed-slots.md) ([status](2.1-typed-slots.status.md)) — Slot model, resolution chain, providers, generated code
  - [Phase Execution](2.2-phase-execution.md) ([status](2.2-phase-execution.status.md)) — Saga pattern, phases, retry/rollback, compensation
  - [Orchestration Primitives](2.3-orchestration-primitives.md) ([status](2.3-orchestration-primitives.status.md)) — Gather, Choose, WaitUntil, Complete, Degraded, Fatal, Elevate, SlotProxy, lifecycle hooks
  - [Hermeticity Guarantees](2.4-hermeticity-guarantees.md) ([status](2.4-hermeticity-guarantees.status.md)) — Git-native input snapshots, scope separation (System/Home graphs), layer hermeticity

### 3. Providers

- [Action Namespaces](3-operation-namespaces.md) ([status](3-operation-namespaces.status.md)) — How to add new action namespaces to the execution engine
  - [Provider Loading and Lifetime](3.1-provider-loading.md) ([status](3.1-provider-loading.status.md)) — Module loading via `@devlore//`, provider lifetime model
  - [Projected Provider API](3.2-projected-provider-api.md) ([status](3.2-projected-provider-api.status.md)) — Reflection-based Go struct to Starlark bindings

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

### 7. Knowledge and LLM

- [Registry Knowledge Base](7-registry-knowledge.md) ([status](7-registry-knowledge.status.md)) — Knowledge domain structure in devlore-registry
  - [LLM Integration](7.1-llm-integration.md) ([status](7.1-llm-integration.status.md)) — Provider abstraction and prompt loading for AI-assisted commands
  - [E2E Testing](7.2-e2e-testing.md) ([status](7.2-e2e-testing.status.md)) — Multi-provider testing strategy for LLM commands

### 8. Rust Migration

- [Rust Migration](8-rust-migration.md) — Architecture decisions for porting devlore-cli to Rust (draft — long-term)

## Overview

The devlore-cli consists of two main tools that share a common execution engine:

- **writ** — File tree management (dotfiles, configs, templates)
- **lore** — Package management (software installation, configuration)

Both tools produce execution graphs that are processed by the shared engine in `pkg/op/` and `internal/execution/`.
