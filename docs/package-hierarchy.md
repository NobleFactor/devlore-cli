# Package Hierarchy

Public elements per package, verified against code.

---

## internal/cli

CLI infrastructure shared by writ and lore.

### PUBLIC

**Constants:**
- `ExitOK` (0), `ExitError` (1), `ExitUsage` (64), `ExitDataErr` (65), `ExitNoInput` (66), `ExitUnavailable` (69), `ExitSoftware` (70), `ExitCantCreate` (73), `ExitIOErr` (74), `ExitNoPerm` (77)

**Types:**
- `ConfigInfo` - Configuration metadata for a tool
- `ManHeader` - Metadata for man page generation
- `SelfInstallInfo` - Metadata for self-installation
- `VersionInfo` - Version metadata (Version, Commit, BuildDate)
- `ViperConfig` - Configuration for Viper initialization

**Functions (output):**
- `Note(format, args...)` - Print informational message
- `Warn(format, args...)` - Print warning message
- `Error(format, args...)` - Print error message (non-fatal)
- `Failure(format, args...)` - Print error and return error
- `Success(format, args...)` - Print success message
- `ExitWith(code, err)` - Return error with specific exit code
- `ExitCode(err)` - Extract exit code from error
- `SetProgramName(name)` - Set program name for output prefixes
- `SetSilent(s)` - Enable/disable silent mode
- `AddSilentFlag(cmd)` - Add --silent flag to root command

**Functions (config):**
- `InitViper(cfg)` - Initialize Viper with devlore conventions
- `BindPFlags(cmd)` - Bind persistent flags to Viper
- `GetString(toolName, key, useShared)` - Retrieve string config value
- `SharedConfigPath()` - Return shared devlore config file path

**Functions (XDG):**
- `ConfigHome()`, `DataHome()`, `CacheHome()`, `StateHome()` - XDG base directories
- `DevloreConfigHome()`, `DevloreCacheHome()`, `DevloreStateHome()`, `DevloreDataHome()` - Devlore directories
- `ManPath()` - Return user man page directory
- `WritLayersDir()` - Return writ layers directory

**Functions (receipts):**
- `ReceiptsDir()` - Return receipts storage directory
- `LatestReceiptPath(producer)` - Return path to latest receipt symlink
- `LoadReceipt(path)` - Load execution graph from YAML receipt file
- `LoadLatestReceipt(producer)` - Load most recent receipt for producer
- `WriteReceipt(g, producer)` - Write graph as receipt with checksum
- `WriteReceiptWithSigningDir(g, producer, signingDir)` - Write receipt with signing

**Functions (commands):**
- `NewConfigCmd(info)` - Create config command with subcommands
- `NewHelpCmd(rootCmd, header)` - Create help command preferring man pages
- `NewManCmd(rootCmd, header)` - Create man command
- `NewSelfInstallCmd(rootCmd, info)` - Create self-install command
- `NewVersionCmd(info)` - Create version command
- `DisplayManPage(cmd, header)` - Generate and display man page

---

## internal/execution

Core execution graph: typed actions, saga-pattern phases, and the graph executor.

### PUBLIC

**Core Types:**
- `Action` (interface) - Name(), Do(ctx, slots) (Result, UndoState, error), Undo(ctx, slots, state) error
- `Result` (alias `any`) - Data flowing to downstream nodes via edges
- `UndoState` (alias `any`) - State captured by Do, passed to Undo during rollback
- `Context` - Execution context: DryRun, Logger, Data, Graph, NodeID, checksums

**Graph Types:**
- `Graph` - Execution graph: nodes, edges, phases, summary, rollback, checksum, signature
- `Node` - Unit of work: ID, Action, Status, Slots, checksums, annotations
- `Edge` - Dependency: From, To
- `SlotValue` - Node input: Immediate value or promise (NodeRef, Slot, GatherRef, Field)
- `GraphState` - pending, executed, failed
- `NodeStatus` - pending, completed, skipped, failed
- `Platform` - OS, Arch
- `GraphContext` - SourceRoot, TargetRoot, Projects, Packages, Segments, etc.
- `Summary` - Execution statistics
- `Collision` - Source conflict record
- `Signature` - Cryptographic signature (Method, Value, KeyID)

**Phase Types (Saga Pattern):**
- `Phase` - Lifecycle phase: ID, Name, Status, Retry, NodeIDs, Compensate, Attempts, State
- `PhaseStatus` - pending, completed, failed, rolled_back, skipped
- `RetryPolicy` - MaxAttempts, Backoff, InitialDelay, MaxDelay
- `BackoffStrategy` - none, linear, exponential
- `Attempt` - Number, Status, Error, Timestamp
- `RollbackEntry` - Phase-level rollback record (serialized to receipts)

**Recovery Types:**
- `RecoveryStack` - LIFO stack of completed node entries for saga unwind
- `RecoveryEntry` - Node + UndoState pair (in-memory, not yet serialized)

