# devlore-cli Package Reference

Two-tiered list of all Go packages. Public elements (uppercase) listed first, then private elements, both sorted alphabetically.

---

## cmd/docgen
- **main** (func): Entry point for docgen CLI tool
- countCommands (func): Count commands recursively
- run (func): Generate CLI reference documentation

## cmd/indexgen
- **ExampleEntry** (struct): Describes an examples asset with discovery metadata
- **KnowledgeIndex** (struct): Represents index.yaml manifest for knowledge domain
- **PromptEntry** (struct): Describes a prompt asset with discovery metadata
- **SchemaEntry** (struct): Describes a JSON schema asset with discovery metadata
- **SignatureEntry** (struct): Describes a signature asset with discovery metadata
- **SlotEntry** (struct): Describes a slots asset with discovery metadata
- **TransformEntry** (struct): Describes a transform asset with discovery metadata
- **main** (func): Entry point for index generator
- buildIndex (func): Build index for a knowledge domain
- listFiles (func): List files in a directory
- loadExistingIndex (func): Load existing index to preserve metadata
- mergeExamples (func): Merge example entries preserving metadata
- mergePrompts (func): Merge prompt entries preserving metadata
- mergeSchemas (func): Merge schema entries preserving metadata
- mergeSignatures (func): Merge signature entries preserving metadata
- mergeSlots (func): Merge slot entries preserving metadata
- mergeTransforms (func): Merge transform entries preserving metadata
- writeIndex (func): Write index to YAML file

## cmd/lore
- **main** (func): Entry point for lore CLI tool

## cmd/writ
- **main** (func): Entry point for writ CLI tool

