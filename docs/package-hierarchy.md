# Package Hierarchy

Complete list of public and private elements per package, verified against code.

---

## internal/cli

CLI infrastructure shared by writ and lore.

### PUBLIC

**Constants:**
- `ExitOK` (0) - Success
- `ExitError` (1) - Generic error
- `ExitUsage` (64) - Bad CLI syntax
- `ExitDataErr` (65) - Invalid manifest/config
- `ExitNoInput` (66) - File not found
- `ExitUnavailable` (69) - Registry unreachable
- `ExitSoftware` (70) - Internal error (bug)
- `ExitCantCreate` (73) - Can't create file/symlink
- `ExitIOErr` (74) - Read/write failure
- `ExitNoPerm` (77) - Permission denied
- `DefaultFormat` - Default output format ("json")

**Variables:**
- `ErrManNotAvailable` - Error when man command unavailable

**Types:**
- `ConfigInfo` - Configuration metadata for a tool (Name, Schema, DefaultConfig)
- `ManHeader` - Metadata for man page generation (Title, Section, Source, Manual)
- `MutationFlags` - Flags for mutating commands (Passthru, Format)
- `OutputFlags` - Filter and format flag values
- `SelfInstallInfo` - Metadata for self-installation
- `VersionInfo` - Version metadata (Version, Commit, BuildDate)
- `ViperConfig` - Configuration for Viper initialization

**Functions:**
- `AddMutationFlags(cmd, flags)` - Add --passthru and --format flags
- `AddOutputFlags(cmd, flags)` - Add --filter and --format flags
- `AddSilentFlag(cmd)` - Add --silent flag to root command
- `AllSettings()` - Return all settings as map
- `BashCompletionPath()` - Return bash completion directory
- `BindFlags(cmd, toolName, useShared)` - Bind persistent flags to Viper
- `BindFlagsWithPrefix(flags, prefix)` - Bind flags with custom prefix
- `CacheHome()` - Return XDG_CACHE_HOME
- `ConfigFileUsed()` - Return loaded config file path
- `ConfigHome()` - Return XDG_CONFIG_HOME
- `DataHome()` - Return XDG_DATA_HOME
- `Debug()` - Print Viper state for debugging
- `DevloreCacheHome()` - Return unified devlore cache directory
- `DevloreConfigHome()` - Return unified devlore config directory
- `DevloreDataHome()` - Return unified devlore data directory
- `DevloreStateHome()` - Return unified devlore state directory
- `DisplayManPage(cmd, header)` - Generate and display man page
- `Error(format, args...)` - Print error message (non-fatal)
- `ExitCode(err)` - Extract exit code from error
- `ExitWith(code, err)` - Return error with specific exit code
- `Failure(format, args...)` - Print error and return error (fatal)
- `FishCompletionPath()` - Return fish completion directory
- `Get(toolName, key, useShared)` - Retrieve config value
- `GetBool(toolName, key, useShared)` - Retrieve boolean config value
- `GetInt(toolName, key, useShared)` - Retrieve integer config value
- `GetString(toolName, key, useShared)` - Retrieve string config value
- `GetStringMap(toolName, key, useShared)` - Retrieve string map config value
- `GetStringSlice(toolName, key, useShared)` - Retrieve string slice config value
- `InitViper(cfg)` - Initialize Viper with devlore conventions
- `LatestReceiptPath(producer)` - Return path to latest receipt symlink
- `LoadLatestReceipt(producer)` - Load most recent receipt for producer
- `LoadReceipt(path)` - Load execution graph from YAML receipt file
- `ManPath()` - Return user man page directory
- `NewConfigCmd(info)` - Create config command with subcommands
- `NewHelpCmd(rootCmd, header)` - Create help command preferring man pages
- `NewManCmd(rootCmd, header)` - Create man command for displaying/installing
- `NewSelfInstallCmd(rootCmd, info)` - Create self-install command
- `NewVersionCmd(info)` - Create version command
- `Note(format, args...)` - Print informational message
- `ReceiptsDir()` - Return receipts storage directory
- `Render(w, data, flags)` - Output data per format specification
- `RenderMutation(w, data, flags)` - Output if --passthru set
- `RenderMutationTo(data, flags)` - Render mutation to stdout
- `RenderTo(data, flags)` - Render to stdout
- `SetProgramName(name)` - Set program name for output prefixes
- `SetSilent(s)` - Enable/disable silent mode
- `SharedConfigPath()` - Return shared devlore config file path
- `StateHome()` - Return XDG_STATE_HOME
- `Success(format, args...)` - Print success message
- `ToolConfigPath(toolName)` - Return tool-specific config file path
- `Warn(format, args...)` - Print warning message
- `WriteReceipt(g, producer)` - Write graph as receipt with checksum
- `WritLayersDir()` - Return writ layers directory
- `ZshCompletionPath()` - Return zsh completion directory

