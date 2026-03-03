# Plan: Redesign `writ migrate` Analysis with LLM-First Approach

## Goal

Replace the broken heuristic-based analysis in `writ migrate` with an LLM-first approach that uses proper inputs:
1. `tree -J` output (directory structure as JSON)
2. Contents of all executable scripts
3. LLM analyzes these inputs and returns structured output

## Problem Statement

The current `writ migrate` analysis is fundamentally broken:

| Issue | Current Behavior | Correct Behavior |
|-------|------------------|------------------|
| Directory scanning | Scans root level only | Should scan inside `Home/Configs/` |
| Project detection | Returns top-level dirs (Home, Tools, etc.) | Should find actual project groups |
| Platform detection | Misses `all-Darwin`, `noblefactor-Unix` patterns | Should detect `<group>-<Platform>` inside Home/Configs |
| Tuckr detection | Doesn't detect tuckr usage in scripts | Should read scripts that call `tuckr add`, `tuckr rm` |
| LLM input | Sends garbage summarized inventory | Should send tree + script contents |
| Execution graph | Empty (no renames found) | Should have rename operations |

## New Architecture

### Input Gathering

```
1. Walk directory tree with Go    → Directory structure as JSON (cross-platform)
2. Find executables with Go       → List of executable scripts (check mode bits, *.ps1)
3. Read executable contents       → Script source code (up to token budget)
```

Note: Cannot use `tree` command - not available on Windows. Build tree structure in Go using `filepath.WalkDir`.

### LLM Analysis Flow

The LLM receives:
- The tree JSON output
- Contents of all executable scripts
- Knowledge of dotfile systems (tuckr, stow, chezmoi, yadm, etc.)
- Writ naming conventions (`<project>.<Platform>` not `<project>-<Platform>`)

The LLM performs analysis in order:

1. **Summarize inputs**: Describe what the tree structure shows, what scripts are present, what patterns are visible
2. **Identify system**: Detect dotfile system (tuckr, stow, chezmoi, etc.) based on evidence
3. **Analyze structure**: Where do groups live? What naming convention is used? What platforms are targeted?
4. **Make observations**: What's notable about this repository?
5. **Identify warnings**: What might cause problems? (encryption systems, unusual patterns)
6. **Make recommendations**: What should the user do after migration?
7. **Generate execution graph**: Finally, produce the concrete rename operations

### Output Format

The output follows the analysis flow - analysis first, then execution graph:

```json
{
  "analysis": {
    "source_root": "/path/to/source",
    "system": "tuckr",
    "system_confidence": 0.95,
    "input_summary": "Repository with Home/Configs/ containing 13 group directories. Root has Install-UnixUserConfiguration and Install-WindowsUserConfiguration.ps1 scripts that invoke tuckr commands.",
    "structure": {
      "groups_path": "Home/Configs",
      "naming_convention": "<group>-<Platform>",
      "groups": ["all", "all-Darwin", "all-Linux", "all-Unix", "all-Windows", "microsoft", "microsoft-Unix", "noblefactor", "noblefactor-Unix", "thenobles", "thenobles-Darwin"],
      "platforms": ["Darwin", "Linux", "Unix", "Windows", "Debian"]
    },
    "observations": [
      "Tuckr-managed repository with groups in Home/Configs/",
      "Install scripts at root invoke tuckr add/rm for deployment",
      "Uses git-crypt for secret encryption"
    ],
    "warnings": [
      "git-crypt detected - writ uses SOPS; consider migration"
    ],
    "recommendations": [
      "After migration, update Install-UnixUserConfiguration to use new group names",
      "Create .sops.yaml to migrate from git-crypt to SOPS"
    ]
  },
  "execution_graph": {
    "version": "1.0",
    "tool": "writ",
    "state": "pending",
    "context": {
      "source_root": "/path/to/source"
    },
    "nodes": [
      {"id": "rename-1", "operations": ["rename"], "source": "Home/Configs/all-Darwin", "target": "Home/Configs/all.Darwin", "status": "pending"},
      {"id": "rename-2", "operations": ["rename"], "source": "Home/Configs/all-Linux", "target": "Home/Configs/all.Linux", "status": "pending"},
      {"id": "rename-3", "operations": ["rename"], "source": "Home/Configs/all-Unix", "target": "Home/Configs/all.Unix", "status": "pending"}
    ],
    "edges": [
      {"from": "rename-1", "to": "rename-2", "relation": "orders"},
      {"from": "rename-2", "to": "rename-3", "relation": "orders"}
    ]
  }
}
```

