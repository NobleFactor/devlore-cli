# Architecture Documentation

Internal engineering documentation for devlore-cli developers.

Each architecture document has a companion `*.status.md` file tracking completion, PR links, document discrepancies, and outstanding work.

## Documents

### 1. System Model

- [System Model](1-system-model.md) ([status](1-system-model.status.md)) — Hosts, deployments, dependency taxonomy, receipt graph as system model, distributed orchestration vision

### Configuration

- [Configuration](configuration.md) ([status](configuration.status.md)) — Distributed config participation: `devconfig.{Config, Section, Setting}`, schema announcement and the section registry, the two-axis roll-up, owner-located sections, and prior art (star / OpenTelemetry / Kubernetes)

### 2. Execution Graph

- [Execution Graph](2-execution-graph.md) ([status](2-execution-graph.status.md)) — The sealed `op.Graph` model: the Node/Subgraph unit tree, spec-based construction, the Binding set, the GraphExecutor contract, and the graph-vs-trace document split
  - [Typed Slots](2.1-typed-slots.md) ([status](2.1-typed-slots.status.md)) — The sealed Binding set, the variable surface, slot typing from method signatures, and the one Go↔Starlark conversion machine (op.Convert, the Converter, the Invoker)
  - [Phase Execution](2.2-phase-execution.md) ([status](2.2-phase-execution.status.md)) — The saga model on the unit tree: the receipt contract, recovery-stack tree of compensators, failure adjudication, and the RunStatus state machine
  - [Orchestration Primitives](2.3-orchestration-primitives.md) ([status](2.3-orchestration-primitives.status.md)) — The one recursive rule (a subgraph is an executor); the flow combinators (Subgraph, Choose, Gather, WaitUntil), the terminals (Complete, Degraded, Failed), and the lifecycle-hook seam
  - [Hermeticity Guarantees](2.4-hermeticity-guarantees.md) ([status](2.4-hermeticity-guarantees.status.md)) — Git-native input snapshots, scope separation (System/Home graphs), layer hermeticity
  - [Lifecycle Pipeline Construction](2.5-lifecycle-pipeline-construction.md) ([status](2.5-lifecycle-pipeline-construction.status.md)) — How commands build per-OS lifecycle-pipeline graphs from packages: phase-script→subgraph harvest, Origin/provenance, build/save/load/run portability
  - [Execution Policies: Retry, Elevation, and Error Handling](2.6-execution-policies.md) ([status](2.6-execution-policies.status.md)) — The per-unit policy triplet, its floor-plus-inherit resolution at plan time, the maturity gradient (retry live / error-action observation-only / elevation dormant), and elevation's requirement/offer/provision/realization model
  - [Control Plane](2.7-control-plane.md) ([status](2.7-control-plane.status.md)) — The executor's async command / event channel: `Request`/response + event push, the HTTP/2 (h2c) wire surface, curl examples. The command surface landed; the event-stream / narration integration is step 50
  - [Eventing Infrastructure](2.8-eventing-infrastructure.md) ([status](2.8-eventing-infrastructure.status.md)) — The app-agnostic event bus and the hook interface that feeds it; the three output streams (narration / operational events / diagnostics) and their OpenTelemetry mapping (proposed)

### 3. Providers

