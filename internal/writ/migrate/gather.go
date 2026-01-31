// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TreeNode represents a directory or file in the tree structure.
// This mirrors the format produced by `tree -J` for LLM consumption.
type TreeNode struct {
	Type     string      `json:"type"`               // "directory" or "file"
	Name     string      `json:"name"`               // File/directory name
	Contents []*TreeNode `json:"contents,omitempty"` // Children (directories only)
}

// ExecutableFile holds a script's path and contents.
type ExecutableFile struct {
	Path     string `json:"path"`     // Relative path from root
	Contents string `json:"contents"` // File contents
}

// GatherInput collects tree structure and script contents for LLM analysis.
type GatherInput struct {
	Root        string           `json:"root"`        // Absolute path to source root
	Tree        *TreeNode        `json:"tree"`        // Directory structure
	Executables []ExecutableFile `json:"executables"` // Scripts with contents
}

// GatherInputs walks the directory tree and collects structure and executable contents.
// maxDepth limits how deep to recurse (0 = root only, -1 = unlimited).
// maxScriptBytes limits total script content (0 = unlimited).
func GatherInputs(root string, maxDepth int, maxScriptBytes int) (*GatherInput, error) {
	root = filepath.Clean(root)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	// Build tree structure
	tree, executables, err := buildTree(absRoot, maxDepth)
	if err != nil {
		return nil, err
	}

	// Read executable contents with budget
	execsWithContent := readExecutables(absRoot, executables, maxScriptBytes)

	return &GatherInput{
		Root:        absRoot,
		Tree:        tree,
		Executables: execsWithContent,
	}, nil
}

// buildTree walks the directory and builds the TreeNode structure.
// Returns the tree and a list of executable paths found.
func buildTree(root string, maxDepth int) (*TreeNode, []string, error) {
	var executables []string

	rootNode := &TreeNode{
		Type: "directory",
		Name: root,
	}

	// Map of directory path to node for building nested structure
	dirNodes := map[string]*TreeNode{root: rootNode}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}

		// Skip .git directory
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		// Calculate depth
		relPath, _ := filepath.Rel(root, path)
		if relPath == "." {
			return nil // Skip root itself
		}
		depth := strings.Count(relPath, string(os.PathSeparator)) + 1
		if maxDepth > 0 && depth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Get parent directory
		parentPath := filepath.Dir(path)
		parentNode, ok := dirNodes[parentPath]
		if !ok {
			return nil // Parent not tracked (shouldn't happen)
		}

		// Create node for this entry
		node := &TreeNode{
			Name: d.Name(),
		}

		if d.IsDir() {
			node.Type = "directory"
			dirNodes[path] = node
		} else {
			node.Type = "file"

			// Check if executable
			if isExecutable(path, d) {
				executables = append(executables, relPath)
			}
		}

		parentNode.Contents = append(parentNode.Contents, node)
		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	return rootNode, executables, nil
}

// isExecutable checks if a file is executable.
// On Unix: checks execute bits
// On Windows: checks for .ps1, .bat, .cmd extensions
func isExecutable(path string, d fs.DirEntry) bool {
	// Check extension first (works on all platforms)
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ps1", ".bat", ".cmd":
		return true
	}

	// Check mode bits (Unix)
	info, err := d.Info()
	if err != nil {
		return false
	}
	return info.Mode()&0111 != 0
}

// readExecutables reads script contents with prioritization and budget.
// Prioritizes Install-* and Initialize-* scripts.
func readExecutables(root string, relPaths []string, maxBytes int) []ExecutableFile {
	if len(relPaths) == 0 {
		return nil
	}

	// Sort: prioritize Install-* and Initialize-* scripts
	sort.Slice(relPaths, func(i, j int) bool {
		pi := priority(relPaths[i])
		pj := priority(relPaths[j])
		if pi != pj {
			return pi < pj // Lower priority value = higher priority
		}
		return relPaths[i] < relPaths[j]
	})

	var result []ExecutableFile
	var totalBytes int
	const maxFileBytes = 50 * 1024 // Skip files > 50KB

	for _, relPath := range relPaths {
		absPath := filepath.Join(root, relPath)

		// Check file size
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}
		if info.Size() > maxFileBytes {
			continue // Skip large files
		}

		// Check budget
		if maxBytes > 0 && totalBytes+int(info.Size()) > maxBytes {
			continue // Would exceed budget
		}

		// Read contents
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}

		// Skip binary files (simple heuristic: check for null bytes in first 512 bytes)
		if isBinary(data) {
			continue
		}

		result = append(result, ExecutableFile{
			Path:     relPath,
			Contents: string(data),
		})
		totalBytes += len(data)
	}

	return result
}

// priority returns a sort priority for a path.
// Lower values = higher priority.
func priority(path string) int {
	name := filepath.Base(path)
	nameLower := strings.ToLower(name)

	// Highest priority: root-level Install-* and Initialize-*
	if !strings.Contains(path, string(os.PathSeparator)) {
		if strings.HasPrefix(name, "Install-") {
			return 0
		}
		if strings.HasPrefix(name, "Initialize-") {
			return 1
		}
	}

	// High priority: any Install-* or Initialize-*
	if strings.HasPrefix(name, "Install-") {
		return 2
	}
	if strings.HasPrefix(name, "Initialize-") {
		return 3
	}

	// Medium priority: other lifecycle-like names
	if strings.Contains(nameLower, "setup") || strings.Contains(nameLower, "bootstrap") {
		return 4
	}

	// Default priority
	return 5
}

// isBinary checks if data looks like binary content.
func isBinary(data []byte) bool {
	checkLen := 512
	if len(data) < checkLen {
		checkLen = len(data)
	}
	for i := 0; i < checkLen; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// ToJSON serializes the GatherInput for the LLM prompt.
func (g *GatherInput) ToJSON() ([]byte, error) {
	return json.MarshalIndent(g.Tree, "", "  ")
}

// FormatForPrompt formats the gathered input for the LLM prompt.
func (g *GatherInput) FormatForPrompt() string {
	var sb strings.Builder

	// Tree structure
	sb.WriteString("### Directory Structure\n")
	treeJSON, _ := json.MarshalIndent(g.Tree, "", "  ")
	sb.WriteString("```json\n")
	sb.Write(treeJSON)
	sb.WriteString("\n```\n\n")

	// Executable scripts
	if len(g.Executables) > 0 {
		sb.WriteString("### Executable Scripts\n\n")
		for _, exec := range g.Executables {
			sb.WriteString("#### ")
			sb.WriteString(exec.Path)
			sb.WriteString("\n```\n")
			sb.WriteString(exec.Contents)
			if !strings.HasSuffix(exec.Contents, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("```\n\n")
		}
	}

	return sb.String()
}