### private

**types:**
- `exitError` - Error carrying exit code
- `installFlags` - Flags for self-install command

**constants:**
- `colorReset`, `colorRed`, `colorGreen`, `colorYellow`, `colorGray` - ANSI colors
- `symbolNote`, `symbolWarn`, `symbolError`, `symbolSuccess` - Output symbols

**variables:**
- `programName` - Current program name for output
- `silent` - Silent mode flag

---

## internal/execution

Core execution graph primitives and executor shared by writ and lore.

### PUBLIC

**Constants:**
- `StatePending` - Graph not yet executed
- `StateExecuted` - Graph completed successfully
- `StateFailed` - Graph execution failed
- `StatusPending` - Node not yet processed
- `StatusCompleted` - Node executed successfully
- `StatusSkipped` - Node was skipped
- `StatusFailed` - Node failed
- `ResultPending`, `ResultRunning`, `ResultCompleted`, `ResultFailed`, `ResultSkipped` - Result statuses
- `ResolutionStop` - Abort on first conflict
- `ResolutionBackup` - Move conflicts to backups
- `ResolutionOverwrite` - Remove conflicts without backup
- `ResolutionSkip` - Skip conflicts and continue
- `OpTransform`, `OpWriter`, `OpDirect` - Operation categories
- `ConflictNone`, `ConflictRegularFile`, `ConflictDirectory`, `ConflictForeignSymlink`, `ConflictOurSymlink` - Conflict types

**Types:**
- `Graph` - Execution graph with nodes and edges (the core data structure)
- `Node` - Single unit of work (ID, Operations, Status, Source, Target, etc.)
- `Edge` - Dependency relationship between nodes (From, To, Relation)
- `GraphState` - Execution state (pending, executed, failed)
- `NodeStatus` - Node execution status
- `Platform` - OS and architecture record
- `GraphContext` - Tool-specific metadata (SourceRoot, TargetRoot, Projects, etc.)
- `Summary` - Execution statistics (TotalFiles, Links, Copies, etc.)
- `Signature` - Cryptographic signature (Method, Value, Recipient)
- `Collision` - Source conflict record (Target, Winner, Loser, etc.)
- `GraphExecutor` - Executes operation graphs
- `ExecutorOptions` - Executor configuration (DryRun, Logger, Data, etc.)
- `OperationRegistry` - Maps operation names to implementations
- `Context` - Execution context for operations (DryRun, Logger, Data)
- `Result` - Outcome of executing a node
- `ResultStatus` - Execution status enum
- `ConflictResolution` - Conflict handling strategy
- `ConflictType` - Kind of conflict at target path
- `Conflict` - Pre-flight detected conflict
- `PreflightResult` - Results of conflict detection
- `Plan` - Builder for constructing execution graphs
- `Encoder` - Interface for graph serialization (Encode method)
- `Executable` - Interface for executable units (GetID, GetOperations, etc.)
- `Operation` - Base interface for all operations (Name, Category)
- `Transform` - Operations that transform content (decrypt, expand)
- `Writer` - Operations that write content to filesystem (copy)
- `Direct` - Operations managing own I/O (link, mkdir, etc.)
- `GraphBuilder` - Interface for building graphs
- `SubgraphBuilder` - Builds subgraphs from manifest files
- `BuildOptions` - Subgraph building configuration

**Operation Types (all implement Operation):**
- `BackupOp` - Move file to timestamped backup
- `CopyOp` - Write content to target
- `DecryptOp` - Decrypt content using SOPS
- `ExpandOp` - Process content as Go template
- `FileWriteOp` - Write inline content to target
- `LinkOp` - Create symlink
- `MkdirOp` - Create directory
- `RemoveOp` - Delete file
- `RenameOp` - Move file (git mv when possible)
- `UnlinkOp` - Remove symlink
- `ValidateOp` - Check precondition
- `ShellOp` - Execute shell command
- `PowerShellOp` - Execute PowerShell command (Windows)
- `PackageInstallOp` - Install packages
- `PackageRemoveOp` - Remove packages
- `PackageUpdateOp` - Refresh package manager index
- `PackageUpgradeOp` - Upgrade packages
- `LaunchdStartOp`, `LaunchdStopOp`, `LaunchdRestartOp`, `LaunchdEnableOp`, `LaunchdDisableOp` - macOS service ops
- `SystemdStartOp`, `SystemdStopOp`, `SystemdRestartOp`, `SystemdEnableOp`, `SystemdDisableOp` - Linux service ops
- `WinServiceStartOp`, `WinServiceStopOp`, `WinServiceRestartOp`, `WinServiceEnableOp`, `WinServiceDisableOp` - Windows service ops

