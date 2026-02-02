# Architecture Documentation

Internal engineering documentation for devlore-cli developers.

## Documents

- [Execution Graph](execution-graph.md) - Unified graph design for all lifecycle commands
- [LLM Integration](llm-integration.md) - Provider abstraction and prompt loading for AI-assisted commands
- [Registry Knowledge Base](registry-knowledge.md) - Knowledge domain structure in devlore-registry
- [E2E Testing](e2e-testing.md) - Multi-provider testing strategy for LLM commands
- [Operation Namespaces](operation-namespaces.md) - How to add new operation namespaces to the execution engine
- [Receipt Integrity](receipt-integrity.md) - Checksum and signature verification for receipts

## Overview

The devlore-cli consists of two main tools that share a common execution engine:

- **writ** - File tree management (dotfiles, configs, templates)
- **lore** - Package management (software installation, configuration)

Both tools produce execution graphs that are processed by the shared engine in `internal/engine/`.
