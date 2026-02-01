// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/model"
)

const fixtureDir = "testdata/fixture"

func fixtureRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// TestGatherInputs tests the input gathering functionality.
func TestGatherInputs(t *testing.T) {
	root := fixtureRoot(t)
	input, err := GatherInputs(root, 10, 100*1024)
	if err != nil {
		t.Fatal(err)
	}

	if input.Root != root {
		t.Errorf("GatherInputs: root = %q, want %q", input.Root, root)
	}

	if input.Tree == nil {
		t.Fatal("GatherInputs: tree is nil")
	}

	if input.Tree.Type != "directory" {
		t.Errorf("GatherInputs: tree.Type = %q, want %q", input.Tree.Type, "directory")
	}

	// Check that we found some contents
	if len(input.Tree.Contents) == 0 {
		t.Error("GatherInputs: tree has no contents")
	}

	// The fixture should have some executable scripts
	// (though permission bits may not be preserved in all test environments)
	t.Logf("Found %d executable files", len(input.Executables))
}

// TestGatherInputsDepthLimit tests that depth limit works.
func TestGatherInputsDepthLimit(t *testing.T) {
	root := fixtureRoot(t)
	inputDeep, _ := GatherInputs(root, 10, 0)
	inputShallow, _ := GatherInputs(root, 1, 0)

	// Count nodes at each level
	deepCount := countNodes(inputDeep.Tree)
	shallowCount := countNodes(inputShallow.Tree)

	if shallowCount >= deepCount && deepCount > 10 {
		t.Errorf("depth limit not working: shallow=%d >= deep=%d", shallowCount, deepCount)
	}
}

func countNodes(n *TreeNode) int {
	if n == nil {
		return 0
	}
	count := 1
	for _, child := range n.Contents {
		count += countNodes(child)
	}
	return count
}

// TestFormatForPrompt tests the prompt formatting.
func TestFormatForPrompt(t *testing.T) {
	root := fixtureRoot(t)
	input, err := GatherInputs(root, 5, 50*1024)
	if err != nil {
		t.Fatal(err)
	}

	prompt := input.FormatForPrompt()

	if !strings.Contains(prompt, "Directory Structure") {
		t.Error("prompt missing Directory Structure section")
	}

	if !strings.Contains(prompt, "```json") {
		t.Error("prompt missing JSON code block")
	}

	// If we have executables, check they're listed
	if len(input.Executables) > 0 && !strings.Contains(prompt, "Executable Scripts") {
		t.Error("prompt missing Executable Scripts section")
	}
}

// TestParseLLMResponse tests parsing of LLM responses.
func TestParseLLMResponse(t *testing.T) {
	sourceRoot := "/test/root"

	// Minimal valid response
	response := `{
		"analysis": {
			"system": "tuckr",
			"system_confidence": 0.95,
			"input_summary": "Test repository",
			"structure": {
				"groups_path": "Home/Configs",
				"naming_convention": "<group>-<Platform>",
				"groups": ["all", "noblefactor"],
				"platforms": ["Darwin", "Unix"]
			},
			"repo_layer": "personal",
			"encryption_systems": ["git-crypt"],
			"scripts": [],
			"secret_findings": [],
			"observations": ["Test observation"],
			"warnings": ["Test warning"],
			"recommendations": ["Test recommendation"]
		},
		"execution_graph": {
			"nodes": [
				{"id": "rename-1", "operations": ["rename"], "source": "Home/Configs/all-Darwin", "target": "Home/Configs/all.Darwin", "status": "pending"}
			],
			"edges": []
		}
	}`

	result, err := parseLLMResponse(response, sourceRoot)
	if err != nil {
		t.Fatalf("parseLLMResponse failed: %v", err)
	}

	// Check analysis
	if result.Analysis.System != SystemTuckr {
		t.Errorf("system = %q, want %q", result.Analysis.System, SystemTuckr)
	}
	if result.Analysis.SystemConfidence != 0.95 {
		t.Errorf("confidence = %f, want 0.95", result.Analysis.SystemConfidence)
	}
	if result.Analysis.Structure == nil {
		t.Fatal("structure is nil")
	}
	if result.Analysis.Structure.GroupsPath != "Home/Configs" {
		t.Errorf("groups_path = %q, want %q", result.Analysis.Structure.GroupsPath, "Home/Configs")
	}
	if len(result.Analysis.Observations) != 1 {
		t.Errorf("observations count = %d, want 1", len(result.Analysis.Observations))
	}
	if len(result.Analysis.EncryptionSystems) != 1 || result.Analysis.EncryptionSystems[0] != EncryptGitCrypt {
		t.Errorf("encryption_systems = %v, want [git-crypt]", result.Analysis.EncryptionSystems)
	}

	// Check graph
	if len(result.Graph.Nodes) != 1 {
		t.Errorf("graph nodes = %d, want 1", len(result.Graph.Nodes))
	}
	if len(result.Graph.Nodes) > 0 {
		node := result.Graph.Nodes[0]
		expectedSource := sourceRoot + "/Home/Configs/all-Darwin"
		if node.Source != expectedSource {
			t.Errorf("node source = %q, want %q", node.Source, expectedSource)
		}
	}
}

