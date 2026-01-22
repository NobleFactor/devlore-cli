// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package exec provides execution of deployment tree operations.
package exec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/template"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

// ConflictType describes the kind of conflict at a target path.
type ConflictType int

const (
	// ConflictNone indicates no conflict exists.
	ConflictNone ConflictType = iota
	// ConflictRegularFile indicates a regular file exists at target.
	ConflictRegularFile
	// ConflictDirectory indicates a directory exists at target.
	ConflictDirectory
	// ConflictForeignSymlink indicates a symlink pointing elsewhere exists.
	ConflictForeignSymlink
	// ConflictOurSymlink indicates our symlink already exists (no action needed).
	ConflictOurSymlink
)

// String returns a human-readable description of the conflict type.
func (c ConflictType) String() string {
	switch c {
	case ConflictNone:
		return "none"
	case ConflictRegularFile:
		return "file"
	case ConflictDirectory:
		return "directory"
	case ConflictForeignSymlink:
		return "foreign symlink"
	case ConflictOurSymlink:
		return "already deployed"
	default:
		return "unknown"
	}
}

// Conflict represents a pre-flight detected conflict.
type Conflict struct {
	Node         *tree.Node   `json:"node"`
	Type         ConflictType `json:"type"`
	ExistingPath string       `json:"existing_path"`          // For symlinks, where it points
	ExistingInfo os.FileInfo  `json:"-"`                      // File info of existing target
	Message      string       `json:"message"`                // Human-readable description
}

// ConflictResolution specifies how to handle conflicts.
type ConflictResolution int

const (
	// ResolutionStop aborts deployment on first conflict.
	ResolutionStop ConflictResolution = iota
	// ResolutionBackup moves conflicting files to timestamped backups.
	ResolutionBackup
	// ResolutionOverwrite removes conflicting files without backup.
	ResolutionOverwrite
	// ResolutionSkip skips conflicting files and continues.
	ResolutionSkip
)

// PreflightResult contains the results of pre-flight conflict detection.
type PreflightResult struct {
	Conflicts   []Conflict `json:"conflicts,omitempty"`
	AlreadyDone []Conflict `json:"already_done,omitempty"` // Symlinks that already point correctly
	Ready       []*tree.Node `json:"ready,omitempty"`       // Nodes ready to deploy (no conflict)
}

// Executor executes deployment tree operations.
type Executor struct {
	// DryRun prevents any file system modifications when true.
	DryRun bool

	// Force overwrites existing files without prompting.
	// Deprecated: Use ConflictResolution instead.
	Force bool

	// ConflictResolution specifies how to handle conflicts.
	// If Force is true and this is ResolutionStop, Force takes precedence (ResolutionOverwrite).
	ConflictResolution ConflictResolution

	// BackupSuffix is appended to backup filenames (default: ".writ-backup").
	// Backups are timestamped: file.writ-backup.20060102-150405
	BackupSuffix string

	// Identities for age decryption.
	Identities []age.Identity

	// TemplateData provides data for template expansion.
	// User-defined variables from config (writ.vars).
	TemplateData map[string]any

	// Segments provides platform segments for template expansion.
	// Auto-detected (OS, DISTRO, ARCH) plus custom segments.
	Segments map[string]string

	// Output for logging operations (defaults to os.Stdout).
	Output io.Writer
}

// Result represents the result of executing a single node.
type Result struct {
	Node    *tree.Node `json:"node"`
	Success bool       `json:"success"`
	Error   error      `json:"-"`
	ErrorMsg string    `json:"error,omitempty"`
	Skipped bool       `json:"skipped,omitempty"`
	Message string     `json:"message,omitempty"`

	// SourceChecksum is set for copied files (expand, decrypt, copy).
	// Format: "sha256:<hex>"
	SourceChecksum string `json:"source_checksum,omitempty"`

	// TargetChecksum is set for copied files (expand, decrypt, copy).
	// Format: "sha256:<hex>"
	TargetChecksum string `json:"target_checksum,omitempty"`
}

// DryRunOutput is the JSON structure for dry-run output.
type DryRunOutput struct {
	SourceRoot  string          `json:"source_root"`
	TargetRoot  string          `json:"target_root"`
	Projects    []string        `json:"projects"`
	MatchedDirs []string        `json:"matched_dirs"`
	Operations  []DryRunOp      `json:"operations"`
	Delegated   []DryRunOp      `json:"delegated,omitempty"`
}

// DryRunOp represents a single operation in dry-run output.
type DryRunOp struct {
	Source     string   `json:"source"`
	Target     string   `json:"target"`
	RelSource  string   `json:"rel_source"`
	RelTarget  string   `json:"rel_target"`
	Operations []string `json:"operations"`
	Mode       string   `json:"mode,omitempty"`
	Project    string   `json:"project"`
	Depth      int      `json:"depth"`
}