## internal/cli
- **AddMutationFlags** (func): Adds --passthru and --format flags for mutating commands
- **AddOutputFlags** (func): Adds --filter and --format flags to command
- **AddSilentFlag** (func): Adds --silent flag to root command
- **AllSettings** (func): Returns all settings as map
- **BashCompletionPath** (func): Returns bash completion directory
- **BindFlags** (func): Binds all persistent flags from command to Viper
- **BindFlagsWithPrefix** (func): Binds flags with custom prefix
- **CacheHome** (func): Returns XDG_CACHE_HOME or ~/.cache
- **ConfigFileUsed** (func): Returns config file path that Viper loaded
- **ConfigHome** (func): Returns XDG_CONFIG_HOME or ~/.config
- **ConfigInfo** (struct): Contains configuration metadata for a tool
- **DataHome** (func): Returns XDG_DATA_HOME or ~/.local/share
- **Debug** (func): Prints current Viper state for debugging
- **DefaultFormat** (const): Default output format (JSON for scriptability)
- **DevloreCacheHome** (func): Returns unified devlore cache directory
- **DevloreConfigHome** (func): Returns unified devlore config directory
- **DevloreDataHome** (func): Returns unified devlore data directory
- **DevloreStateHome** (func): Returns unified devlore state directory
- **ErrManNotAvailable** (var): Indicates man command is not available
- **Error** (func): Prints error message to stderr
- **ExitCantCreate, ExitDataErr, ExitError, ExitIOErr, ExitNoInput, ExitNoPerm, ExitOK, ExitSoftware, ExitUnavailable, ExitUsage** (const): Exit codes (BSD sysexits.h)
- **ExitCode** (func): Extracts exit code from error
- **ExitWith** (func): Returns error that carries specific exit code
- **Failure** (func): Prints error message and returns error
- **FishCompletionPath** (func): Returns fish completion directory
- **Get** (func): Retrieves config value with tool's section prefix
- **GetBool** (func): Retrieves boolean config value
- **GetInt** (func): Retrieves integer config value
- **GetString** (func): Retrieves string config value
- **GetStringMap** (func): Retrieves string map config value
- **GetStringSlice** (func): Retrieves string slice config value
- **InitViper** (func): Initializes Viper with standard devlore conventions
- **LatestReceiptPath** (func): Returns path to latest receipt symlink
- **LoadLatestReceipt** (func): Loads most recent receipt for producer
- **LoadReceipt** (func): Loads execution graph from YAML receipt file
- **ManHeader** (struct): Contains metadata for man page generation
- **ManPath** (func): Returns user man page directory
- **MutationFlags** (struct): Holds flags for mutating commands (--passthru)
- **NewConfigCmd** (func): Creates config command with git-style subcommands
- **NewHelpCmd** (func): Creates help command that prefers man pages when available
- **NewManCmd** (func): Creates man command for displaying/installing man pages
- **NewSelfInstallCmd** (func): Creates self-install command
- **NewVersionCmd** (func): Creates version command
- **Note** (func): Prints informational message to stderr
- **OutputFlags** (struct): Holds --filter and --format flag values
- **ReceiptsDir** (func): Returns directory where receipts are stored
- **Render** (func): Outputs data according to format specification
- **RenderMutation** (func): Outputs data only if --passthru is set
- **RenderMutationTo** (func): Convenience function that renders to stdout if --passthru is set
- **RenderTo** (func): Convenience function that renders to stdout
- **SelfInstallInfo** (struct): Contains metadata needed for self-installation
- **SetProgramName** (func): Sets program name used in output prefixes
- **SetSilent** (func): Enables or disables silent mode
- **SharedConfigPath** (func): Returns path to shared devlore config file
- **StateHome** (func): Returns XDG_STATE_HOME or ~/.local/state
- **Success** (func): Prints success message to stderr
- **ToolConfigPath** (func): Returns path to tool-specific config file
- **VersionInfo** (struct): Contains version metadata set at build time
- **ViperConfig** (struct): Holds configuration for Viper initialization
- **Warn** (func): Prints warning message to stderr
- **WriteReceipt** (func): Writes graph as receipt to receipts directory
- **WriteReceiptWithSigningDir** (func): Writes graph with signing directory configuration
- **WritLayersDir** (func): Returns writ layers directory
- **ZshCompletionPath** (func): Returns zsh completion directory
- applyFilter (func): Filters data using key=value pairs (AND logic)
- coerceValue (func): Converts string value to appropriate Go type based on schema
- configEdit (func): Opens config file in user's editor
- configFormat (func): Returns "json" or "yaml" based on file extension
- configKeyCompletion (func): Returns ValidArgsFunction for config key completion
- configPath (func): Shows config file location
- configSchema (func): Outputs embedded JSON schema
- configValidate (func): Validates config against schema
- copyFile (func): Copies file from src to dst
- deleteNestedValue (func): Removes value from nested map using dot notation
- detectShells (func): Returns list of shells available on system
- exitError (struct): Wraps error with specific exit code
- expandTilde (func): Expands ~ to $HOME in path
- extractKeys (func): Recursively extracts keys from JSON schema properties
- formatFieldValue (func): Converts value to string for display
- formatValue (func): Formats value for display
- getFieldNames (func): Returns field names from struct or map
- getFieldValue (func): Retrieves field value from struct or map
- getNestedValue (func): Retrieves value from nested map using dot notation
- getSchemaKeys (func): Extracts all valid config keys from JSON schema
- hasMan (func): Returns true if man command is available
- initDevloreCache (func): Creates unified devlore cache structure
- initDevloreConfig (func): Creates unified devlore config structure
- initWritLayers (func): Creates writ layer directories
- installBinary (func): Copies current executable to target location
- installCompletionsForShells (func): Installs completions for specified shells
- installFlags (struct): Holds flag values for self-install
- installManPagesTo (func): Installs man pages and returns list of installed files
- loadConfig (func): Loads config file as map, supports YAML and JSON
- manPagePath (func): Returns expected path for command's man page
- matchesAllFilters (func): Checks if item matches all filter expressions
- matchesFilter (func): Checks if item matches single key=value filter
- newConfigEditCmd (func): Creates config edit subcommand
- newConfigGetCmd (func): Creates config get subcommand
- newConfigListCmd (func): Creates config list subcommand
- newConfigPathCmd (func): Creates config path subcommand
- newConfigSchemaCmd (func): Creates config schema subcommand
- newConfigSetCmd (func): Creates config set subcommand
- newConfigUnsetCmd (func): Creates config unset subcommand
- newConfigValidateCmd (func): Creates config validate subcommand
- printFlattened (func): Prints nested map in key=value format
- printShellSetupInstructions (func): Prints setup instructions for installed shells
- renderJSON (func): Outputs data as JSON
- renderTable (func): Outputs data as formatted table
- renderTemplate (func): Outputs data using Go text/template string
- renderYAML (func): Outputs data as YAML
- runSelfInstall (func): Performs complete installation
- saveConfig (func): Saves config map to file
- schemaTypeForKey (func): Walks JSON schema to find type declaration for dot-path key
- setNestedValue (func): Sets value in nested map using dot notation
- shellCompletionPath (func): Returns installation path and filename for shell's completion file
- signGraph (func): Signs graph using first available signing backend
- toSlice (func): Converts data to slice of interfaces

