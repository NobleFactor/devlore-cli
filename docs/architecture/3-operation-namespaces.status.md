# Status: Operation Namespaces

**Architecture document:** [3-operation-namespaces.md](3-operation-namespaces.md)

This document has been **substantially updated**. The namespace tables (lines 44–77) are current and accurate — they list the correct providers, actions, access levels, and packages. The "Removed" section correctly notes `content` was removed. The Provider Method Contracts and Naming Conventions sections are accurate. The checklist at the bottom is correct.

## Completion

| Component | Status | Completed | PR |
|-----------|--------|-----------|-----|
| Namespace system | Complete | 2026-02-14 | [#128](https://github.com/NobleFactor/devlore-cli/pull/128)–[#130](https://github.com/NobleFactor/devlore-cli/pull/130) |
| Provider extraction to subpackages | Complete | 2026-02-14 | [#129](https://github.com/NobleFactor/devlore-cli/pull/129) |
| Action registration | Complete | 2026-03-06 | [#190](https://github.com/NobleFactor/devlore-cli/pull/190) |
| 20+ provider namespaces | Complete | 2026-03-07 | [#197](https://github.com/NobleFactor/devlore-cli/pull/197)–[#203](https://github.com/NobleFactor/devlore-cli/pull/203) |
| Announce-and-callback model | Complete | 2026-03-06 | [#190](https://github.com/NobleFactor/devlore-cli/pull/190) |

## Document Discrepancies

The namespace tables, provider method contracts, naming conventions, and checklist are all correct. The issues are limited to **example code paths** in the "Adding a New Namespace" walkthrough:

- **Step 1 (line 163)**: Path says `internal/execution/provider/docker/` — should be `pkg/op/provider/docker/`
- **Step 2 (line 194)**: Shows `internal/execution` imports and old action pattern — should use `pkg/op` imports and `RegisterReflectedActions` pattern
- **Step 4 (line 305)**: Path says `internal/starlark/plan_docker_gen.go` — should be `pkg/op/provider/docker/gen/planned.gen.go`
- **Testing section (line 418)**: Uses old `execution.` types — should use `op.` types
- **Internal contradiction**: Step 1 says `internal/execution/provider/` but the checklist (line 463) correctly says `pkg/op/provider/`

## Outstanding Work

1. **Update paths in Steps 1, 2, 4 and Testing** to use `pkg/op/provider/` and `op.` types
2. **Resolve internal contradiction** between Step 1 path and checklist path