// Preflight performs pre-flight conflict detection without modifying anything.
// Call this before Execute to identify conflicts upfront.
func (e *Executor) Preflight(t *tree.Tree) PreflightResult {
	var result PreflightResult

	for _, node := range t.Nodes {
		// Skip delegate nodes - they don't write to target
		if node.IsDelegate() {
			result.Ready = append(result.Ready, node)
			continue
		}

		conflict := e.detectConflict(node)
		switch conflict.Type {
		case ConflictNone:
			result.Ready = append(result.Ready, node)
		case ConflictOurSymlink:
			result.AlreadyDone = append(result.AlreadyDone, conflict)
		default:
			result.Conflicts = append(result.Conflicts, conflict)
		}
	}

	return result
}

// detectConflict checks if a target path has a conflict.
func (e *Executor) detectConflict(node *tree.Node) Conflict {
	info, err := os.Lstat(node.Target)
	if os.IsNotExist(err) {
		return Conflict{Node: node, Type: ConflictNone}
	}
	if err != nil {
		return Conflict{
			Node:    node,
			Type:    ConflictRegularFile, // Treat errors as conflicts
			Message: fmt.Sprintf("cannot stat: %v", err),
		}
	}

	// Check what exists at target
	if info.IsDir() {
		return Conflict{
			Node:         node,
			Type:         ConflictDirectory,
			ExistingInfo: info,
			Message:      fmt.Sprintf("directory exists at %s", node.Target),
		}
	}

	if info.Mode()&os.ModeSymlink != 0 {
		// It's a symlink - check where it points
		linkTarget, err := os.Readlink(node.Target)
		if err != nil {
			return Conflict{
				Node:         node,
				Type:         ConflictForeignSymlink,
				ExistingInfo: info,
				Message:      fmt.Sprintf("cannot read symlink: %v", err),
			}
		}

		// Check if it points to our source
		if linkTarget == node.Source {
			return Conflict{
				Node:         node,
				Type:         ConflictOurSymlink,
				ExistingPath: linkTarget,
				ExistingInfo: info,
				Message:      "already deployed",
			}
		}

		return Conflict{
			Node:         node,
			Type:         ConflictForeignSymlink,
			ExistingPath: linkTarget,
			ExistingInfo: info,
			Message:      fmt.Sprintf("symlink exists pointing to %s", linkTarget),
		}
	}

	// Regular file
	return Conflict{
		Node:         node,
		Type:         ConflictRegularFile,
		ExistingInfo: info,
		Message:      fmt.Sprintf("file exists at %s (%d bytes)", node.Target, info.Size()),
	}
}

// Execute runs all operations for a deployment tree.
// Nodes are processed breadth-first by directory depth.
// If conflicts exist and ConflictResolution is ResolutionStop, returns error.
func (e *Executor) Execute(t *tree.Tree) ([]Result, error) {
	if e.Output == nil {
		e.Output = os.Stdout
	}
	if e.BackupSuffix == "" {
		e.BackupSuffix = ".writ-backup"
	}

	// Handle Force flag for backward compatibility
	resolution := e.ConflictResolution
	if e.Force && resolution == ResolutionStop {
		resolution = ResolutionOverwrite
	}

	// Sort nodes by depth (breadth-first)
	nodes := sortByDepth(t.Nodes)

	// In dry-run mode, output JSON
	if e.DryRun {
		return e.dryRun(t, nodes)
	}

	var results []Result

	for _, node := range nodes {
		// Detect conflict before executing
		conflict := e.detectConflict(node)

		// Handle conflict based on resolution strategy
		if conflict.Type != ConflictNone && conflict.Type != ConflictOurSymlink {
			switch resolution {
			case ResolutionStop:
				return results, fmt.Errorf("conflict at %s: %s", node.Target, conflict.Message)

			case ResolutionBackup:
				if err := e.backupFile(node.Target); err != nil {
					return results, fmt.Errorf("backup %s: %w", node.Target, err)
				}

			case ResolutionOverwrite:
				// Will be handled in executeNode

			case ResolutionSkip:
				results = append(results, Result{
					Node:    node,
					Success: true,
					Skipped: true,
					Message: fmt.Sprintf("skipped: %s", conflict.Message),
				})
				continue
			}
		}

		// Skip if already correctly deployed
		if conflict.Type == ConflictOurSymlink {
			results = append(results, Result{
				Node:    node,
				Success: true,
				Message: "already deployed",
			})
			continue
		}

		result := e.ExecuteNode(node)
		results = append(results, result)

		if result.Error != nil && resolution == ResolutionStop {
			return results, result.Error
		}
	}

	return results, nil
}

