// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"os"
	"path/filepath"
	"strings"
)

// SourceSystem identifies the dotfile management approach used in the source repository.
type SourceSystem string

const (
	SystemNative      SourceSystem = "native"       // Already writ-compatible (Home/ or System/)
	SystemTuckr       SourceSystem = "tuckr"
	SystemStow        SourceSystem = "stow"
	SystemChezmoi     SourceSystem = "chezmoi"
	SystemYadm        SourceSystem = "yadm"
	SystemBareGit     SourceSystem = "bare-git"
	SystemScriptBased SourceSystem = "script-based"
	SystemUnknown     SourceSystem = "unknown"
)

// Detect identifies the source system used in the given directory.
// It checks for tool-specific markers first, then falls back to native/unknown.
func Detect(root string) (SourceSystem, error) {
	// 1. Hooks.toml anywhere → tuckr
	if found, _ := findFile(root, "Hooks.toml"); found {
		return SystemTuckr, nil
	}

	// 2. .stow-local-ignore → stow
	if exists(filepath.Join(root, ".stow-local-ignore")) {
		return SystemStow, nil
	}

	// 3. dot_ prefixed dirs → chezmoi
	if hasDotUnderscoreDirs(root) {
		return SystemChezmoi, nil
	}

	// 4. ## in filenames → yadm
	if hasYadmTemplates(root) {
		return SystemYadm, nil
	}

	// 5. Bare git (HEAD/objects/refs at root) → bare-git
	if isBareGit(root) {
		return SystemBareGit, nil
	}

	// 6. <project>-<Platform> directory pattern with known platforms → script-based
	if hasProjectPlatformDirs(root) {
		return SystemScriptBased, nil
	}

	// 7. Home/ or System/ at root with no other tool markers → native writ
	if isNativeWrit(root) {
		return SystemNative, nil
	}

	return SystemUnknown, nil
}

// knownPlatforms are the platform values recognized in directory names.
var knownPlatforms = map[string]bool{
	"Darwin":  true,
	"Linux":   true,
	"Unix":    true,
	"Windows": true,
	"Debian":  true,
	"Ubuntu":  true,
	"Arch":    true,
}

// findFile searches recursively for a file with the given name.
func findFile(root, name string) (bool, error) {
	found := false
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.Name() == name {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found, err
}

// exists checks if a path exists.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// hasDotUnderscoreDirs checks for chezmoi-style dot_ prefixed directories.
func hasDotUnderscoreDirs(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "dot_") {
			return true
		}
	}
	return false
}

// hasYadmTemplates checks for yadm-style ## template markers in filenames.
func hasYadmTemplates(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "##") {
			return true
		}
	}
	return false
}

// isBareGit checks if the root looks like a bare git repository.
func isBareGit(root string) bool {
	return exists(filepath.Join(root, "HEAD")) &&
		exists(filepath.Join(root, "objects")) &&
		exists(filepath.Join(root, "refs"))
}

// isNativeWrit checks if the root is already writ-compatible (has Home/ or System/ directories).
func isNativeWrit(root string) bool {
	return exists(filepath.Join(root, "Home")) || exists(filepath.Join(root, "System"))
}

// hasProjectPlatformDirs checks for <project>-<Platform> directory naming.
// Returns true if at least two directories match the pattern.
func hasProjectPlatformDirs(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, platform := parseProjectPlatform(e.Name()); platform != "" {
			count++
			if count >= 2 {
				return true
			}
		}
	}
	return false
}

// parseProjectPlatform splits a directory name on the last dash that precedes
// a known platform value. Returns project and platform (empty if no platform suffix).
func parseProjectPlatform(name string) (project, platform string) {
	// Try splitting from the right on each dash
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '-' {
			candidate := name[i+1:]
			if knownPlatforms[candidate] {
				return name[:i], candidate
			}
		}
	}
	return name, ""
}

// EncryptionSystem identifies the secret encryption tool in use.
type EncryptionSystem string

const (
	EncryptGitCrypt     EncryptionSystem = "git-crypt"
	EncryptBlackbox     EncryptionSystem = "blackbox"
	EncryptTranscrypt   EncryptionSystem = "transcrypt"
	EncryptGPG          EncryptionSystem = "gpg"
	EncryptAge          EncryptionSystem = "age"
	EncryptAnsibleVault EncryptionSystem = "ansible-vault"
	EncryptSOPS         EncryptionSystem = "sops"
	EncryptNone         EncryptionSystem = "none"
)

// DetectEncryption identifies encryption systems in use in the repository.
// Returns a list of detected systems (there may be multiple).
func DetectEncryption(root string) []EncryptionSystem {
	var systems []EncryptionSystem

	// git-crypt: check .gitattributes for filter=git-crypt
	if hasGitCrypt(root) {
		systems = append(systems, EncryptGitCrypt)
	}

	// Blackbox: .blackbox directory
	if exists(filepath.Join(root, ".blackbox")) {
		systems = append(systems, EncryptBlackbox)
	}

	// transcrypt: .transcrypt directory
	if exists(filepath.Join(root, ".transcrypt")) {
		systems = append(systems, EncryptTranscrypt)
	}

	// SOPS: .sops.yaml
	if exists(filepath.Join(root, ".sops.yaml")) {
		systems = append(systems, EncryptSOPS)
	}

	if len(systems) == 0 {
		systems = append(systems, EncryptNone)
	}

	return systems
}

// hasGitCrypt checks for git-crypt configuration in .gitattributes.
func hasGitCrypt(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, ".gitattributes"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "filter=git-crypt")
}

// DetectEncryptedFile checks a single file's content for encryption signatures.
func DetectEncryptedFile(path string) EncryptionSystem {
	data, err := os.ReadFile(path)
	if err != nil {
		return EncryptNone
	}

	content := string(data)

	// Check for various encryption signatures
	if strings.HasPrefix(content, "-----BEGIN AGE ENCRYPTED FILE-----") ||
		strings.HasPrefix(content, "age-encryption.org") {
		return EncryptAge
	}

	if strings.HasPrefix(content, "-----BEGIN PGP MESSAGE-----") ||
		strings.HasPrefix(content, "-----BEGIN PGP ENCRYPTED MESSAGE-----") {
		return EncryptGPG
	}

	if strings.HasPrefix(content, "$ANSIBLE_VAULT;") {
		return EncryptAnsibleVault
	}

	// SOPS JSON/YAML has a "sops" key with metadata
	if strings.Contains(content, `"sops":`) || strings.Contains(content, "sops:") {
		if strings.Contains(content, "lastmodified") && strings.Contains(content, "mac") {
			return EncryptSOPS
		}
	}

	// git-crypt files start with \x00GITCRYPT
	if len(data) >= 9 && data[0] == 0x00 && string(data[1:9]) == "GITCRYPT" {
		return EncryptGitCrypt
	}

	return EncryptNone
}
