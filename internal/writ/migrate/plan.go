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
	"github.com/NobleFactor/devlore-cli/pkg/op"

	// Blank import triggers init() in all provider packages.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider"
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
	Description       string      `yaml:"description"`
	LiteLLMProvider   string      `yaml:"litellm_provider"`   // Maps to litellm_provider in cache
	MaxInputOverride  int         `yaml:"max_input_override"` // ReceiverFactory-enforced limit (e.g., GitHub)
	MaxOutputOverride int         `yaml:"max_output_override"`
	InputLimits       InputLimits `yaml:"input_limits"` // Default limits if model not in cache
}

// ModelCache represents the litellm-cache.json structure.
type ModelCache struct {
	Meta   ModelCacheMeta         `json:"_meta"`
	Models map[string]ModelLimits `json:"models"`
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
func LoadInputLimits(reg *lorepackage.Registry, aiProvider model.Provider) (InputLimits, error) {
	if reg == nil {
		return InputLimits{}, fmt.Errorf("registry required for provider limits")
	}
	if aiProvider == nil {
		return InputLimits{}, fmt.Errorf("provider required for input limits")
	}

	providerName := strings.ToLower(aiProvider.Name())
	modelName := aiProvider.Model()

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
func BuildMigration(ctx context.Context, opts Options) (*op.Graph, *MigrationAnalysis, error) {
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

	// LLM analysis using registry prompt
	result, err := AnalyzeWithLLMFromRegistry(ctx, opts.Provider, opts.RegClient, input)
	if err != nil {
		return nil, nil, fmt.Errorf("LLM analysis: %w", err)
	}

	return result.Graph, result.Analysis, nil
}

// LLMResult holds the parsed response from LLM analysis.
type LLMResult struct {
	Analysis *MigrationAnalysis
	Graph    *op.Graph
}

// AnalyzeWithLLMFromRegistry sends gathered inputs to the LLM using registry-loaded prompt.
func AnalyzeWithLLMFromRegistry(ctx context.Context, aiProvider model.Provider, reg *lorepackage.Registry, input *GatherInput) (*LLMResult, error) {
	prompt, err := loadMigrationPrompt(reg)
	if err != nil {
		return nil, fmt.Errorf("loading migration prompt: %w", err)
	}

	userMessage := buildUserMessage(input)

	resp, err := aiProvider.Chat(ctx, model.ChatRequest{
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

	return parseRegistryLLMResponse(resp.Content, input.Root)
}

// loadMigrationPrompt loads the migration prompt from the registry.
func loadMigrationPrompt(reg *lorepackage.Registry) (string, error) {
	if reg == nil {
		return "", fmt.Errorf("registry required for prompt loading")
	}
	return reg.Knowledge("migration").Prompt("migrate-to-writ.txt")
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

// =============================================================================
// LLM Response Parsing (Registry Prompt Format)
// =============================================================================

// registryResponse is the JSON structure from the registry migration prompt.
type registryResponse struct {
	SourceSystem   string                 `json:"source_system"`
	RepoLayer      string                 `json:"repo_layer"`
	Projects       []registryProject      `json:"projects"`
	ExecutionGraph registryExecutionGraph `json:"execution_graph"`
	// Optional fields that may be included
	Warnings           []string         `json:"warnings,omitempty"`
	UnencryptedSecrets []registrySecret `json:"unencrypted_secrets,omitempty"`
}

type registryProject struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	SourceGroups []string `json:"source_groups,omitempty"`
}

type registrySecret struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Action string `json:"action,omitempty"`
}

type registryExecutionGraph struct {
	Nodes []registryNode `json:"nodes"`
	Edges []registryEdge `json:"edges"`
}

type registryNode struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	Source  string `json:"source"`
	Target  string `json:"target"`
	Project string `json:"project,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type registryEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// parseRegistryLLMResponse parses LLM output from the registry migration prompt.
func parseRegistryLLMResponse(content, sourceRoot string) (*LLMResult, error) {
	var resp registryResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("parse registry LLM response: %w\nResponse: %s", err, content)
	}

	// Convert to MigrationAnalysis
	analysis := &MigrationAnalysis{
		SourceRoot: sourceRoot,
		System:     parseSourceSystem(resp.SourceSystem),
		RepoLayer:  parseRepoLayer(resp.RepoLayer),
		Warnings:   resp.Warnings,
	}

	// Extract project names
	for _, p := range resp.Projects {
		analysis.Projects = append(analysis.Projects, p.Name)
	}

	// Convert unencrypted secrets to secret findings
	for _, s := range resp.UnencryptedSecrets {
		analysis.SecretFindings = append(analysis.SecretFindings, SecretFinding{
			RelPath:    s.Path,
			Encryption: EncryptNone,
			Reason:     s.Reason,
		})
	}

	// Build execution graph from registry format
	graph := buildGraphFromRegistry(sourceRoot, &resp.ExecutionGraph)

	// Compute stats
	analysis.Stats = computeStatsFromGraph(graph, analysis)

	return &LLMResult{
		Analysis: analysis,
		Graph:    graph,
	}, nil
}

// buildGraphFromRegistry constructs an execution.Graph from registry prompt output.
func buildGraphFromRegistry(sourceRoot string, regGraph *registryExecutionGraph) *op.Graph {
	reg := op.NewActionRegistry()
	op.InitAll(reg, op.Context{})
	plan := execution.NewPlan(reg, "migrate")
	nodeMap := make(map[string]*op.Node)

	for _, n := range regGraph.Nodes {
		source := n.Source
		target := n.Target
		if sourceRoot != "" && !strings.HasPrefix(source, "/") {
			source = sourceRoot + "/" + source
			target = sourceRoot + "/" + target
		}

		var node *op.Node
		switch n.Action {
		case "file.move":
			node = plan.Rename(source, target)
		case "file.mkdir":
			node = plan.Mkdir(target)
		case "file.copy":
			node = plan.Copy(source, target)
		case "file.remove":
			node = plan.Remove(source)
		}
		if node != nil {
			nodeMap[n.ID] = node
		}
	}

	// Apply edges
	for _, e := range regGraph.Edges {
		from := nodeMap[e.From]
		to := nodeMap[e.To]
		if from != nil && to != nil {
			plan.DependsOn(from, to)
		}
	}

	return plan.Graph()
}

// =============================================================================
// String Parsing Helpers
// =============================================================================

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

// computeStatsFromGraph computes summary statistics from the graph and analysis.
func computeStatsFromGraph(graph *op.Graph, analysis *MigrationAnalysis) MigrationStats {
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
	for i := range scripts {
		if scripts[i].Phase == "install" || scripts[i].Phase == "initialize" {
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