**Functions:**
- `NewGraphExecutor(registry, opts)` - Create executor with registry and options
- `NewOperationRegistry()` - Create empty operation registry
- `NewPlan(project)` - Create new plan for building graph
- `AllOps()` - Return all operations for registration
- `FileOps()` - Return file operations
- `PackageOps()` - Return package manager operations
- `ServiceOps()` - Return service manager operations
- `Preflight(graph)` - Pre-flight conflict detection
- `ExpandDelegates(ctx, graph, builder, opts)` - Replace delegate nodes with subgraphs
- `ChecksumBytes(content)` - Compute SHA256 of bytes
- `ChecksumFile(path)` - Compute SHA256 of file
- `GitStyleChecksum(type, basename, content)` - Git-style checksum

**Methods:**
- `Graph.Serialize(enc)` - Write graph to encoder
- `Graph.Filename()` - Return standard filename
- `Graph.CanonicalContent()` - Return content for checksumming
- `Graph.ApplyResults(results)` - Update nodes from results
- `Graph.ComputeSummary()` - Calculate statistics
- `GraphExecutor.Run(ctx, g)` - Execute graph, update state
- `GraphExecutor.RunNodes(ctx, nodes, edges)` - Execute nodes (lower-level API)
- `OperationRegistry.Register(op)` - Add operation to registry
- `OperationRegistry.Get(name)` - Get operation by name
- `OperationRegistry.Names()` - Get all registered names
- `Plan.Link/Copy/Mkdir/Remove/Unlink/Backup/Rename/Validate(...)` - Add operations
- `Plan.DependsOn(from, to)` - Add ordering edge
- `Plan.Graph()` - Return built graph
- `Node.GetID/GetOperations/GetSource/GetTarget/GetProject/GetMode/GetMetadata()` - Node accessors

### private

**types:**
- `pipelineState` - Internal state for operation pipeline execution

---

## internal/writ

Writ CLI commands for configuration deployment.

### PUBLIC

**Constants:**
- `CurrentVersion` ("5") - Graph format version
- `VerifyOK`, `VerifyUnsigned`, `VerifyInvalid`, `VerifyMissing` - Verification results

**Variables:**
- `LayerOrder` - Processing order for repository layers (base, team, personal)

**Types:**
- `Config` - Base settings for lifecycle operations
- `DeployConfig` - Settings for deploy operation
- `DecommissionConfig` - Settings for decommission operation (+ Force, Prune)
- `UpgradeConfig` - Settings for upgrade operation (+ Force)
- `ReconcileConfig` - Settings for reconcile operation (+ CheckDrift, JSONOutput)
- `AdoptConfig` - Settings for adopt operation (+ Files, Layer, Project, etc.)
- `GraphBuilder` - Interface for graph builders (Build method)
- `DeployGraphBuilder` - Builds deploy graphs
- `DecommissionGraphBuilder` - Builds decommission graphs
- `UpgradeGraphBuilder` - Builds upgrade graphs
- `ReconcileGraphBuilder` - Builds reconcile graphs
- `AdoptGraphBuilder` - Builds adopt graphs
- `MigrateGraphBuilder` - Builds migrate graphs
- `TargetSpec` - Source directory and deployment target
- `VerifyResult` - Signature verification outcome

**Functions:**
- `NewRootCmd()` - Create root writ command
- `NewGraph(cfg)` - Create execution graph with writ defaults
- `BuildTree(g, cfg)` - Walk source directories, populate graph nodes
- `ConfigureEngine(cfg)` - Create and configure execution engine
- `CollectLayerSources()` - Gather configured repository layers
- `TargetOrder()` - Return processing order for targets
- `VerifyGraphSignature(g, identities)` - Verify graph signature
- `NewDeployGraphBuilder(cfg)` - Create deploy graph builder
- `NewDecommissionGraphBuilder(cfg, state)` - Create decommission graph builder
- `NewUpgradeGraphBuilder(cfg, state)` - Create upgrade graph builder
- `NewReconcileGraphBuilder(cfg)` - Create reconcile graph builder
- `NewAdoptGraphBuilder(cfg)` - Create adopt graph builder
- `NewMigrateGraphBuilder(cfg, sourcePath)` - Create migrate graph builder