// backupFile moves a file to a timestamped backup location.
func (e *Executor) backupFile(path string) error {
	timestamp := time.Now().Format("20060102-150405")
	backupPath := path + e.BackupSuffix + "." + timestamp

	return os.Rename(path, backupPath)
}

// sortByDepth sorts nodes by directory depth (breadth-first order).
// Shallower paths are processed first.
func sortByDepth(nodes []*tree.Node) []*tree.Node {
	sorted := make([]*tree.Node, len(nodes))
	copy(sorted, nodes)

	sort.SliceStable(sorted, func(i, j int) bool {
		depthI := strings.Count(sorted[i].RelTarget, string(filepath.Separator))
		depthJ := strings.Count(sorted[j].RelTarget, string(filepath.Separator))
		return depthI < depthJ
	})

	return sorted
}

// dryRun outputs JSON describing what would be done.
func (e *Executor) dryRun(t *tree.Tree, nodes []*tree.Node) ([]Result, error) {
	var ops []DryRunOp
	var delegated []DryRunOp
	var results []Result

	for _, node := range nodes {
		depth := strings.Count(node.RelTarget, string(filepath.Separator))

		op := DryRunOp{
			Source:     node.Source,
			Target:     node.Target,
			RelSource:  node.RelSource,
			RelTarget:  node.RelTarget,
			Operations: node.Operations.Strings(),
			Project:    node.Project,
			Depth:      depth,
		}
		if node.Mode != 0 {
			op.Mode = fmt.Sprintf("%04o", node.Mode)
		}

		// Separate delegated operations
		if node.IsDelegate() {
			delegated = append(delegated, op)
		} else {
			ops = append(ops, op)
		}
		results = append(results, Result{Node: node, Success: true})
	}

	// Collect matched directory names
	var matchedDirs []string
	for _, m := range t.MatchedDirs {
		matchedDirs = append(matchedDirs, filepath.Base(m.Path))
	}

	output := DryRunOutput{
		SourceRoot:  t.SourceRoot,
		TargetRoot:  t.TargetRoot,
		Projects:    t.Projects,
		MatchedDirs: matchedDirs,
		Operations:  ops,
		Delegated:   delegated,
	}

	enc := json.NewEncoder(e.Output)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return results, fmt.Errorf("encode dry-run output: %w", err)
	}

	return results, nil
}

// ExecuteNode processes a single node through its operation pipeline.
// Exported for use by upgrade command.
func (e *Executor) ExecuteNode(node *tree.Node) Result {
	// Read source content
	content, err := os.ReadFile(node.Source)
	if err != nil {
		return Result{Node: node, Error: fmt.Errorf("read source: %w", err)}
	}

	// Track checksums for copied files
	var sourceChecksum, targetChecksum string
	isCopied := node.Operations.HasCopy()

	if isCopied {
		// Checksum the original source file
		sourceChecksum = checksumBytes(content)
	}

	// Process through operation pipeline
	for _, op := range node.Operations {
		switch op {
		case tree.OpDecrypt:
			content, err = e.decrypt(content)
			if err != nil {
				return Result{Node: node, Error: fmt.Errorf("decrypt: %w", err)}
			}

		case tree.OpExpand:
			content, err = e.expand(content, node)
			if err != nil {
				return Result{Node: node, Error: fmt.Errorf("expand: %w", err)}
			}

		case tree.OpCopy:
			if err := e.writeFile(node.Target, content, node.Mode); err != nil {
				return Result{Node: node, Error: fmt.Errorf("copy: %w", err)}
			}
			// Checksum the final content written to target
			targetChecksum = checksumBytes(content)

		case tree.OpLink:
			if err := e.createSymlink(node.Source, node.Target); err != nil {
				return Result{Node: node, Error: fmt.Errorf("link: %w", err)}
			}

		case tree.OpDelegate:
			// Delegate operations are collected but not executed by writ.
			// They are returned for lore or other tools to process.
			return Result{Node: node, Success: true, Message: "delegated to lore"}
		}
	}

	return Result{
		Node:           node,
		Success:        true,
		SourceChecksum: sourceChecksum,
		TargetChecksum: targetChecksum,
	}
}