## Implementation

### Phase 1: New Input Gathering

Create new file: `internal/writ/migrate/gather.go`

```go
// TreeNode represents a directory or file in the tree.
type TreeNode struct {
    Type     string      `json:"type"`               // "directory" or "file"
    Name     string      `json:"name"`               // File/directory name
    Contents []*TreeNode `json:"contents,omitempty"` // Children (directories only)
}

// GatherInput collects tree output and script contents for LLM analysis.
type GatherInput struct {
    Tree        *TreeNode        // Directory structure
    Executables []ExecutableFile // Scripts with contents
}

type ExecutableFile struct {
    Path     string // Relative path
    Contents string // File contents
}

func GatherInputs(root string, maxDepth int) (*GatherInput, error)
```

Implementation (pure Go, cross-platform):
1. Walk directory with `filepath.WalkDir`, build `TreeNode` structure up to maxDepth
2. Track executables during walk: check `info.Mode()&0111 != 0` (Unix) or `*.ps1` extension (Windows)
3. Skip `.git` directory
4. Read each executable's contents
5. Apply token budget (skip files > 50KB, prioritize Install-*, Initialize-*)

### Phase 2: New LLM Analysis

Replace `enhanceAnalysisWithAI` with `AnalyzeWithLLM`:

```go
// LLMAnalysisResult is the structured response from the LLM.
type LLMAnalysisResult struct {
    Analysis       MigrationAnalysis `json:"analysis"`
    ExecutionGraph execution.Graph   `json:"execution_graph"`
}

func AnalyzeWithLLM(ctx context.Context, provider model.Provider, input *GatherInput) (*LLMAnalysisResult, error)
```

The prompt should:
1. Explain writ naming conventions (`.` not `-` for platform segments)
2. List known dotfile systems and their signatures
3. Ask for structured JSON matching the output schema
4. Request the LLM to identify where groups live, what renames are needed

### Phase 3: Update BuildMigration

Replace the current flow:

```go
func BuildMigration(ctx context.Context, opts Options) (*execution.Graph, *MigrationAnalysis, error) {
    // 1. Gather inputs (tree + scripts)
    input, err := GatherInputs(opts.SourceRoot, 5)
    if err != nil {
        return nil, nil, fmt.Errorf("gather inputs: %w", err)
    }

    // 2. LLM analysis
    result, err := AnalyzeWithLLM(ctx, opts.Provider, input)
    if err != nil {
        return nil, nil, fmt.Errorf("LLM analysis: %w", err)
    }

    // 3. Return graph and analysis
    return &result.ExecutionGraph, &result.Analysis, nil
}
```

### Phase 4: Update Output Format

Update `FormatMigrationPlan` in `format.go` to output both `analysis` and `execution_graph` at the top level:

```go
type migrationOutput struct {
    Analysis       *MigrationAnalysis `json:"analysis"`
    ExecutionGraph *execution.Graph   `json:"execution_graph"`
}
```

## Files to Modify

| File | Change |
|------|--------|
| `internal/writ/migrate/gather.go` | **NEW** - Input gathering (tree + scripts) |
| `internal/writ/migrate/plan.go` | Rewrite `BuildMigration` to use LLM-first approach |
| `internal/writ/migrate/format.go` | Update output to include `execution_graph` |
| `internal/writ/migrate/analysis.go` | Add `InputSummary` and `Structure` fields to MigrationAnalysis |

## Files to Delete

| File | Reason |
|------|--------|
| `internal/writ/migrate/detect.go` | Heuristic detection replaced by LLM |
| `internal/writ/migrate/inventory.go` | Replaced by tree -J |
| `internal/writ/migrate/scan.go` | Replaced by tree -J |
| `internal/writ/migrate/classify.go` | LLM handles classification |
| `internal/writ/migrate/signatures.go` | LLM has built-in knowledge |
| `internal/writ/migrate/signature.go` | LLM has built-in knowledge |

Keep but simplify:
- `analyze.go` - Script analysis (may still be useful for detailed parsing)
- `graph.go` - Graph building from LLM output