## internal/console
- **Console** (struct): Main console output handler
- **DefaultStyles** (func): Returns default Styles
- **DefaultTheme** (func): Returns default theme
- **Model** (struct): Bubble Tea model for interactive output
- **New** (func): Creates new Console
- **NewModel** (func): Creates new Model with session
- **NewStyles** (func): Creates new Styles from theme
- **Option** (struct): Represents optional choice for user
- **Session** (interface): Represents an output session with steps and options
- **Step** (struct): Represents single operation step
- **StepType** (const): Type of step (pending, running, completed, failed, skipped)
- **Styles** (struct): Collection of lipgloss styles
- **Theme** (struct): Color and styling theme

## internal/credentials
- **CredentialsFilePath** (func): Returns path to credentials file
- **Delete** (func): Removes credential by key
- **Get** (func): Retrieves credential value by key
- **Set** (func): Stores credential securely

## internal/execution
- **ChecksumBytes** (func): Computes checksum of bytes
- **ChecksumFile** (func): Computes checksum of file
- **Conflict** (struct): Represents conflict in graph
- **ConflictPolicy** (type): Strategy for conflict resolution
- **ConflictType** (type): Type of conflict detected
- **Edge** (struct): Dependency relationship between nodes
- **ExecutorOptions** (struct): Options for executor
- **GitStyleChecksum** (func): Computes git-style checksum
- **Graph** (struct): Directed graph of nodes and edges
- **GraphBuilder** (interface): Interface for building graphs
- **GraphContext** (struct): Contains tool-specific metadata
- **GraphExecutor** (struct): Executes graphs by running operations
- **GraphState** (type): Represents execution state of graph
- **NewGraphExecutor** (func): Creates new executor
- **Node** (struct): Single unit of work with operations
- **NodeStatus** (type): Represents execution status of node
- **Operation** (interface): Represents executable operation
- **OperationRegistry** (struct): Maps operation names to implementations
- **Platform** (struct): Records OS and architecture
- **Preflight** (func): Performs preflight checks on graph
- **PreflightResult** (struct): Results of preflight checks
- **Result** (struct): Result of operation execution
- **ResultStatus** (type): Status of operation result
- **Signature** (struct): Contains signature data for graph
- **Summary** (struct): Contains execution statistics

## internal/host
- **DetectPlatform** (func): Detects current platform
- **Host** (interface): Platform abstraction interface
- **NewHost** (func): Creates appropriate Host implementation
- **PackageManager** (interface): Manages system packages
- **Platform** (struct): Describes OS and architecture
- **Result** (struct): Result of command execution
- **SearchResult** (struct): Result of search operation
- **ServiceManager** (interface): Manages system services

## internal/lore
- **NewRootCmd** (func): Creates root lore command with subcommands
- initConfig (func): Initializes Viper configuration

## internal/lorepackage
- **Action** (struct): Package action/operation
- **DefaultSearchOptions** (func): Returns default search options
- **Git** (struct): Git operations
- **Lifecycle** (struct): Package lifecycle hooks
- **Load** (func): Loads manifest from file
- **LoadLifecycle** (func): Loads lifecycle from package directory
- **LoadPackages** (func): Loads packages from registry
- **LoadRegistryConfig** (func): Loads registry configuration
- **LoadSignatures** (func): Loads package signatures
- **MatchResult** (struct): Result of package matching
- **New** (func): Creates new registry
- **NewDefault** (func): Creates registry with default configuration
- **NewSyntheticCache** (func): Creates synthetic cache
- **Package** (struct): Represents lore package
- **PackagesManifest** (struct): Declares software dependencies
- **Parse** (func): Parses packages manifest from bytes
- **Registry** (struct): Package registry client
- **Schema** (const): JSON schema for packages manifest
- **Search** (struct): Search operation
- **SearchOptions** (struct): Options for search
- **SyntheticCache** (struct): Caches synthetic packages
- **Validate** (func): Validates manifest against schema
- **ValidateBytes** (func): Validates manifest bytes against schema

## internal/manifest
- **Builder** (struct): Builds manifests
- **Load** (func): Loads manifest from file
- **NewBuilder** (func): Creates new manifest builder
- **PackagesManifest** (struct): Manifest of packages and their metadata
- **Parse** (func): Parses manifest from bytes
- **Validate** (func): Validates manifest

