// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package docgen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// skipCommands lists command names to skip when generating docs.
var skipCommands = map[string]bool{
	"help":       true,
	"completion": true,
	"man":        true,
	"version":    true,
}

// GenerateTree walks a Cobra command tree and generates a markdown file
// for each non-hidden, non-skipped command.
func GenerateTree(cmd *cobra.Command, outDir, toolName, version string) error {
	return walkTree(cmd, outDir, toolName, version)
}

func walkTree(cmd *cobra.Command, outDir, toolName, version string) error {
	if shouldSkip(cmd) {
		return nil
	}

	if err := generatePage(cmd, outDir, toolName, version); err != nil {
		return fmt.Errorf("generating %s: %w", fullCommandName(cmd), err)
	}

	for _, child := range cmd.Commands() {
		if err := walkTree(child, outDir, toolName, version); err != nil {
			return err
		}
	}

	return nil
}

func shouldSkip(cmd *cobra.Command) bool {
	if cmd.Hidden {
		return true
	}
	return skipCommands[cmd.Name()]
}

func generatePage(cmd *cobra.Command, outDir, toolName, version string) error {
	data := BuildPageData(cmd, toolName, version)

	var buf bytes.Buffer
	if err := pageTemplate.Execute(&buf, data); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	filePath := outputPath(cmd, outDir)
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	if err := os.WriteFile(filePath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", filePath, err)
	}

	fmt.Printf("  %s\n", filePath)
	return nil
}

// outputPath computes the file path for a given command.
// Root commands: outDir/writ.md
// Subcommands: outDir/writ/add.md
// Nested: outDir/writ/repo/init.md
func outputPath(cmd *cobra.Command, outDir string) string {
	parts := commandParts(cmd)

	if len(parts) == 1 {
		// Root command: writ.md
		return filepath.Join(outDir, parts[0]+".md")
	}

	// Subcommands: writ/add.md or writ/repo/init.md
	dir := filepath.Join(outDir, strings.Join(parts[:len(parts)-1], string(filepath.Separator)))
	return filepath.Join(dir, parts[len(parts)-1]+".md")
}

func commandParts(cmd *cobra.Command) []string {
	var parts []string
	for c := cmd; c != nil; c = c.Parent() {
		parts = append([]string{c.Name()}, parts...)
	}
	return parts
}