- [Action Namespaces](3-operation-namespaces.md) ([status](3-operation-namespaces.status.md)) — How to add a namespace: the hand-written provider contract, `make generate` (announcement + action-name constants + inventory), tests, and the catalog/design-doc step
  - [Provider Loading and Lifetime](3.1-provider-loading.md) ([status](3.1-provider-loading.status.md)) — Module loading via `@devlore//`, provider lifetime model
  - [Projected Provider API](3.2-projected-provider-api.md) ([status](3.2-projected-provider-api.status.md)) — The runtime projection: goReceiver + announced metadata, the promise model over detached invocations, member-projection rules, and the provider-owned broker pattern
  - [Static Starlark Value Codegen](3.3-static-starlark-codegen.md) — **Archived** (2026-07-22): the static-codegen draft, superseded by the landed reflection bridge; retained as history
  - [Platform and Package Management](3.4-platform-package-managers.md) ([status](3.4-platform-package-managers.status.md)) — `op.Platform`, the Composite `op.PackageManager` router, purl routing, the driver catalog, and state-query verification (design)
  - [Provider Catalog](3.5-provider-catalog.md) ([status](3.5-provider-catalog.status.md)) — index of record for every provider: namespace, access role zone, and per-provider design-doc links
  - [Method Classification — Pure, Query, Action](3.6-method-classification.md) ([status](3.6-method-classification.status.md)) — replaces `+devlore:access` with a per-method kind from which namespace placement and hermeticity both derive; the action tier infers itself from a `*Receipt` return, `pure` is annotated and verified, `query` is the fail-closed default
    - [Archive Provider](3.5.1-archive-provider.md) ([status](3.5.1-archive-provider.status.md)) — content-based format detection (magic-byte sniff over extension), the decompressor → container pipeline (tar.gz / tar.bz2 / tar.xz / tar.zst / tar + zip), and extraction as unified file-mutation receipts with reverse-unwind compensation
    - [Flow Provider](3.5.2-flow-provider.md) ([status](3.5.2-flow-provider.status.md)) — the root provider's orchestration combinators: Subgraph the base case, Choose/Gather/WaitUntil quantifiers over it, on the per-subgraph-executor + stamped-recovery-stack core (Subgraph + Gather implemented; Choose next)
    - [Plan Provider](3.5.3-plan-provider.md) ([status](3.5.3-plan-provider.status.md)) — the planning front door: the three-tier attribute surface (adapters / promoted / own), detached invocations + the dual Go/Starlark door, assembly into the sealed graph, and the save/load/run lifecycle
    - [File Provider](3.5.4-file-provider.md) ([status](3.5.4-file-provider.status.md)) — the filesystem authority: the sealed intent-declared taxonomy, the unified mutation receipt (one receipt, one compensating action, five kinds), and the conflict-policy write seam
    - [JSON Provider](3.5.5-json-provider.md) ([status](3.5.5-json-provider.status.md)) — encode/decode transforms + the content-addressed `json.Resource` (canonical-form SHA-256, content-keyed interning)
    - [YAML Provider](3.5.6-yaml-provider.md) ([status](3.5.6-yaml-provider.status.md)) — the json sibling: YAML parsed then canonicalized via JSON, so semantically equal documents hash identically across formats
    - [Template Provider](3.5.7-template-provider.md) ([status](3.5.7-template-provider.status.md)) — pure Go text/template expansion; dispatch-time `Env` resolution keeps environmental state out of persisted graphs
    - [Shell Provider](3.5.8-shell-provider.md) ([status](3.5.8-shell-provider.status.md)) — POSIX `sh -c` execution: the deliberately non-compensable escape hatch, one `Result` emission shape, context-bound cancellation
    - [PowerShell Provider](3.5.9-powershell-provider.md) ([status](3.5.9-powershell-provider.status.md)) — PowerShell 7+ via hard-required `pwsh` (no `powershell.exe` fallback); the shell contract otherwise
    - [Git Provider](3.5.10-git-provider.md) ([status](3.5.10-git-provider.status.md)) — repositories as resources: compensable clone, fallible checkout/pull, the observe fact, and dirty-aware Digest/Etag content identity
    - [Service Provider](3.5.11-service-provider.md) ([status](3.5.11-service-provider.status.md)) — five compensable pairs over `Platform.ServiceManager` with prior-state receipts; `CompensateRestart` a no-op by design
    - [Appnet Provider](3.5.12-appnet-provider.md) ([status](3.5.12-appnet-provider.status.md)) — network download; URL-addressed resources whose identity is the address, deliberately not the fetched content
    - [Encryption Provider](3.5.13-encryption-provider.md) ([status](3.5.13-encryption-provider.status.md)) — SOPS-backed compensable pairs over the file taxonomy; ciphersuite custody in pkg/sops
    - [Function Provider](3.5.14-function-provider.md) ([status](3.5.14-function-provider.status.md)) — Starlark functions as content-addressed resources: synthesized self-contained source is identity, the pack carries bytecode as cache, `function.call` evaluates at dispatch
    - [Regexp Provider](3.5.15-regexp-provider.md) ([status](3.5.15-regexp-provider.status.md)) — eight pure regular-expression operations with compiled-pattern caching
    - [UI Provider](3.5.16-ui-provider.md) ([status](3.5.16-ui-provider.status.md)) — the narration passthrough: six thin methods onto the bootstrap-installed `status.Narrator`

### 4. Resources

