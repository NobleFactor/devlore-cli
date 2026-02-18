# Architecture Documentation

Internal engineering documentation for devlore-cli developers.

## Documents

- [Execution Graph](devlore-execution-graph.md) - Unified graph design for all lifecycle commands
- [Graph Operations](devlore-graph-convergence-operations.md) - Convergence, control flow, and system interaction (probe, guard, choose, gather, retry, rollback, elevate)
- [LLM Integration](devlore-llm-integration.md) - Provider abstraction and prompt loading for AI-assisted commands
- [Registry Knowledge Base](devlore-registry-knowledge.md) - Knowledge domain structure in devlore-registry
- [E2E Testing](devlore-e2e-testing.md) - Multi-provider testing strategy for LLM commands
- [Action Namespaces](devlore-operation-namespaces.md) - How to add new action namespaces to the execution engine
- [Receipt Integrity](devlore-receipt-integrity.md) - Checksum and signature verification for receipts
- [Phase Execution](devlore-phase-execution.md) - Saga pattern, phases, retry/rollback
- [Typed Slots](devlore-typed-slots.md) - Slot model and resolution chain
- [Orchestration Primitives](devlore-orchestration-primitives.md) - Gather, Choose, WaitUntil, SlotProxy, ActivationState, Sidecar hooks

## Overview

The devlore-cli consists of two main tools that share a common execution engine:

- **writ** - File tree management (dotfiles, configs, templates)
- **lore** - Package management (software installation, configuration)

Both tools produce execution graphs that are processed by the shared engine in `internal/execution/`.
