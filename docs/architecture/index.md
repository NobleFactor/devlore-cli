# Architecture Documentation

Internal engineering documentation for devlore-cli developers.

## Documents

- [Operation Namespaces](operation-namespaces.md) - How to add new operation namespaces to the execution engine

## Overview

The devlore-cli consists of two main tools that share a common execution engine:

- **writ** - File tree management (dotfiles, configs, templates)
- **lore** - Package management (software installation, configuration)

Both tools produce execution graphs that are processed by the shared engine in `internal/engine/`.
