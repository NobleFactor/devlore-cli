// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/internal/model"
)

// Options controls migration behavior.
type Options struct {
	SourceRoot   string
	TargetRoot   string // empty = rename in place
	Execute      bool
	Verbose      bool
	Format       string // "json" (default), "yaml", "text"
	Provider     model.Provider
	RegClient    *lorepackage.Registry
	TreeDepth    int // max directory depth to scan (0 = auto based on provider)
	ScriptBudget int // max bytes of script content to include (0 = auto based on provider)
}

// InputLimits holds computed limits for input gathering.
type InputLimits struct {
	TreeDepth    int `yaml:"tree_depth" json:"tree_depth"`
	ScriptBudget int `yaml:"script_budget" json:"script_budget"`
}

// ProviderConfig represents the providers.yaml structure from devlore-registry.
type ProviderConfig struct {
	ModelCache string                  `yaml:"model_cache"` // Path to litellm-cache.json
	Providers  map[string]ProviderInfo `yaml:"providers"`
}

// ProviderInfo holds configuration for a specific provider.
type ProviderInfo struct {
	Description      string      `yaml:"description"`
	LiteLLMProvider  string      `yaml:"litellm_provider"`   // Maps to litellm_provider in cache
	MaxInputOverride int         `yaml:"max_input_override"` // Provider-enforced limit (e.g., GitHub)
	MaxOutputOverride int        `yaml:"max_output_override"`
	InputLimits      InputLimits `yaml:"input_limits"` // Default limits if model not in cache
}

// ModelCache represents the litellm-cache.json structure.
type ModelCache struct {
	Meta   ModelCacheMeta          `json:"_meta"`
	Models map[string]ModelLimits  `json:"models"`
}

// ModelCacheMeta contains metadata about the cache.
type ModelCacheMeta struct {
	Source     string `json:"source"`
	SourceFile string `json:"source_file"`
	FetchedAt  string `json:"fetched_at"`
}