## internal/model
- **AnthropicProvider** (struct): Anthropic API provider
- **AzureOpenAIProvider** (struct): Azure OpenAI provider
- **CLIFlags** (struct): Command-line flags for model selection
- **ChatRequest** (struct): Request to model provider
- **ChatResponse** (struct): Response from model provider
- **Config** (struct): Model provider configuration
- **ConfigPath** (func): Returns config file path
- **DefaultConfig** (func): Returns default configuration
- **EnsureProvider** (func): Ensures model provider is available
- **LoadConfig** (func): Loads model configuration
- **Message** (struct): Chat message
- **NewAnthropicProvider** (func): Creates Anthropic provider
- **NewAzureOpenAIProvider** (func): Creates Azure OpenAI provider
- **NewOllamaProvider** (func): Creates Ollama provider
- **NewOpenAIProvider** (func): Creates OpenAI provider
- **OllamaProvider** (struct): Local Ollama provider
- **OpenAIProvider** (struct): OpenAI API provider
- **Provider** (interface): Model provider interface
- **Role** (type): Message role (user, assistant, system)
- **SaveConfig** (func): Saves model configuration

## internal/pwsh
- **Audit** (func): Logs command for audit trail
- **Available** (func): Checks if PowerShell is available
- **Bootstrap** (func): Performs bootstrap operations
- **Ensure** (func): Ensures PowerShell is installed
- **Eval** (func): Evaluates PowerShell expression
- **History** (func): Returns command history
- **New** (func): Creates new PowerShell session
- **Result** (struct): PowerShell command execution result
- **Run** (func): Runs PowerShell command
- **Script** (func): Runs PowerShell script
- **Session** (struct): PowerShell session
- **Set** (func): Sets variable in session
- **Version** (func): Gets PowerShell version
- **With** (func): Sets variable for single command

## internal/shell
- **Audit** (func): Logs command for audit trail
- **Eval** (func): Evaluates shell expression
- **History** (func): Returns command history
- **New** (func): Creates new shell session
- **Result** (struct): Shell command execution result
- **Run** (func): Runs shell command
- **Script** (func): Runs shell script
- **Session** (struct): Shell session
- **Set** (func): Sets variable in session
- **With** (func): Sets variable for single command

## internal/signing
- **AWSKMSSigner** (struct): AWS KMS signing backend
- **AzureKVSigner** (struct): Azure Key Vault signing backend
- **Backend** (type): Signing backend type
- **BuildSignerChain** (func): Builds signer chain from .sops.yaml
- **CreationRule** (struct): Creation rule from SOPS config
- **FindSopsConfig** (func): Finds .sops.yaml in directory tree
- **GCPKMSSigner** (struct): GCP KMS signing backend
- **GPGSigner** (struct): GPG signing backend
- **NewAWSKMSSigner** (func): Creates AWS KMS signer
- **NewAzureKVSigner** (func): Creates Azure KV signer
- **NewGCPKMSSigner** (func): Creates GCP KMS signer
- **NewGPGSigner** (func): Creates GPG signer
- **NewSignerChain** (func): Creates new signer chain
- **ParseSopsConfig** (func): Parses SOPS configuration
- **ParsedBackend** (struct): Parsed backend configuration
- **SignError** (struct): Signing operation error
- **Signature** (struct): Contains signature data
- **Signer** (interface): Signing operation interface
- **SignerChain** (struct): Chain of signing backends
- **SopsConfig** (struct): SOPS configuration
- **VerifyAWSKMS** (func): Verifies AWS KMS signature
- **VerifyAzureKV** (func): Verifies Azure KV signature
- **VerifyError** (struct): Signature verification error
- **VerifyGCPKMS** (func): Verifies GCP KMS signature

## internal/starlark
- **Bindings** (struct): Starlark bindings container
- **DockerBindings** (struct): Docker CLI bindings
- **GitBindings** (struct): Git CLI bindings
- **GitProvider** (struct): Git repository provider
- **NewBindings** (func): Creates new bindings
- **NewDockerBindings** (func): Creates Docker bindings
- **NewGitBindings** (func): Creates Git bindings
- **NewGitProvider** (func): Creates Git provider
- **NewNpmBindings** (func): Creates NPM bindings
- **NewSession** (func): Creates new session
- **NewSessionWithProvider** (func): Creates session with provider
- **NewSystemBindings** (func): Creates system bindings
- **NpmBindings** (struct): NPM CLI bindings
- **PlanBindings** (interface): Bindings for plan operations
- **Session** (struct): Starlark interpreter session
- **SystemBindings** (struct): System operation bindings

