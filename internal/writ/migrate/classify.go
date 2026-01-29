// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"path/filepath"
	"strings"
)

// FileClass categorizes a file by its role in the environment.
type FileClass string

const (
	ClassStaticConfig    FileClass = "static-config"
	ClassLifecycleScript FileClass = "lifecycle-script"
	ClassScript          FileClass = "script"
	ClassSecret          FileClass = "secret"
	ClassTemplate        FileClass = "template"
	ClassFont            FileClass = "font"
	ClassManPage         FileClass = "man-page"
	ClassCompletion      FileClass = "completion"
	ClassBinary          FileClass = "binary"
)

// Classify assigns a FileClass to each inventory entry based on path, name,
// and file attributes. It modifies entries in place.
func Classify(entries []InventoryEntry) {
	for i := range entries {
		entries[i].Class = classifyFile(entries[i])
	}
}

func classifyFile(e InventoryEntry) FileClass {
	name := filepath.Base(e.RelPath)
	ext := strings.ToLower(filepath.Ext(name))
	relLower := strings.ToLower(e.RelPath)

	// 1. Path contains secrets directory
	if containsSecretsDir(relLower) {
		return ClassSecret
	}

	// 2. Encrypted file extensions
	if ext == ".age" || ext == ".sops" {
		return ClassSecret
	}

	// 3. Template extension
	if ext == ".template" {
		return ClassTemplate
	}

	// 4. Lifecycle scripts: Install-* or Initialize-* and executable
	if e.IsExecutable && isLifecycleScript(name) {
		return ClassLifecycleScript
	}

	// 5. Font files
	if isFont(ext) {
		return ClassFont
	}

	// 6. Man pages
	if isManPage(e.RelPath) {
		return ClassManPage
	}

	// 7. Shell completions
	if isCompletion(relLower) {
		return ClassCompletion
	}

	// 8. Executable scripts in bin/ directories
	if e.IsExecutable && inBinDir(e.RelPath) {
		return ClassScript
	}

	// 9. Binary file extensions
	if isBinary(ext) {
		return ClassBinary
	}

	// 10. Everything else
	return ClassStaticConfig
}

func containsSecretsDir(relPath string) bool {
	parts := strings.Split(relPath, string(filepath.Separator))
	for _, p := range parts {
		if strings.Contains(strings.ToLower(p), "secrets") {
			return true
		}
	}
	return false
}

func isLifecycleScript(name string) bool {
	return strings.HasPrefix(name, "Install-") || strings.HasPrefix(name, "Initialize-")
}

func isFont(ext string) bool {
	switch ext {
	case ".otf", ".ttf", ".woff", ".woff2":
		return true
	}
	return false
}

func isManPage(relPath string) bool {
	// Match paths like man/man1/foo.1 or share/man/man5/bar.5
	parts := strings.Split(relPath, string(filepath.Separator))
	for i, p := range parts {
		if strings.HasPrefix(p, "man") && i+1 < len(parts) {
			next := parts[i+1]
			if strings.HasPrefix(next, "man") && len(next) == 4 {
				return true
			}
		}
	}
	return false
}

func isCompletion(relPath string) bool {
	return strings.Contains(relPath, "bash-completion/completions/") ||
		strings.Contains(relPath, "zsh/site-functions/") ||
		strings.Contains(relPath, "fish/completions/")
}

func inBinDir(relPath string) bool {
	parts := strings.Split(relPath, string(filepath.Separator))
	for _, p := range parts {
		if p == "bin" {
			return true
		}
	}
	return false
}

func isBinary(ext string) bool {
	switch ext {
	case ".exe", ".dll", ".chm", ".msi":
		return true
	}
	return false
}

// SecretFinding represents a detected secret file.
type SecretFinding struct {
	RelPath          string           `json:"rel_path" yaml:"rel_path"`
	Encryption       EncryptionSystem `json:"encryption" yaml:"encryption"`
	Reason           string           `json:"reason" yaml:"reason"`
	SuggestedPattern string           `json:"suggested_pattern,omitempty" yaml:"suggested_pattern,omitempty"`
}