// ModelLimits holds limits for a specific model from LiteLLM.
type ModelLimits struct {
	MaxInputTokens  int    `json:"max_input_tokens"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	LiteLLMProvider string `json:"litellm_provider"`
}

// LoadInputLimits reads provider and model limits from the registry.
//
// Resolution order:
// 1. Look up model in litellm-cache.json, compute limits from max_input_tokens
// 2. Apply provider overrides (e.g., GitHub's rate limits)
// 3. Fall back to provider's default input_limits if model not found
func LoadInputLimits(reg *lorepackage.Registry, provider model.Provider) (InputLimits, error) {
	if reg == nil {
		return InputLimits{}, fmt.Errorf("registry required for provider limits")
	}
	if provider == nil {
		return InputLimits{}, fmt.Errorf("provider required for input limits")
	}

	providerName := strings.ToLower(provider.Name())
	modelName := provider.Model()

	// Load provider config
	configData, err := reg.Knowledge("shared").Providers("providers.yaml")
	if err != nil {
		return InputLimits{}, fmt.Errorf("reading providers.yaml: %w", err)
	}

	var config ProviderConfig
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return InputLimits{}, fmt.Errorf("parsing providers.yaml: %w", err)
	}

	providerInfo, ok := config.Providers[providerName]
	if !ok {
		return InputLimits{}, fmt.Errorf("provider %q not defined in providers.yaml; add it to devlore-registry", providerName)
	}

	// Try to load model-specific limits from cache
	if config.ModelCache != "" {
		cacheData, err := reg.Knowledge("shared").Providers(config.ModelCache)
		if err == nil {
			var cache ModelCache
			if err := json.Unmarshal(cacheData, &cache); err == nil {
				if limits, ok := computeLimitsFromCache(&cache, &providerInfo, modelName); ok {
					return limits, nil
				}
			}
		}
	}

	// Fall back to provider defaults
	if providerInfo.InputLimits.TreeDepth <= 0 || providerInfo.InputLimits.ScriptBudget <= 0 {
		return InputLimits{}, fmt.Errorf("provider %q has invalid input_limits in providers.yaml", providerName)
	}

	return providerInfo.InputLimits, nil
}

// computeLimitsFromCache looks up the model and computes input limits.
func computeLimitsFromCache(cache *ModelCache, providerInfo *ProviderInfo, modelName string) (InputLimits, bool) {
	modelLimits, ok := cache.Models[modelName]
	if !ok {
		return InputLimits{}, false
	}

	maxInput := modelLimits.MaxInputTokens

	// Apply provider override if set (e.g., GitHub enforces lower limits)
	if providerInfo.MaxInputOverride > 0 && providerInfo.MaxInputOverride < maxInput {
		maxInput = providerInfo.MaxInputOverride
	}

	if maxInput <= 0 {
		return InputLimits{}, false
	}

	// Compute limits from token budget
	// Reserve ~50% for system prompt + response
	usableTokens := maxInput / 2

	// tree_depth scales logarithmically with context
	// 4K tokens -> depth 4, 16K -> 5, 64K -> 6, 128K -> 8, 200K -> 10
	treeDepth := 4
	switch {
	case usableTokens >= 100000:
		treeDepth = 10
	case usableTokens >= 64000:
		treeDepth = 8
	case usableTokens >= 32000:
		treeDepth = 6
	case usableTokens >= 8000:
		treeDepth = 5
	}

	// script_budget: ~25% of usable tokens, converted to bytes (4 chars/token)
	scriptBudget := (usableTokens / 4) * 4 // 25% of tokens * 4 chars/token

	return InputLimits{
		TreeDepth:    treeDepth,
		ScriptBudget: scriptBudget,
	}, true
}

// BuildMigration performs LLM-based analysis, returning an execution Graph and
// MigrationAnalysis. This is the primary API that separates executable operations
// from non-executable understanding.
//
// The LLM receives:
//   - Directory tree structure (built with Go, cross-platform)
//   - Contents of all executable scripts
//
// The LLM returns:
//   - Analysis: system detection, structure, observations, warnings, recommendations
//   - Execution Graph: rename operations for directory structure changes
func BuildMigration(ctx context.Context, opts Options) (*execution.Graph, *MigrationAnalysis, error) {
	root := opts.SourceRoot

	// Check for prior migration
	if exists(root + "/.writ-migrated") {
		return nil, nil, fmt.Errorf("already migrated (found .writ-migrated); remove it to re-run")
	}

	// Require AI provider for LLM-first analysis
	if opts.Provider == nil {
		return nil, nil, fmt.Errorf("AI provider required for migration analysis; configure with 'lore config model'")
	}

	// Compute input limits: use explicit CLI values if provided, otherwise load from registry
	treeDepth := opts.TreeDepth
	scriptBudget := opts.ScriptBudget
	if treeDepth <= 0 || scriptBudget <= 0 {
		limits, err := LoadInputLimits(opts.RegClient, opts.Provider)
		if err != nil {
			return nil, nil, fmt.Errorf("loading input limits: %w", err)
		}
		if treeDepth <= 0 {
			treeDepth = limits.TreeDepth
		}
		if scriptBudget <= 0 {
			scriptBudget = limits.ScriptBudget
		}
	}

	// Gather inputs: tree structure + executable script contents
	input, err := GatherInputs(root, treeDepth, scriptBudget)
	if err != nil {
		return nil, nil, fmt.Errorf("gather inputs: %w", err)
	}

	// LLM analysis
	result, err := AnalyzeWithLLM(ctx, opts.Provider, input)
	if err != nil {
		return nil, nil, fmt.Errorf("LLM analysis: %w", err)
	}

	return result.Graph, result.Analysis, nil
}

// LLMResult holds the parsed response from LLM analysis.
type LLMResult struct {
	Analysis *MigrationAnalysis
	Graph    *execution.Graph
}

// AnalyzeWithLLM sends gathered inputs to the LLM and parses the structured response.
func AnalyzeWithLLM(ctx context.Context, provider model.Provider, input *GatherInput) (*LLMResult, error) {
	prompt := buildSystemPrompt()
	userMessage := buildUserMessage(input)

	resp, err := provider.Chat(ctx, model.ChatRequest{
		SystemPrompt: prompt,
		Messages: []model.Message{
			{Role: model.RoleUser, Content: userMessage},
		},
		Temperature: 0,
		JSONMode:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM chat failed: %w", err)
	}

	return parseLLMResponse(resp.Content, input.Root)
}

// buildSystemPrompt creates the system prompt for LLM analysis.
func buildSystemPrompt() string {
	return `You are analyzing a dotfiles repository for migration to writ conventions.

## Writ Conventions
- Groups live in Home/Configs/ or Home/<project>/
- Naming: <group>.<Platform> (e.g., all.Darwin, noblefactor.Unix)
- NOT: <group>-<Platform> (this is the legacy convention to migrate FROM)
- Known platforms:
  - Darwin (macOS)
  - Linux (generic fallback)
  - Linux.Debian (Debian, Ubuntu, Mint - uses apt)
  - Linux.Fedora (Fedora, RHEL, CentOS - uses dnf)
  - Linux.Arch (Arch, Manjaro, EndeavourOS - uses pacman)
  - Windows
  - Unix (meta-platform: matches Darwin + Linux)

## Known Dotfile Systems
- tuckr: Groups in Configs/, scripts call "tuckr add", "tuckr rm", may have Hooks.toml
- stow: .stow-local-ignore file, GNU Stow symlink farm structure
- chezmoi: dot_ prefix directories, .chezmoiignore, chezmoi commands in scripts
- yadm: ## in filenames for templates, .yadm directory
- bare-git: HEAD/objects/refs at root (bare git repo as home)
- native: Already has Home/ with <group>.<Platform> naming
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

7. **Generate execution graph**: Produce the concrete rename operations needed
   to convert from legacy naming (<group>-<Platform>) to writ naming (<group>.<Platform>)

## Required Output

Return valid JSON matching this schema:
{
  "analysis": {
    "system": "<tuckr|stow|chezmoi|yadm|bare-git|script-based|native|unknown>",
    "system_confidence": <0.0-1.0>,
    "input_summary": "<what you see in the inputs>",
    "structure": {
      "groups_path": "<where groups live, e.g., Home/Configs>",
      "naming_convention": "<current convention, e.g., <group>-<Platform>>",
      "groups": ["<list of group names without platform suffix>"],
      "platforms": ["<list of platforms detected>"]
    },
    "repo_layer": "<base|team|personal>",
    "encryption_systems": ["<git-crypt|sops|age|gpg|none>"],
    "scripts": [
      {
        "rel_path": "<path to script>",
        "name": "<script name>",
        "phase": "<install|initialize|other>",
        "platform_guard": "<platform if guarded>",
        "line_count": <number>,
        "observations": ["<what the script does>"]
      }
    ],
    "secret_findings": [
      {
        "rel_path": "<path to secret>",
        "encryption": "<encryption system or none>",
        "reason": "<why it's a secret>",
        "suggested_pattern": "<glob pattern for .sops.yaml>"
      }
    ],
    "observations": ["<insights about the repository>"],
    "warnings": ["<potential issues>"],
    "recommendations": ["<suggested actions after migration>"]
  },
  "execution_graph": {
    "nodes": [
      {"id": "<unique-id>", "operations": ["rename"], "source": "<from-path>", "target": "<to-path>", "status": "pending"}
    ],
    "edges": [
      {"from": "<node-id>", "to": "<node-id>", "relation": "orders"}
    ]
  }
}

Important:
- Only include rename operations for directories that need to change (e.g., dash to dot separator)
- If the repository is already writ-compatible (uses dot separators), the execution_graph should have empty nodes/edges
- The source and target paths in nodes should be relative to the source root
- Chain renames with edges to ensure proper ordering (parent before child)`
}

// buildUserMessage creates the user message with gathered inputs.
func buildUserMessage(input *GatherInput) string {
	var sb strings.Builder

	sb.WriteString("Please analyze this dotfiles repository:\n\n")
	sb.WriteString("## Source Root\n")
	sb.WriteString(input.Root)
	sb.WriteString("\n\n")

	sb.WriteString("## Directory Structure\n")
	sb.WriteString(input.FormatForPrompt())

	return sb.String()
}

// llmResponse is the raw JSON structure from the LLM.
type llmResponse struct {
	Analysis       llmAnalysis       `json:"analysis"`
	ExecutionGraph llmExecutionGraph `json:"execution_graph"`
}

type llmAnalysis struct {
	System            string            `json:"system"`
	SystemConfidence  float64           `json:"system_confidence"`
	InputSummary      string            `json:"input_summary"`
	Structure         *StructureInfo    `json:"structure"`
	RepoLayer         string            `json:"repo_layer"`
	EncryptionSystems []string          `json:"encryption_systems"`
	Scripts           []llmScript       `json:"scripts"`
	SecretFindings    []llmSecret       `json:"secret_findings"`
	Observations      []string          `json:"observations"`
	Warnings          []string          `json:"warnings"`
	Recommendations   []string          `json:"recommendations"`
}

type llmScript struct {
	RelPath       string   `json:"rel_path"`
	Name          string   `json:"name"`
	Phase         string   `json:"phase"`
	PlatformGuard string   `json:"platform_guard"`
	LineCount     int      `json:"line_count"`
	Observations  []string `json:"observations"`
}

type llmSecret struct {
	RelPath          string `json:"rel_path"`
	Encryption       string `json:"encryption"`
	Reason           string `json:"reason"`
	SuggestedPattern string `json:"suggested_pattern"`
}

type llmExecutionGraph struct {
	Nodes []llmNode `json:"nodes"`
	Edges []llmEdge `json:"edges"`
}

type llmNode struct {
	ID         string   `json:"id"`
	Operations []string `json:"operations"`
	Source     string   `json:"source"`
	Target     string   `json:"target"`
	Status     string   `json:"status"`
}

type llmEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
}

// parseLLMResponse parses the LLM JSON response into our domain types.
func parseLLMResponse(content, sourceRoot string) (*LLMResult, error) {
	var resp llmResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w\nResponse: %s", err, content)
	}

	// Convert analysis
	analysis := &MigrationAnalysis{
		SourceRoot:       sourceRoot,
		System:           parseSourceSystem(resp.Analysis.System),
		SystemConfidence: resp.Analysis.SystemConfidence,
		InputSummary:     resp.Analysis.InputSummary,
		Structure:        resp.Analysis.Structure,
		RepoLayer:        parseRepoLayer(resp.Analysis.RepoLayer),
		Observations:     resp.Analysis.Observations,
		Warnings:         resp.Analysis.Warnings,
		Recommendations:  resp.Analysis.Recommendations,
	}

	// Convert encryption systems
	for _, enc := range resp.Analysis.EncryptionSystems {
		analysis.EncryptionSystems = append(analysis.EncryptionSystems, parseEncryptionSystem(enc))
	}

	// Convert scripts
	for _, s := range resp.Analysis.Scripts {
		analysis.Scripts = append(analysis.Scripts, ScriptAnalysis{
			RelPath:       s.RelPath,
			Name:          s.Name,
			Phase:         s.Phase,
			PlatformGuard: s.PlatformGuard,
			LineCount:     s.LineCount,
			Observations:  s.Observations,
		})
	}

	// Convert secret findings
	for _, sf := range resp.Analysis.SecretFindings {
		analysis.SecretFindings = append(analysis.SecretFindings, SecretFinding{
			RelPath:          sf.RelPath,
			Encryption:       parseEncryptionSystem(sf.Encryption),
			Reason:           sf.Reason,
			SuggestedPattern: sf.SuggestedPattern,
		})
	}

	// Build execution graph
	graph := buildGraphFromLLM(sourceRoot, &resp.ExecutionGraph)

	// Compute stats from graph
	analysis.Stats = computeStatsFromGraph(graph, analysis)

	return &LLMResult{
		Analysis: analysis,
		Graph:    graph,
	}, nil
}

// parseSourceSystem converts a string to SourceSystem.
func parseSourceSystem(s string) SourceSystem {
	switch strings.ToLower(s) {
	case "tuckr":
		return SystemTuckr
	case "stow":
		return SystemStow
	case "chezmoi":
		return SystemChezmoi
	case "yadm":
		return SystemYadm
	case "bare-git":
		return SystemBareGit
	case "script-based":
		return SystemScriptBased
	case "native":
		return SystemNative
	default:
		return SystemUnknown
	}
}

// parseRepoLayer converts a string to RepoLayer.
func parseRepoLayer(s string) RepoLayer {
	switch strings.ToLower(s) {
	case "base":
		return LayerBase
	case "team":
		return LayerTeam
	default:
		return LayerPersonal
	}
}

// parseEncryptionSystem converts a string to EncryptionSystem.
func parseEncryptionSystem(s string) EncryptionSystem {
	switch strings.ToLower(s) {
	case "git-crypt":
		return EncryptGitCrypt
	case "blackbox":
		return EncryptBlackbox
	case "transcrypt":
		return EncryptTranscrypt
	case "gpg":
		return EncryptGPG
	case "age":
		return EncryptAge
	case "ansible-vault":
		return EncryptAnsibleVault
	case "sops":
		return EncryptSOPS
	default:
		return EncryptNone
	}
}

// buildGraphFromLLM constructs an execution.Graph from LLM output.
func buildGraphFromLLM(sourceRoot string, llmGraph *llmExecutionGraph) *execution.Graph {
	plan := execution.NewPlan("migrate")

	// Build node ID map for edge lookup
	nodeMap := make(map[string]*execution.Node)

	for _, n := range llmGraph.Nodes {
		// Join source root with relative paths
		source := n.Source
		target := n.Target
		if sourceRoot != "" && !strings.HasPrefix(source, "/") {
			source = sourceRoot + "/" + source
			target = sourceRoot + "/" + target
		}

		var node *execution.Node
		for _, op := range n.Operations {
			switch op {
			case "rename":
				node = plan.Rename(source, target)
			}
		}
		if node != nil {
			nodeMap[n.ID] = node
		}
	}

	// Apply edges
	for _, e := range llmGraph.Edges {
		from := nodeMap[e.From]
		to := nodeMap[e.To]
		if from != nil && to != nil && e.Relation == "orders" {
			plan.DependsOn(from, to)
		}
	}

	return plan.Graph()
}

// computeStatsFromGraph computes summary statistics from the graph and analysis.
func computeStatsFromGraph(graph *execution.Graph, analysis *MigrationAnalysis) MigrationStats {
	stats := MigrationStats{
		Renames:          len(graph.Nodes),
		Scripts:          len(analysis.Scripts),
		LifecycleScripts: countLifecycleScripts(analysis.Scripts),
		Secrets:          len(analysis.SecretFindings),
	}

	// Count groups and platforms from structure
	if analysis.Structure != nil {
		stats.Projects = len(analysis.Structure.Groups)
		stats.Platforms = len(analysis.Structure.Platforms)
	}

	return stats
}

// countLifecycleScripts counts scripts with install/initialize phases.
func countLifecycleScripts(scripts []ScriptAnalysis) int {
	count := 0
	for _, s := range scripts {
		if s.Phase == "install" || s.Phase == "initialize" {
			count++
		}
	}
	return count
}

// exists checks if a path exists.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