- [Resource Management](4-resource-management.md) ([status](4-resource-management.status.md)) — URI-based resource tracking: the catalog, the ResourceState machine and behavior matrix, shadowing, observations-as-results, receipts + the recovery site
  - [Resource Identity](4.1-resource-identity.md) ([status](4.1-resource-identity.status.md)) — URI schemes, opaque vs hierarchical, interface simplification
  - [Memory Resources](4.2-mem-resource.md) ([status](4.2-mem-resource.status.md)) — `mem:` scheme, callable serialization lifecycle
  - [Resource Registration](4.3-resource-registration.md) ([status](4.3-resource-registration.status.md)) — Generated announcements into the receiver registry, environment-aware constructors, source-type declarations, rehydration by type id
  - [Root-Path Triad](4.4-root-path-triad.md) ([status](4.4-root-path-triad.status.md)) — Root interface, Path struct, RecoverySite, OS-enforced I/O confinement
  - [fsroot — One Confinement, Two Types](4.5-fsroot-variants.md) ([status](4.5-fsroot-variants.status.md)) — one confinement behaviour, the kernel's; `confinedDir` and `confinedScratchDir`; what fsroot adds to `os.Root` (Windows permission enforcement, the two-form `Path`, confined temp creation); open is a query and a missing root is an error; symlink targets left to the kernel; supersedes 4.4 §4

### 5. Operational Integrity

- [Graph and Trace Integrity](5-graph-trace-integrity.md) ([status](5-graph-trace-integrity.status.md)) — Checksum and signature verification for graphs and traces
  - [Reconciliation](5.1-reconciliation.md) ([status](5.1-reconciliation.status.md)) — Audit (the trace), drift detection (`writ status` over recorded content identity), and the landed recovery-safety gates; the ExecutionEvent/Reconcile proposal preserved as unimplemented design
  - [Recovery Serialization](5.2-recovery-serialization.md) ([status](5.2-recovery-serialization.status.md)) — Recovery stack serialization and restart (planned)
  - [Recovery Site](5.3-recovery-site.md) ([status](5.3-recovery-site.status.md)) — Shared recovery service for file archival (zero-copy rename) and data archival (byte serialization)

### 6. Execution Topology

- [Execution Topology](6-execution-topology.md) ([status](6-execution-topology.status.md)) — Elevation, remote execution, telemetry (planned)
  - [Privilege Elevation: The Elevator Provider](6.1-privilege-elevation.md) ([status](6.1-privilege-elevation.status.md)) — The elevator provider: graph/config/runtime split, the two strategies (ProcessSpawn / IdentityAssumption), the token-provider mechanism, the config outline, and failure routing
  - [The Process Pool](6.2-process-pool.md) ([status](6.2-process-pool.status.md)) — Lazily-allocated execution hosts keyed by interpreter × privilege × pooling mode × session; persistent privilege vs persistent interpreter state; multiplexing with request ids; PgBouncer pooling modes; the PowerShell runspace host and why `pwsh` is not the universal one
  - [Command Execution — Two Tiers](6.3-command-execution.md) ([status](6.3-command-execution.status.md)) — A command is an argv vector and a script is a file an argv vector runs; no command string handed to `-c`; tier 2 keeps script bodies static with values as `$1`/`$2`, written to the scratch root; why quoting dissolves rather than centralizes

### 7. Knowledge and LLM

- [Registry Knowledge Base](7-registry-knowledge.md) ([status](7-registry-knowledge.status.md)) — Knowledge domain structure in devlore-registry
  - [LLM Integration](7.1-llm-integration.md) ([status](7.1-llm-integration.status.md)) — Provider abstraction and prompt loading for AI-assisted commands
  - [E2E Testing](7.2-e2e-testing.md) ([status](7.2-e2e-testing.status.md)) — Multi-provider testing strategy for LLM commands

### 8. Rust Migration

- [Rust Migration](8-rust-migration.md) — **Historical** (2026-07-22): the port rationale argued from the pre-`op` system; any real Rust effort re-grounds against the landed architecture

### 9. Star

- [Star Extensions](star-extensions.md) — Extension model, discovery, loading, runtime types (`star.Extension`, `star.Command`)
  - Star Configuration — Config hierarchy, schema, on-demand resolution (planned)
  - Star WASM Receivers — WASM sandboxing, capabilities, host module (planned)

## Overview

The devlore-cli consists of three tools that share a common execution engine:

- **writ** — File tree management (dotfiles, configs, templates)
- **lore** — Package management (software installation, configuration)
- **star** — Starlark-powered operations tool (linting, setup, hooks, project automation)

All three tools consume the shared provider framework in `pkg/op/` and the starlark runtime. Writ and lore produce sealed execution graphs run by `op.GraphExecutor` in `pkg/op/`. Star uses a lightweight command model where extensions define commands in Starlark and providers supply the bindings.
