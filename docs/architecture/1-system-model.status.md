# Status: System Model

**Architecture document:** [1-system-model.md](1-system-model.md)

This document is a **design thesis** — it describes conceptual architecture and future vision. Sections 1–11 are architectural intent, not code specifications. Only Section 12 makes claims about the current codebase.

## Completion

| Component | Status | Completed | PR |
|-----------|--------|-----------|-----|
| Core execution engine | Complete | 2025-12-01 | [#10](https://github.com/NobleFactor/devlore-cli/pull/10), [#43](https://github.com/NobleFactor/devlore-cli/pull/43) |
| Provider system (Operation→Action) | Complete | 2026-02-16 | [#128](https://github.com/NobleFactor/devlore-cli/pull/128)–[#137](https://github.com/NobleFactor/devlore-cli/pull/137) |
| Compensation (saga pattern) | Complete | 2026-02-17 | [#141](https://github.com/NobleFactor/devlore-cli/pull/141)–[#146](https://github.com/NobleFactor/devlore-cli/pull/146) |
| Orchestration primitives | Complete | 2026-02-16 | [#139](https://github.com/NobleFactor/devlore-cli/pull/139) |
| Resource management | Complete | 2026-03-06 | [#176](https://github.com/NobleFactor/devlore-cli/pull/176)–[#187](https://github.com/NobleFactor/devlore-cli/pull/187) |
| Provider registration | Complete | 2026-03-06 | [#190](https://github.com/NobleFactor/devlore-cli/pull/190) |
| Hermeticity guarantees | Complete | 2026-03-10 | [#207](https://github.com/NobleFactor/devlore-cli/pull/207) |
| Package planning (Section 5) | Not implemented | — | — |
| Distributed orchestration (Sections 6.2–6.4) | Not implemented | — | — |
| Global receipt graph (Section 7) | Not implemented | — | — |

## Document Discrepancies

Sections 1–11 are conceptual architecture / future vision and do not require code-level accuracy. The discrepancies below are limited to **Section 12 (Implementation Status)** and one cross-reference:

- **Line 14–15**: Cross-reference says "Sidecar" — actual primitives are Gather, Choose, WaitUntil, Complete, Degraded, Fatal, Elevate
- **Section 12, provider path**: Says `internal/execution/provider/` — actual is `pkg/op/provider/`
- **Section 12, provider count**: Says 10 — actual is 20+
- **Section 12, generated files**: Says `**/actions_gen.go` — actual pattern is `provider.gen.go`, `params.gen.go`, `planned.gen.go`, `immediate.gen.go` in `pkg/op/provider/*/gen/`
- **Section 12, orchestration status**: Says "Designed" — actually implemented ([#139](https://github.com/NobleFactor/devlore-cli/pull/139), [#194](https://github.com/NobleFactor/devlore-cli/pull/194))
- **Section 12, location column**: Uses `internal/execution/` throughout — most types now in `pkg/op/`

## Outstanding Work

1. **Update Section 12** — fix paths, counts, generated file names, and statuses to match current codebase
2. **Fix Sidecar cross-reference** (line 14–15)
3. **Package planning** (Section 5) — not implemented; design vision only
4. **Distributed orchestration** (Sections 6.2–6.4) — not implemented; design vision only
5. **Global receipt graph** (Section 7) — not implemented; design vision only