## internal/starlark/platform
- **CommonPlanBindings** (struct): Common plan bindings
- **DarwinPlanBindings** (struct): Darwin-specific bindings
- **LinuxPlanBindings** (struct): Linux-specific bindings
- **NewPlatformPlanBindings** (func): Creates platform-specific bindings
- **WindowsPlanBindings** (struct): Windows-specific bindings

## internal/tools/docgen
- **BuildPageData** (func): Builds page data from command
- **ChildInfo** (struct): Information about child command
- **FlagInfo** (struct): Information about command flag
- **GenerateTree** (func): Generates documentation tree for commands
- **PageData** (struct): Data for documentation page template
- **ParentInfo** (struct): Information about parent command

## internal/writ
- **NewRootCmd** (func): Creates root writ command with subcommands
- **VerifyGraph** (func): Verifies graph integrity
- **VerifyGraphSignature** (func): Verifies execution graph signature
- **VerifyResult** (type): Result of signature verification

## internal/writ/deploystate
- **Load** (func): Loads deployment state
- **LoadFrom** (func): Loads state from specific path
- **Save** (func): Saves deployment state
- **Signature** (struct): Deployment signature
- **State** (struct): Deployment state tracker

## internal/writ/identity
- **GenerateIdentity** (func): Generates age identity
- **IdentityToRecipient** (func): Converts identity to recipient
- **LoadIdentities** (func): Loads identities from default locations
- **LoadIdentitiesFromPaths** (func): Loads identities from specific paths

## internal/writ/migrate
- **AnalyzeScripts** (func): Analyzes shell scripts
- **BuildMigration** (func): Builds migration graph
- **BuildMigrationAnalysis** (func): Builds migration analysis
- **Classify** (func): Classifies inventory entries
- **Detect** (func): Detects source system
- **DetectEncryptedFile** (func): Detects file encryption
- **DetectEncryption** (func): Detects encryption systems
- **DetectWithSignatures** (func): Detects with signature matching
- **DetectionResult** (struct): Result of system detection
- **EncryptedFileInfo** (struct): Information about encrypted file
- **EncryptionSystem** (type): Encryption system type
- **Inventory** (func): Returns file inventory
- **InventoryEntry** (struct): Entry in file inventory
- **MigrateGraphBuilder** (struct): Builds migration execution graph
- **MigrationAnalysis** (struct): Analysis of migration
- **NewMigrateGraphBuilder** (func): Creates migration graph builder
- **NewPlan** (func): Creates new plan
- **NewSession** (func): Creates migration session
- **Options** (struct): Migration options
- **Plan** (struct): Migration plan
- **ScriptAnalysis** (struct): Analysis of shell script
- **Session** (struct): Migration session
- **Signature** (struct): Signature for detection
- **SourceSystem** (type): Source system type

## internal/writ/reconcile
- **Reconcile** (func): Reconciles configuration
- **ReconcileGraphBuilder** (struct): Builds reconciliation graph

## internal/writ/secrets
- **DecryptData** (func): Decrypts data
- **DecryptFile** (func): Decrypts file
- **IsEncrypted** (func): Checks if data is encrypted
- **IsSecretFile** (func): Checks if file is secret
- **Manager** (struct): Secrets manager
- **NewManager** (func): Creates secrets manager

## internal/writ/segment
- **Detect** (func): Detects segment matches
- **DetectSegments** (func): Detects segments
- **DetectSegmentsWithNames** (func): Detects segments by name
- **SegmentMatcher** (struct): Segment matching logic
- **Segments** (struct): Platform segments

## internal/writ/tree
- **BuildResult** (struct): Result of tree build
- **BuildTree** (func): Builds deployment tree
- **CollectLayerSources** (func): Collects layer sources
- **FromBuildResult** (func): Converts build result to report
- **GroupByProject** (func): Groups results by project
- **LayerSource** (struct): Layer source configuration
- **Operation** (struct): Tree operation
- **Report** (struct): Build report
- **ScanSource** (func): Scans source directory
- **ScanTarget** (func): Scans target directory

## schema
- **DevloreSchema** (var): Shared JSON schema for devlore config
- **LoreDefaultConfig** (var): Default lore configuration
- **PackagesManifestSchema** (var): JSON schema for packages manifest
- **SharedDefaultConfig** (var): Default shared configuration
- **WritDefaultConfig** (var): Default writ configuration

---

**Summary**: 4 cmd packages, 20+ internal packages with subpackages, 1 schema package.
