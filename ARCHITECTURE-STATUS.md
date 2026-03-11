# Architecture Status

Status overview for all architecture documents in `docs/architecture/`. Each document has a companion `*.status.md` file with completion details, PR links, and outstanding work.

## Fully Complete

| Document | Notes |
|----------|-------|
| [2.1 Typed Slots](docs/architecture/2.1-typed-slots.md) | No discrepancies |
| [4.1 Resource Identity](docs/architecture/4.1-resource-identity.md) | Minor note about rename comment |
| [4.2 Memory Resources](docs/architecture/4.2-mem-resource.md) | Minor doc clarity notes |
| [4.3 Resource Registration](docs/architecture/4.3-resource-registration.md) | Minor oversimplification in callable Init() example |
| [4.4 Root-Path Triad](docs/architecture/4.4-root-path-triad.md) | Minor notation difference (method vs field) |
| [5 Receipt Integrity](docs/architecture/5-receipt-integrity.md) | No discrepancies |
| [5.3 Recovery Site](docs/architecture/5.3-recovery-site.md) | No discrepancies |
| [7.1 LLM Integration](docs/architecture/7.1-llm-integration.md) | Minor: GitHub Models impl detail |
| [7.2 E2E Testing](docs/architecture/7.2-e2e-testing.md) | No discrepancies |

## Complete — Docs Need Code Example Updates

| Document | What needs updating |
|----------|---------------------|
| [1 System Model](docs/architecture/1-system-model.md) | Section 12 paths/counts (`internal/execution/` → `pkg/op/`), Sidecar cross-ref |
| [2 Execution Graph](docs/architecture/2-execution-graph.md) | Type names (`ExecutionGraph` → `Graph`), package paths, method signatures |
| [2.2 Phase Execution](docs/architecture/2.2-phase-execution.md) | Provider signatures (string → Resource/Tombstone), generated file paths |
| [3 Operation Namespaces](docs/architecture/3-operation-namespaces.md) | Steps 1/2/4 paths (`internal/execution/provider/` → `pkg/op/provider/`) |
| [3.1 Provider Loading](docs/architecture/3.1-provider-loading.md) | `With()` API example → `cfg.Receivers` |
| [3.2 Projected Provider API](docs/architecture/3.2-projected-provider-api.md) | Projection Layer section: `pkg/projection/` → `pkg/op/`, access directives |
| [4 Resource Management](docs/architecture/4-resource-management.md) | 13 skipped tests (macOS SIP), `pkg.Resource.Resolve()` skeleton |
| [7 Registry Knowledge](docs/architecture/7-registry-knowledge.md) | Missing knowledge domains in doc |

## Partially Complete

| Document | Complete | Not implemented |
|----------|----------|-----------------|
| [2.3 Orchestration Primitives](docs/architecture/2.3-orchestration-primitives.md) | 7/8 primitives | Elevate (stub only) |
| [2.4 Hermeticity Guarantees](docs/architecture/2.4-hermeticity-guarantees.md) | Phases 1–7 | Phase 8: E2E validation |
| [5.1 Reconciliation](docs/architecture/5.1-reconciliation.md) | RecoveryStack, drift detection | 4-value Do(), ReconcilableAction, AuditLedger, reconciliation engine |

## Future Design (No Implementation)

| Document | Status |
|----------|--------|
| [5.2 Recovery Serialization](docs/architecture/5.2-recovery-serialization.md) | Design topic for a future plan |
| [6 Execution Topology](docs/architecture/6-execution-topology.md) | Design topic for a future plan — Elevate stub only |