**Flow Support Types:**
- `ChooseUndoState` - Branch recovery state (Results, Entries)
- `ChooseCase` - Predicate + PhaseID pair
- `GatherUndoState` - Per-iteration recovery state (Iterations)
- `IterationUndo` - ProxyCtx, Results, Entries
- `Predicate` (interface) - Eval(input) (bool, error), String()

**Executor Types:**
- `GraphExecutor` - Executes graphs (flat or phased)
- `ExecutorOptions` - DryRun, Logger, Data, ConflictPolicy, BackupSuffix
- `ActionRegistry` - Maps action names to implementations
- `NodeResult` - Per-node execution outcome
- `ResultStatus` - pending, running, completed, failed, skipped
- `ConflictPolicy` - stop, backup, overwrite, skip

**Lifecycle Hooks:**
- `LifecycleHook` (interface) - OnNodeStart, OnNodeComplete, OnPhaseStart, OnPhaseComplete
- `HookRegistry` - Manages lifecycle hooks

**Plan Builder:**
- `Plan` - Builder for constructing execution graphs with action registry

**Constants:**
- `StatePending`, `StateExecuted`, `StateFailed`
- `StatusPending`, `StatusCompleted`, `StatusSkipped`, `StatusFailed`
- `PhasePending`, `PhaseCompleted`, `PhaseFailed`, `PhaseRolledBack`, `PhaseSkipped`
- `BackoffNone`, `BackoffLinear`, `BackoffExponential`
- `ConflictStop`, `ConflictBackup`, `ConflictOverwrite`, `ConflictSkip`

**Functions:**
- `NewGraphExecutor(opts)` - Create executor
- `NewActionRegistry()` - Create action registry
- `NewHookRegistry()` - Create hook registry
- `NewPlan(reg, project)` - Create plan builder
- `StubAction(name)` - Create stub action for receipt deserialization
- `OrderNodes(nodes, edges)` - Topological sort
- `FillSlotsFromData(slots, data)` - Populate slots from context data
- `ChecksumBytes(content)`, `ChecksumFile(path)`, `GitStyleChecksum(type, basename, content)` - Integrity

---

## internal/execution/flow

Flow actions for control flow within the execution graph.

### PUBLIC

**Types (all implement Action):**
- `Choose` - Predicate-driven branch selector (OR). Evaluates cases, executes first matching phase.
- `Gather` - Fan-in AND. Iterates items, executes body phase per item, collects results.
- `Elevate` - Privilege transition. Marks a privilege boundary in the graph.
- `WaitUntil` - Temporal gate. Waits for a duration or condition.

---

## internal/execution/provider

Resource providers with saga-pattern compensation. Each provider exposes forward methods (returning receipt state) and Compensate methods (reading state to undo).

### Provider Packages

