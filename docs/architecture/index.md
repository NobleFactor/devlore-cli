# Architecture Documentation

Internal engineering documentation for devlore-cli developers.

## Documents

### System Architecture

- [Emergent System Model](devlore-emergent-system-model.md) - System-level architecture: hosts, deployments, dependency taxonomy (structural, functional, procedural), receipt graph as system model, distributed orchestration vision

### Execution Engine

- [Execution Graph](devlore-execution-graph.md) - Unified graph design for all lifecycle commands
- [Phase Execution](devlore-phase-execution.md) - Saga pattern, phases, retry/rollback, compensation
- [Typed Slots](devlore-typed-slots.md) - Slot model, resolution chain, providers, generated code
- [Orchestration Primitives](devlore-orchestration-primitives.md) - Gather, Choose, WaitUntil, SlotProxy, ActivationState, Sidecar hooks
- [Graph Operations](devlore-graph-convergence-operations.md) - Original convergence and control flow (partially superseded by Orchestration Primitives)
- [Action Namespaces](devlore-operation-namespaces.md) - How to add new action namespaces to the execution engine
- [Provider Loading and Lifetime](devlore-provider-loading.md) - Module loading via `@devlore//`, `With()` opt-in, three-level provider lifetime model (stateless, phase, session)
- [Receipt Integrity](devlore-receipt-integrity.md) - Checksum and signature verification for receipts

### Tooling

- [LLM Integration](devlore-llm-integration.md) - Provider abstraction and prompt loading for AI-assisted commands
- [Registry Knowledge Base](devlore-registry-knowledge.md) - Knowledge domain structure in devlore-registry
- [E2E Testing](devlore-e2e-testing.md) - Multi-provider testing strategy for LLM commands

## Overview

The devlore-cli consists of two main tools that share a common execution engine:

- **writ** - File tree management (dotfiles, configs, templates)
- **lore** - Package management (software installation, configuration)

Both tools produce execution graphs that are processed by the shared engine in `internal/execution/`.