**Methods:**
- `Config.SegmentMap()` - Return segments as string map
- `VerifyResult.String()` - Human-readable verification result
- All builder `.Build()` methods

### private

**types:**
- `upgradeResult` - Result enum for upgrade operations

**functions:**
- `runDeployV2`, `runDecommission`, `runUpgrade`, `runReconcile`, `runAdopt` - Command implementations
- `parseDeployConfig`, `parseDecommissionConfig`, etc. - Config parsers
- `reportGraphContext`, `reportCollisions` - Output helpers
- `builtinTemplateData`, `graphBuiltinTemplateData` - Template data builders
- `expandPath`, `projectSet`, `findSigningKey` - Utility functions
- `adoptFiles`, `adoptItem`, `adoptDirectory`, `adoptFile` - Adopt helpers
- `loadDecommissionState`, `updateDecommissionState` - State management
- `buildReconcileReport`, `reconcileFromState` - Reconcile helpers
- `outputDryRun`, `outputReconcileJSON`, `outputReconcileText` - Output formatters

---

## internal/writ/deploystate

Deployment state persistence with signing.

### PUBLIC

**Types:**
- `State` - Deployment state (SourceRoot, TargetRoot, Files map, Signature)
- `FileEntry` - Single file in state (Source, Project, Layer, checksums, Operations)
- `Signature` - State signature (Method, Value, Recipient)

**Functions:**
- `Load()` - Load state from disk
- `StatePath()` - Return state file path

**Methods:**
- `State.Write()` - Persist state to disk
- `State.Sign(identity)` - Sign state with age identity
- `State.Verify(identities)` - Verify state signature
- `State.IsSigned()` - Check if state is signed
- `State.AddEntry(relTarget, entry)` - Add file entry
- `State.RemoveEntry(relTarget)` - Remove file entry
- `State.UpdateChecksum(relTarget, source, target)` - Update checksums
- `State.CopiedFiles()` - Return files with copy operations
- `State.Projects()` - Return list of projects
- `FileEntry.IsCopied()` - Check if entry was copied (not linked)

---

## internal/writ/reconcile

Full-stack drift detection and repair.

### PUBLIC

**Constants:**
- `StateLinked` - Symlink exists and correct
- `StateConflict` - File exists but isn't our symlink
- `StateMissing` - Source exists but target symlink missing
- `StateOrphan` - Symlink points to nonexistent file
- `StateCopied` - File was copied and exists
- `StateStale` - Source changed since deployment
- `StateModified` - Target modified locally
- `StateDriftConflict` - Both source and target changed

**Types:**
- `State` - Status of a deployed file (enum)
- `Entry` - Status of a single file
- `Report` - Full status report

**Functions:**
- `FromBuildResult(br)` - Generate status from build result
- `ScanTarget(targetRoot, sourceRoot)` - Scan target for writ-managed symlinks

**Methods:**
- `State.String()` - Status indicator for display
- `State.Label()` - Human-readable label
- `Report.Summary()` - Counts of each state
- `Report.HasIssues()` - Check for non-linked/copied states

### private

**functions:**
- `nodeIsDelegate(ops)` - Check if node is delegate
- `checkEntry(source, target, ...)` - Check status of single file
- `checkEntryWithDrift(...)` - Check with checksums for drift

---

## internal/writ/tree

File tree building with layer/segment processing.

### PUBLIC

**Types:**
- `BuildConfig` - Configuration for tree building
- `BuildResult` - Result of tree building (Files, Collisions)
- `FileEntry` - Single file with operations
- `Collision` - Source conflict record
- `LayerSource` - Layer configuration (Layer, SourceRoot, TargetRoot, etc.)
- `Operation` - File operation enum (OpLink, OpCopy, OpExpand, OpDecrypt, etc.)
- `Operations` - Slice of operations with methods

**Functions:**
- `Build(cfg)` - Build file tree from configuration
- `ProcessingPipeline(filename)` - Determine operations from filename

**Methods:**
- `Operations.Strings()` - Convert to string slice
- `Operations.Has(op)` - Check for operation

---

## internal/writ/segment

Platform segment detection and matching.

### PUBLIC

**Types:**
- `Segment` - Named segment with value
- `Segments` - Slice of segments with methods
- `Matcher` - Matches paths against segments

**Functions:**
- `DetectSegments()` - Detect platform segments (OS, ARCH, DISTRO, etc.)

**Methods:**
- `Segments.LoadFromEnv()` - Load segment values from environment
- `Segments.String()` - String representation
- `Segments.Get(name)` - Get segment by name
- `Matcher.Match(path)` - Match path against segment patterns