**file/** - Filesystem operations
- Link, Copy, Write, Remove, Move, Unlink, Mkdir, Backup
- Each has a Compensate counterpart

**pkg/** - Package manager operations
- Install, Upgrade, Remove
- Each has a Compensate counterpart

**service/** - Service manager operations
- Start, Stop, Restart, Enable, Disable
- Each has a Compensate counterpart

**template/** - Template rendering
- Render, CompensateRender

**encryption/** - SOPS decryption
- Decrypt, CompensateDecrypt

**net/** - Network operations
- Download, CompensateDownload

**archive/** - Archive operations
- Extract, CompensateExtract

**git/** - Git operations
- Clone, Checkout, Pull
- Each has a Compensate counterpart

**shell/** - Shell command execution
- Execute, CompensateExecute (returns nil — shell commands are not compensable)

**content/** - Content pipeline
- Literal

---

## internal/writ

Writ CLI commands for configuration deployment.

### PUBLIC

**Types:**
- `Config` - Base settings for lifecycle operations
- `DeployConfig`, `DecommissionConfig`, `UpgradeConfig`, `ReconcileConfig`, `AdoptConfig` - Per-command config
- `DeployGraphBuilder`, `UpgradeGraphBuilder`, `ReconcileGraphBuilder`, `AdoptGraphBuilder`, `MigrateGraphBuilder` - Graph builders
- `TargetSpec` - Source directory and deployment target
- `VerifyResult` - Signature verification outcome

**Functions:**
- `NewRootCmd()` - Create root writ command
- `NewGraph(cfg)` - Create execution graph with writ defaults
- `BuildTree(g, cfg)` - Walk source directories, populate graph nodes
- `ConfigureEngine(cfg)` - Create and configure execution engine
- `CollectLayerSources()` - Gather configured repository layers
- `VerifyGraphSignature(g, identities)` - Verify graph signature
- Builder constructors: `NewDeployGraphBuilder`, `NewUpgradeGraphBuilder`, etc.

---

## internal/writ/tree

File tree building with layer/segment processing.

### PUBLIC

**Types:**
- `BuildConfig` - Configuration for tree building
- `BuildResult` - Result of tree building (Files, Collisions)
- `FileEntry` - Single file with operations
- `Collision` - Source conflict record
- `LayerSource` - Layer configuration
- `Operation` - File operation enum (OpLink, OpCopy, OpExpand, OpDecrypt, etc.)
- `Operations` - Slice of operations with methods

**Functions:**
- `Build(cfg)` - Build file tree from configuration
- `ProcessingPipeline(filename)` - Determine operations from filename

---

## internal/writ/segment

Platform segment detection and matching.

### PUBLIC

**Types:**
- `Segment`, `Segments`, `Matcher`

**Functions:**
- `DetectSegments()` - Detect platform segments (OS, ARCH, DISTRO, etc.)

---

## internal/writ/reconcile

Drift detection and repair.

### PUBLIC

**Constants:**
- `StateLinked`, `StateConflict`, `StateMissing`, `StateOrphan`, `StateCopied`, `StateStale`, `StateModified`, `StateDriftConflict`

**Types:**
- `State` - Status enum
- `Entry` - Status of a single file
- `Report` - Full status report

**Functions:**
- `FromBuildResult(br)` - Generate status from build result
- `ScanTarget(targetRoot, sourceRoot)` - Scan target for writ-managed symlinks

---

## internal/writ/deploystate

Deployment state persistence with signing.

### PUBLIC

**Types:**
- `State` - Deployment state (SourceRoot, TargetRoot, Files map, Signature)
- `FileEntry` - Single file in state
- `Signature` - State signature

**Functions:**
- `Load()` - Load state from disk
- `StatePath()` - Return state file path

---

## internal/lore

Lore CLI commands for package management.

### PUBLIC

**Functions:**
- `NewRootCmd()` - Create root lore command

---

## internal/lorepackage

Package resolution and lifecycle management.

### PUBLIC

**Constants:**
- `SourceLore`, `SourceApt`, `SourceDnf`, `SourceBrew`, `SourcePort`, `SourceWinget`
- `Deploy`, `Upgrade`, `Decommission`, `Reconcile`
- `PMInstall`, `PMUpgrade`, `PMRemove`

**Types:**
- `PackageSource`, `Release`, `Action`, `Lifecycle`, `PhaseAction`, `ScriptAction`, `NativePMAction`, `Registry`, `PMCommand`

**Functions:**
- `Resolve(name, opts)` - Resolve package to release
- `VerifySyntheticPackage(path)` - Verify synthetic package structure
- `DiscoverPhaseScripts(dir)` - Find phase scripts in directory
- `ParsePackagePrefix(spec)` - Parse package manager prefix
- `IsNative(source)`, `IsSynthetic(source)` - Source classification

---

## internal/manifest

Package manifest parsing and validation.

### PUBLIC

**Types:**
- `PackagesManifest` - Parsed packages-manifest.yaml
- `PackageEntry` - Single package entry (Name, With)

**Functions:**
- `Load(path)`, `Parse(data)`, `Validate(manifest)`, `ValidateBytes(data)`, `IsManifestFile(filename)`

---

## internal/starlark

Starlark runtime and receiver bindings.

### PUBLIC

**Receiver Types:**
- `ArchiveReceiver` - archive.extract, archive.list
- `DockerReceiver` - docker operations
- `EnvReceiver` - env.get, env.set, env.expand
- `GitReceiver` - git.clone, git.pull, git.push, etc.
- `HTTPReceiver` - http.get, http.post, http.put, http.delete, http.download
- `LogReceiver` - log.info, log.warn, log.error, log.debug
- `NpmReceiver` - npm.install, npm.update, npm.remove, etc.
- `PackageReceiver` - package.manager, package.installed, package.install, etc.
- `ServiceReceiver` - service.start, service.stop, service.restart, etc.
- `ShellReceiver` - shell.run, shell.which

**Plan Types:**
- `PlanRoot` - Namespace: file, package, service, shell, template, encryption, archive, git, net, content
- `FilePlan`, `PackagePlan`, `ServicePlan`, `ShellPlan`, `TemplatePlan`, `EncryptionPlan`, `ArchivePlan`, `GitPlan`, `NetPlan`, `ContentPlan` - Plan builders

**Package Context:**
- `PackageContext` - Name, Version, Features, Settings, DryRun, SourceRoot, TargetRoot

---

## Other Packages

### internal/host
Platform detection and package/service management (Host, Platform, PackageManager, ServiceManager interfaces).

### internal/console
TUI console for interactive operations (Console, Model, Styles).

### internal/credentials
Credential storage (Get, Set, Delete).

### internal/model
AI model configuration (Config, Provider, LoadConfig, SelectModel).

### internal/tools/docgen
Documentation generation (PageData, GenerateTree, BuildPageData).