// TestParseSourceSystem tests source system string parsing.
func TestParseSourceSystem(t *testing.T) {
	cases := []struct {
		input string
		want  SourceSystem
	}{
		{"tuckr", SystemTuckr},
		{"Tuckr", SystemTuckr},
		{"TUCKR", SystemTuckr},
		{"stow", SystemStow},
		{"chezmoi", SystemChezmoi},
		{"yadm", SystemYadm},
		{"bare-git", SystemBareGit},
		{"script-based", SystemScriptBased},
		{"native", SystemNative},
		{"unknown", SystemUnknown},
		{"garbage", SystemUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := parseSourceSystem(tc.input)
			if got != tc.want {
				t.Errorf("parseSourceSystem(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestParseEncryptionSystem tests encryption system string parsing.
func TestParseEncryptionSystem(t *testing.T) {
	cases := []struct {
		input string
		want  EncryptionSystem
	}{
		{"git-crypt", EncryptGitCrypt},
		{"sops", EncryptSOPS},
		{"age", EncryptAge},
		{"gpg", EncryptGPG},
		{"none", EncryptNone},
		{"unknown", EncryptNone},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := parseEncryptionSystem(tc.input)
			if got != tc.want {
				t.Errorf("parseEncryptionSystem(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestFormatMigrationViewJSON tests the JSON output format.
func TestFormatMigrationViewJSON(t *testing.T) {
	analysis := &MigrationAnalysis{
		SourceRoot:       "/test/root",
		System:           SystemTuckr,
		SystemConfidence: 0.9,
		RepoLayer:        LayerPersonal,
		Observations:     []string{"obs1"},
		Stats: MigrationStats{
			Renames:   2,
			Projects:  3,
			Platforms: 2,
		},
	}

	// Create a mock graph
	graph := buildGraphFromLLM("/test/root", &llmExecutionGraph{
		Nodes: []llmNode{
			{ID: "r1", Operations: []string{"rename"}, Source: "all-Darwin", Target: "all.Darwin", Status: "pending"},
			{ID: "r2", Operations: []string{"rename"}, Source: "all-Unix", Target: "all.Unix", Status: "pending"},
		},
		Edges: []llmEdge{
			{From: "r1", To: "r2", Relation: "orders"},
		},
	})

	var buf bytes.Buffer
	if err := FormatMigrationPlan(&buf, graph, analysis, "json"); err != nil {
		t.Fatal(err)
	}

	// Parse and verify structure
	var parsed struct {
		Analysis struct {
			System     SourceSystem `json:"system"`
			SourceRoot string       `json:"source_root"`
		} `json:"analysis"`
		ExecutionGraph struct {
			Version string `json:"version"`
			Tool    string `json:"tool"`
			State   string `json:"state"`
			Nodes   []struct {
				ID         string   `json:"id"`
				Operations []string `json:"operations"`
				Source     string   `json:"source"`
				Target     string   `json:"target"`
			} `json:"nodes"`
			Edges []struct {
				From     string `json:"from"`
				To       string `json:"to"`
				Relation string `json:"relation"`
			} `json:"edges"`
		} `json:"execution_graph"`
	}

	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON parse failed: %v\nOutput: %s", err, buf.String())
	}

	if parsed.Analysis.System != SystemTuckr {
		t.Errorf("analysis.system = %q, want %q", parsed.Analysis.System, SystemTuckr)
	}
	if parsed.ExecutionGraph.Version != "1.0" {
		t.Errorf("execution_graph.version = %q, want %q", parsed.ExecutionGraph.Version, "1.0")
	}
	if parsed.ExecutionGraph.Tool != "writ" {
		t.Errorf("execution_graph.tool = %q, want %q", parsed.ExecutionGraph.Tool, "writ")
	}
	if len(parsed.ExecutionGraph.Nodes) != 2 {
		t.Errorf("execution_graph.nodes count = %d, want 2", len(parsed.ExecutionGraph.Nodes))
	}
}

// TestFormatMigrationViewYAML tests the YAML output format.
func TestFormatMigrationViewYAML(t *testing.T) {
	analysis := &MigrationAnalysis{
		SourceRoot: "/test/root",
		System:     SystemStow,
		RepoLayer:  LayerTeam,
	}

	graph := buildGraphFromLLM("/test/root", &llmExecutionGraph{
		Nodes: []llmNode{
			{ID: "r1", Operations: []string{"rename"}, Source: "pkg-Darwin", Target: "pkg.Darwin", Status: "pending"},
		},
		Edges: []llmEdge{},
	})

	var buf bytes.Buffer
	if err := FormatMigrationPlan(&buf, graph, analysis, "yaml"); err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Analysis struct {
			System SourceSystem `yaml:"system"`
		} `yaml:"analysis"`
		ExecutionGraph struct {
			Version string `yaml:"version"`
			Nodes   []struct {
				ID string `yaml:"id"`
			} `yaml:"nodes"`
		} `yaml:"execution_graph"`
	}

	if err := yaml.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("YAML parse failed: %v\nOutput: %s", err, buf.String())
	}

	if parsed.Analysis.System != SystemStow {
		t.Errorf("analysis.system = %q, want %q", parsed.Analysis.System, SystemStow)
	}
	if len(parsed.ExecutionGraph.Nodes) != 1 {
		t.Errorf("execution_graph.nodes count = %d, want 1", len(parsed.ExecutionGraph.Nodes))
	}
}

// TestExecuteWithMockGraph tests the execution with a manually created graph.
func TestExecuteWithMockGraph(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directories
	srcDir := filepath.Join(tmpDir, "all-Darwin")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	analysis := &MigrationAnalysis{
		SourceRoot: tmpDir,
		System:     SystemScriptBased,
		RepoLayer:  LayerPersonal,
	}

	graph := buildGraphFromLLM(tmpDir, &llmExecutionGraph{
		Nodes: []llmNode{
			{ID: "r1", Operations: []string{"rename"}, Source: "all-Darwin", Target: "all.Darwin", Status: "pending"},
		},
		Edges: []llmEdge{},
	})

	var buf bytes.Buffer
	if err := Execute(&buf, graph, analysis); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify rename happened
	if exists(filepath.Join(tmpDir, "all-Darwin")) {
		t.Error("source dir still exists after rename")
	}
	if !exists(filepath.Join(tmpDir, "all.Darwin")) {
		t.Error("target dir does not exist after rename")
	}
	if !exists(filepath.Join(tmpDir, "all.Darwin", "test.txt")) {
		t.Error("file not preserved after rename")
	}

	// Verify marker written
	if !exists(filepath.Join(tmpDir, ".writ-migrated")) {
		t.Error(".writ-migrated marker not written")
	}
}

// TestExecuteConflict tests that execution fails when target exists.
func TestExecuteConflict(t *testing.T) {
	tmpDir := t.TempDir()

	// Create both source and target directories
	if err := os.MkdirAll(filepath.Join(tmpDir, "all-Darwin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "all.Darwin"), 0755); err != nil {
		t.Fatal(err)
	}

	analysis := &MigrationAnalysis{
		SourceRoot: tmpDir,
		System:     SystemScriptBased,
	}

	graph := buildGraphFromLLM(tmpDir, &llmExecutionGraph{
		Nodes: []llmNode{
			{ID: "r1", Operations: []string{"rename"}, Source: "all-Darwin", Target: "all.Darwin", Status: "pending"},
		},
		Edges: []llmEdge{},
	})

	var buf bytes.Buffer
	err := Execute(&buf, graph, analysis)
	if err == nil {
		t.Fatal("Execute should fail when target exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want to contain 'already exists'", err.Error())
	}
}

// TestAlreadyMigrated tests that BuildMigration fails when already migrated.
func TestAlreadyMigrated(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a marker to simulate prior migration
	if err := os.WriteFile(filepath.Join(tmpDir, ".writ-migrated"), []byte("timestamp: now\n"), 0644); err != nil {
		t.Fatal(err)
	}

	opts := Options{SourceRoot: tmpDir}
	_, _, err := BuildMigration(nil, opts)
	if err == nil {
		t.Fatal("BuildMigration should fail when already migrated")
	}
	if !strings.Contains(err.Error(), "already migrated") {
		t.Errorf("error = %q, want to contain 'already migrated'", err.Error())
	}
}

// TestBuildMigrationRequiresProvider tests that BuildMigration requires an AI provider.
func TestBuildMigrationRequiresProvider(t *testing.T) {
	tmpDir := t.TempDir()

	opts := Options{SourceRoot: tmpDir}
	_, _, err := BuildMigration(nil, opts)
	if err == nil {
		t.Fatal("BuildMigration should fail without provider")
	}
	if !strings.Contains(err.Error(), "AI provider required") {
		t.Errorf("error = %q, want to contain 'AI provider required'", err.Error())
	}
}

// mockProvider is a minimal Provider implementation for testing.
type mockProvider struct {
	name string
}

func (m *mockProvider) Chat(_ context.Context, _ model.ChatRequest) (*model.ChatResponse, error) {
	return nil, nil
}
func (m *mockProvider) Name() string                      { return m.name }
func (m *mockProvider) Model() string                     { return "test-model" }
func (m *mockProvider) Endpoint() string                  { return "" }
func (m *mockProvider) Available(_ context.Context) bool  { return true }

func TestLoadInputLimits(t *testing.T) {
	// LoadInputLimits requires both registry and provider
	t.Run("nil-registry", func(t *testing.T) {
		p := &mockProvider{name: "github"}
		_, err := LoadInputLimits(nil, p)
		if err == nil {
			t.Error("expected error for nil registry")
		}
		if !strings.Contains(err.Error(), "registry required") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("nil-provider", func(t *testing.T) {
		// We can't easily mock the registry, so just test the nil provider case
		// Real integration tests would use an actual synced registry
		_, err := LoadInputLimits(nil, nil)
		if err == nil {
			t.Error("expected error for nil provider")
		}
	})
}