---

## internal/host

Platform detection and package/service management.

### PUBLIC

**Types:**
- `Platform` - Platform info (OS, Distro, Version, Arch, etc.)
- `PackageManager` - Interface for package managers
- `ServiceManager` - Interface for service managers
- `Host` - Platform-specific host implementation
- `Result` - Command execution result
- `SearchResult` - Package search result

**Functions:**
- `NewHost()` - Create host for current platform
- `DetectPlatform()` - Detect current platform

**Methods:**
- `Platform.String()` - Platform string representation
- `Host.Platform()` - Get platform info
- `Host.PackageManager()` - Get package manager
- `Host.ServiceManager()` - Get service manager
- `PackageManager.Install/Remove/Upgrade/Update/Search/IsInstalled/List(...)` - PM operations
- `ServiceManager.Start/Stop/Restart/Enable/Disable/Status(...)` - Service operations

### private

**types:**
- `linuxHost`, `darwinHost`, `windowsHost` - Platform-specific hosts
- `aptManager`, `dnfManager`, `brewManager`, `portManager`, `wingetManager` - Package managers
- `darwinServiceManager`, `windowsServiceManager` - Service managers

---

## internal/lore

Lore CLI commands for package management.

### PUBLIC

**Functions:**
- `NewRootCmd()` - Create root lore command

### private

**variables:**
- `version`, `commit`, `buildDate` - Build metadata

**functions:**
- `initConfig` - Initialize configuration
- `newDeployCmd`, `newUpgradeCmd`, `newDecommissionCmd`, etc. - Command constructors
- `runDeploy`, `runUpgrade`, etc. - Command implementations

---

## internal/lorepackage

Package resolution and lifecycle management.

### PUBLIC

**Constants:**
- `SourceLore`, `SourceApt`, `SourceDnf`, `SourceBrew`, `SourcePort`, `SourceWinget` - Package sources
- `OpDeploy`, `OpUpgrade`, `OpDecommission` - Lifecycle operations
- `PMInstall`, `PMUpgrade`, `PMRemove` - Package manager operations

**Types:**
- `PackageSource` - Where package comes from
- `Release` - Resolved package release
- `Operation` - Lifecycle operation enum
- `Lifecycle` - Package lifecycle with phases
- `PhaseAction` - Action within a phase
- `ScriptAction` - Script-based action
- `NativePMAction` - Native package manager action
- `Registry` - Package registry interface
- `PMOperation` - Package manager operation enum

**Functions:**
- `Resolve(name, opts)` - Resolve package to release
- `VerifySyntheticPackage(path)` - Verify synthetic package structure
- `SyntheticCache()` - Get synthetic package cache
- `ParsePackagePrefix(spec)` - Parse package manager prefix
- `Lifecycle(release, op)` - Get lifecycle for operation
- `DiscoverPhaseScripts(dir)` - Find phase scripts in directory
- `HasPhase(lifecycle, phase)` - Check if lifecycle has phase
- `PhaseActions(lifecycle, phase)` - Get actions for phase
- `IsNative(source)` - Check if native package manager
- `IsSynthetic(source)` - Check if synthetic (lore) package

---

## internal/manifest

Package manifest parsing and validation.

### PUBLIC

**Types:**
- `PackagesManifest` - Parsed packages-manifest.yaml
- `PackageEntry` - Single package entry
- `PackageOptions` - Package-specific options

**Functions:**
- `Load(path)` - Load manifest from file
- `Parse(data)` - Parse manifest from bytes
- `Validate(manifest)` - Validate manifest structure
- `ValidateBytes(data)` - Validate manifest bytes
- `IsManifestFile(filename)` - Check if file is manifest

**Methods:**
- `PackagesManifest.PackageNames()` - Get list of package names
- `PackagesManifest.String()` - String representation

---

## Other Packages (abbreviated)

### internal/console
TUI console for interactive operations (Model, Console, Styles)

### internal/credentials
Credential storage (Get, Set, Delete)

### internal/shell
Shell command execution (Session, Result, Run, Script)

### internal/pwsh
PowerShell execution (Run, RunWithInput)

### internal/model
AI model configuration (Config, Provider, LoadConfig, SelectModel)

### internal/starlark
Starlark bindings for package scripts (Bindings, Exec, LoadModule)

### internal/bindgen
CLI binding generation (BindingDef, Command, LoadYAML, Merge)

### internal/tools/docgen
Documentation generation (PageData, GenerateTree, BuildPageData)
