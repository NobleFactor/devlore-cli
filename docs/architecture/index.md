# Architecture Documentation

Internal engineering documentation for devlore-cli developers.

## Documents

### 1. System Model

- [System Model](1-system-model.md) — Hosts, deployments, dependency taxonomy, receipt graph as system model, distributed orchestration vision

### 2. Execution Graph

- [Execution Graph](2-execution-graph.md) — Unified graph design for all lifecycle commands
  - [Typed Slots](2.1-typed-slots.md) — Slot model, resolution chain, providers, generated code
  - [Phase Execution](2.2-phase-execution.md) — Saga pattern, phases, retry/rollback, compensation
  - [Orchestration Primitives](2.3-orchestration-primitives.md) — Gather, Choose, WaitUntil, Complete, Degraded, Fatal, Elevate, SlotProxy, lifecycle hooks

### 3. Providers

- [Action Namespaces](3-operation-namespaces.md) — How to add new action namespaces to the execution engine
  - [Provider Loading and Lifetime](3.1-provider-loading.md) — Module loading via `@devlore//`, `With()` opt-in, three-level provider lifetime model (stateless, phase, session)
  - [Projected Provider API](3.2-projected-provider-api.md) — Code generation from Go struct to Starlark bindings

### 4. Resources

- [Resource Management](4-resource-management.md) — URI-based resource tracking, lineage, shadowing, tombstone unification
  - [Resource Identity](4.1-resource-identity.md) — URI schemes, opaque vs hierarchical, interface simplification
  - [Memory Resources](4.2-mem-resource.md) — `mem:` scheme, callable serialization lifecycle

### 5. Operational Integrity

- [Receipt Integrity](5-receipt-integrity.md) — Checksum and signature verification for receipts
  - [Reconciliation](5.1-reconciliation.md) — Audit, reconciliation, and recovery in the execution graph
  - [Recovery Serialization](5.2-recovery-serialization.md) — Recovery stack serialization and restart (planned)

### 6. Execution Topology

- [Execution Topology](6-execution-topology.md) — Elevation, remote execution, telemetry (planned)

### 7. Knowledge and LLM

- [Registry Knowledge Base](7-registry-knowledge.md) — Knowledge domain structure in devlore-registry
  - [LLM Integration](7.1-llm-integration.md) — Provider abstraction and prompt loading for AI-assisted commands
  - [E2E Testing](7.2-e2e-testing.md) — Multi-provider testing strategy for LLM commands

### 8. Rust Migration

- [Rust Migration](8-rust-migration.md) — Architecture decisions for porting devlore-cli to Rust

## Overview

The devlore-cli consists of two main tools that share a common execution engine:

- **writ** — File tree management (dotfiles, configs, templates)
- **lore** — Package management (software installation, configuration)

Both tools produce execution graphs that are processed by the shared engine in `internal/execution/`.