// checksumBytes computes SHA256 of content and returns "sha256:<hex>".
func checksumBytes(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// decrypt decrypts age-encrypted content.
func (e *Executor) decrypt(content []byte) ([]byte, error) {
	if len(e.Identities) == 0 {
		return nil, fmt.Errorf("no age identities configured")
	}

	// Check if content is armored (ASCII) or binary
	reader := bytes.NewReader(content)
	var decReader io.Reader

	// Try armored first (age files are typically armored)
	armoredReader := armor.NewReader(reader)
	if _, err := armoredReader.Read(make([]byte, 1)); err == nil {
		// Reset and use armored reader
		reader.Reset(content)
		decReader = armor.NewReader(reader)
	} else {
		// Use binary reader
		reader.Reset(content)
		decReader = reader
	}

	decrypted, err := age.Decrypt(decReader, e.Identities...)
	if err != nil {
		return nil, err
	}

	return io.ReadAll(decrypted)
}

// expand processes template content with Go text/template.
func (e *Executor) expand(content []byte, node *tree.Node) ([]byte, error) {
	tmpl, err := template.New(node.RelSource).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	// Build template data with builtins, then user overrides
	data := e.builtinTemplateData()

	// User-defined variables override builtins
	for k, v := range e.TemplateData {
		data[k] = v
	}

	// Add node-specific data (highest priority)
	data["Source"] = node.Source
	data["Target"] = node.Target
	data["Project"] = node.Project

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	return buf.Bytes(), nil
}

// builtinTemplateData returns auto-detected platform and environment variables.
// These can be overridden by user-defined variables in writ.vars config.
func (e *Executor) builtinTemplateData() map[string]any {
	data := make(map[string]any)

	// Platform detection (from segments if available, otherwise runtime)
	if e.Segments != nil {
		for k, v := range e.Segments {
			data[k] = v
		}
	} else {
		// Fallback to runtime detection
		data["OS"] = capitalizeOS(runtime.GOOS)
		data["ARCH"] = runtime.GOARCH
	}

	// Hostname
	if hostname, err := os.Hostname(); err == nil {
		data["Hostname"] = hostname
		// ShortHostname: strip domain suffix
		if idx := strings.Index(hostname, "."); idx > 0 {
			data["ShortHostname"] = hostname[:idx]
		} else {
			data["ShortHostname"] = hostname
		}
	}

	// User information
	data["Username"] = os.Getenv("USER")
	data["Home"] = os.Getenv("HOME")

	// Full name from system (best effort)
	if u, err := user.Current(); err == nil {
		data["UID"] = u.Uid
		data["GID"] = u.Gid
		if u.Name != "" {
			data["FullName"] = u.Name
		}
	}

	// Shell
	data["Shell"] = os.Getenv("SHELL")

	// Editor preference (check common vars)
	if editor := os.Getenv("VISUAL"); editor != "" {
		data["Editor"] = editor
	} else if editor := os.Getenv("EDITOR"); editor != "" {
		data["Editor"] = editor
	}

	// XDG directories
	data["XDG_CONFIG_HOME"] = xdgDir("XDG_CONFIG_HOME", ".config")
	data["XDG_DATA_HOME"] = xdgDir("XDG_DATA_HOME", ".local/share")
	data["XDG_CACHE_HOME"] = xdgDir("XDG_CACHE_HOME", ".cache")
	data["XDG_STATE_HOME"] = xdgDir("XDG_STATE_HOME", ".local/state")

	// Useful paths
	data["ConfigDir"] = data["XDG_CONFIG_HOME"]
	data["DataDir"] = data["XDG_DATA_HOME"]
	data["CacheDir"] = data["XDG_CACHE_HOME"]

	// Environment access function
	data["Env"] = func(key string) string {
		return os.Getenv(key)
	}

	return data
}

// xdgDir returns the XDG directory, falling back to $HOME/default if unset.
func xdgDir(envVar, defaultSuffix string) string {
	if dir := os.Getenv(envVar); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), defaultSuffix)
}

// capitalizeOS converts runtime.GOOS to capitalized form for template use.
func capitalizeOS(goos string) string {
	switch goos {
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		if len(goos) > 0 {
			return strings.ToUpper(goos[:1]) + goos[1:]
		}
		return goos
	}
}

// writeFile writes content to a file, creating parent directories as needed.
// Conflicts should be handled before calling this method (via Preflight/Execute).
func (e *Executor) writeFile(path string, content []byte, mode os.FileMode) error {
	// Remove existing file/symlink if present (conflict was already handled)
	if _, err := os.Lstat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove existing: %w", err)
		}
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	// Determine file mode
	if mode == 0 {
		mode = 0644
	}

	return os.WriteFile(path, content, mode)
}

// createSymlink creates a symlink from target pointing to source.
// Conflicts should be handled before calling this method (via Preflight/Execute).
func (e *Executor) createSymlink(source, target string) error {
	// Check if symlink already points correctly (idempotent)
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			existing, err := os.Readlink(target)
			if err == nil && existing == source {
				return nil // Already correct
			}
		}
		// Remove existing (conflict was already handled via backup/overwrite decision)
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("remove existing: %w", err)
		}
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	return os.Symlink(source, target)
}

// DelegatedNodes returns nodes that need to be processed by lore.
func DelegatedNodes(results []Result) []*tree.Node {
	var nodes []*tree.Node
	for _, r := range results {
		if r.Node != nil && r.Node.IsDelegate() {
			nodes = append(nodes, r.Node)
		}
	}
	return nodes
}