## New Types

Add to `analysis.go`:

```go
// StructureInfo describes the detected repository structure.
type StructureInfo struct {
    GroupsPath       string   `json:"groups_path"`        // e.g., "Home/Configs"
    NamingConvention string   `json:"naming_convention"`  // e.g., "<group>-<Platform>"
    Groups           []string `json:"groups"`             // List of group names
    Platforms        []string `json:"platforms"`          // List of platforms
}

// Add to MigrationAnalysis:
type MigrationAnalysis struct {
    // ... existing fields ...

    // InputSummary describes what the LLM saw in the inputs.
    InputSummary string `json:"input_summary,omitempty" yaml:"input_summary,omitempty"`

    // Structure describes the detected repository structure.
    Structure *StructureInfo `json:"structure,omitempty" yaml:"structure,omitempty"`
}
```

## LLM Prompt Design

```
You are analyzing a dotfiles repository for migration to writ conventions.

## Writ Conventions
- Groups live in Home/Configs/ or Home/<project>/
- Naming: <group>.<Platform> (e.g., all.Darwin, noblefactor.Unix)
- NOT: <group>-<Platform> (this is the legacy convention to migrate FROM)
- Known platforms: Darwin, Linux, Unix, Windows, Debian, Ubuntu, RHEL, Fedora, Arch

## Known Dotfile Systems
- tuckr: Groups in Configs/, scripts call `tuckr add`, `tuckr rm`, may have Hooks.toml
- stow: .stow-local-ignore file, GNU Stow symlink farm structure
- chezmoi: dot_ prefix directories, .chezmoiignore, chezmoi commands
- yadm: ## in filenames for templates, .yadm directory
- bare-git: HEAD/objects/refs at root (bare git repo as home)
- script-based: Custom install scripts, no standard tool

## Your Task

Analyze the inputs in this order:

1. **Summarize what you see**: Describe the tree structure, what scripts are present,
   what the scripts do (based on reading their contents)

2. **Identify the dotfile system**: Based on evidence in the tree and scripts,
   determine which system is in use (tuckr, stow, chezmoi, etc.)

3. **Analyze the structure**: Where do groups live? What naming convention is used?
   What platforms are targeted?

4. **Make observations**: What's notable about this repository?

5. **Identify warnings**: What might cause problems? (encryption, unusual patterns)

6. **Make recommendations**: What should the user do after migration?

7. **Generate execution graph**: Finally, produce the concrete rename operations needed
   to convert from legacy naming (<group>-<Platform>) to writ naming (<group>.<Platform>)

## Input

### Directory Structure (JSON)
<tree_json>

### Executable Scripts
<script_path_1>:
<script_contents_1>

<script_path_2>:
<script_contents_2>
...

## Required Output

Return valid JSON matching this schema:
{
  "analysis": {
    "source_root": "<path>",
    "system": "<tuckr|stow|chezmoi|yadm|bare-git|script-based|native>",
    "system_confidence": <0.0-1.0>,
    "input_summary": "<what you see in the inputs>",
    "structure": {
      "groups_path": "<where groups live, e.g., Home/Configs>",
      "naming_convention": "<current convention, e.g., <group>-<Platform>>",
      "groups": ["<list of group names>"],
      "platforms": ["<list of platforms>"]
    },
    "observations": ["<insights about the repository>"],
    "warnings": ["<potential issues>"],
    "recommendations": ["<suggested actions after migration>"]
  },
  "execution_graph": {
    "version": "1.0",
    "tool": "writ",
    "state": "pending",
    "nodes": [
      {"id": "<unique-id>", "operations": ["rename"], "source": "<from-path>", "target": "<to-path>", "status": "pending"}
    ],
    "edges": [
      {"from": "<node-id>", "to": "<node-id>", "relation": "orders"}
    ]
  }
}
```

## Verification

1. `writ migrate --dry-run --non-interactive ~/Workspace/Personal` should:
   - Detect system as "tuckr"
   - Show renames like `Home/Configs/all-Darwin → Home/Configs/all.Darwin`
   - Output valid JSON with both `analysis` and `execution_graph`

2. `go test ./internal/writ/migrate/...` - All tests pass

3. Build succeeds: `go build ./...`
