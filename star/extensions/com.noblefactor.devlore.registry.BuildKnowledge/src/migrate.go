// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// MigrateKnowledgeResult is the JSON response for parse_migrate_knowledge.
type MigrateKnowledgeResult struct {
	SourceSystems     []TypeConstantEntry `json:"source_systems"`
	EncryptionSystems []TypeConstantEntry `json:"encryption_systems"`
	RepoLayers        []TypeConstantEntry `json:"repo_layers"`
	Platforms         []string            `json:"platforms"`
	SystemPrompt      string              `json:"system_prompt"`
}

// TypeConstantEntry represents a typed const declaration.
type TypeConstantEntry struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	TypeName string `json:"type_name"`
	Line     int    `json:"line"`
	File     string `json:"file"`
}

// parseMigrateKnowledge parses migration constants from Go source.
// Port of GoReceiver.parseMigrateKnowledge from noblefactor-ops.
func parseMigrateKnowledge(path string) (*MigrateKnowledgeResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("parse_migrate_knowledge: path must be a directory")
	}

	result := &MigrateKnowledgeResult{
		SourceSystems:     []TypeConstantEntry{},
		EncryptionSystems: []TypeConstantEntry{},
		RepoLayers:        []TypeConstantEntry{},
		Platforms:          []string{},
	}

	analysisPath := filepath.Join(path, "analysis.go")
	if _, err := os.Stat(analysisPath); err == nil {
		if err := parseAnalysisFile(analysisPath, result); err != nil {
			return nil, fmt.Errorf("parse_migrate_knowledge: parsing analysis.go: %w", err)
		}
	}

	planPath := filepath.Join(path, "plan.go")
	if _, err := os.Stat(planPath); err == nil {
		if err := parsePlanFile(planPath, result); err != nil {
			return nil, fmt.Errorf("parse_migrate_knowledge: parsing plan.go: %w", err)
		}
	}

	return result, nil
}

func parseAnalysisFile(path string, result *MigrateKnowledgeResult) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	filename := filepath.Base(path)

	ast.Inspect(node, func(n ast.Node) bool {
		genDecl, ok := n.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			return true
		}

		var currentType string
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if valueSpec.Type != nil {
				if ident, ok := valueSpec.Type.(*ast.Ident); ok {
					currentType = ident.Name
				}
			}
			for i, name := range valueSpec.Names {
				if i >= len(valueSpec.Values) {
					continue
				}
				basicLit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok || basicLit.Kind != token.STRING {
					continue
				}
				tc := TypeConstantEntry{
					Name:     name.Name,
					Value:    strings.Trim(basicLit.Value, `"`),
					TypeName: currentType,
					Line:     fset.Position(name.Pos()).Line,
					File:     filename,
				}
				switch currentType {
				case "SourceSystem":
					result.SourceSystems = append(result.SourceSystems, tc)
				case "EncryptionSystem":
					result.EncryptionSystems = append(result.EncryptionSystems, tc)
				case "RepoLayer":
					result.RepoLayers = append(result.RepoLayers, tc)
				}
			}
		}
		return true
	})

	return nil
}

func parsePlanFile(path string, result *MigrateKnowledgeResult) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	contentStr := string(content)

	promptStart := strings.Index(contentStr, "func buildSystemPrompt() string {")
	if promptStart == -1 {
		return nil
	}

	searchStart := promptStart
	backtickStart := strings.Index(contentStr[searchStart:], "`")
	if backtickStart == -1 {
		return nil
	}
	backtickStart += searchStart

	backtickEnd := strings.Index(contentStr[backtickStart+1:], "`")
	if backtickEnd == -1 {
		return nil
	}
	backtickEnd += backtickStart + 1

	result.SystemPrompt = contentStr[backtickStart+1 : backtickEnd]

	lines := strings.Split(result.SystemPrompt, "\n")
	inPlatformSection := false
	for i, line := range lines {
		if strings.Contains(line, "Known platforms:") {
			colonIdx := strings.Index(line, ":")
			if colonIdx != -1 {
				platformStr := strings.TrimSpace(line[colonIdx+1:])
				if platformStr != "" {
					for _, p := range strings.Split(platformStr, ",") {
						p = strings.TrimSpace(p)
						if p != "" {
							result.Platforms = append(result.Platforms, p)
						}
					}
					break
				}
			}
			inPlatformSection = true
			continue
		}

		if inPlatformSection {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") {
				entry := strings.TrimPrefix(trimmed, "- ")
				platform := entry
				if spaceIdx := strings.Index(entry, " "); spaceIdx > 0 {
					platform = entry[:spaceIdx]
				}
				if parenIdx := strings.Index(platform, "("); parenIdx > 0 {
					platform = platform[:parenIdx]
				}
				platform = strings.TrimSpace(platform)
				if platform != "" {
					result.Platforms = append(result.Platforms, platform)
				}
			} else if trimmed == "" || !strings.HasPrefix(trimmed, "-") {
				if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "##") {
					inPlatformSection = false
				} else if trimmed != "" && !strings.HasPrefix(trimmed, "-") {
					inPlatformSection = false
				}
			}
		}
	}

	return nil
}
