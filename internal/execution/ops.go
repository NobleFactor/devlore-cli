// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

// LinkOp creates a symlink from node's "path" slot pointing to "source" slot.
type LinkOp struct{}

func (o *LinkOp) Name() string         { return "link" }
func (o *LinkOp) Category() OpCategory { return OpDirect }

func (o *LinkOp) Execute(ctx *Context, node Executable) error {
	if ctx.DryRun {
		return nil
	}

	target := node.GetSlot("path")
	source := node.GetSlot("source")

	// Idempotent: check if symlink already points correctly
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			existing, err := os.Readlink(target)
			if err == nil && existing == source {
				return nil // Already correct
			}
		}
		// Remove existing (conflict should have been handled by preflight)
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("remove existing: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	return os.Symlink(source, target)
}

// CopyOp writes content to node's "path" slot and returns the target checksum.
type CopyOp struct{}

func (o *CopyOp) Name() string         { return "copy" }
func (o *CopyOp) Category() OpCategory { return OpWriter }

func (o *CopyOp) Write(ctx *Context, node Executable, content []byte) (string, error) {
	if ctx.DryRun {
		return ChecksumBytes(content), nil
	}

	target := node.GetSlot("path")

	// Remove existing file/symlink if present
	if _, err := os.Lstat(target); err == nil {
		if err := os.Remove(target); err != nil {
			return "", fmt.Errorf("remove existing: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", fmt.Errorf("create parent dirs: %w", err)
	}

	mode := node.GetMode()
	if mode == 0 {
		mode = 0644
	}

	if err := os.WriteFile(target, content, mode); err != nil {
		return "", err
	}

	return ChecksumBytes(content), nil
}

// RenderOp processes content as a Go text/template with ctx.Data as
// the template data. Returns the rendered content.
type RenderOp struct{}

func (o *RenderOp) Name() string         { return "render" }
func (o *RenderOp) Category() OpCategory { return OpTransform }

func (o *RenderOp) Transform(ctx *Context, node Executable, content []byte) ([]byte, error) {
	tmpl, err := template.New(node.GetID()).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	// Build template data from context
	data := make(map[string]any)
	for k, v := range ctx.Data {
		data[k] = v
	}
	// Add node-specific data
	data["Source"] = node.GetSlot("source")
	data["Target"] = node.GetSlot("path")
	data["Project"] = node.GetProject()

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	return buf.Bytes(), nil
}

// DecryptOp decrypts content using the SOPS API. The decryption
// configuration is expected in ctx.Data. Returns the decrypted content.
type DecryptOp struct{}

func (o *DecryptOp) Name() string         { return "decrypt" }
func (o *DecryptOp) Category() OpCategory { return OpTransform }

func (o *DecryptOp) Transform(ctx *Context, node Executable, content []byte) ([]byte, error) {
	decryptor, ok := ctx.Data["decryptor"]
	if !ok {
		return nil, fmt.Errorf("no decryptor configured in context")
	}

	// The decryptor is a function that takes encrypted bytes and returns plaintext.
	// This allows tools to provide their own decryption implementation
	// (SOPS, age, or any other backend) without the engine depending on
	// specific crypto libraries.
	//
	// Two signatures are supported:
	//   func(source string, data []byte) ([]byte, error) — preferred, includes source path
	//   func(data []byte) ([]byte, error) — legacy, data only

	// Try new signature first (includes source path for format detection)
	if decrypt, ok := decryptor.(func(string, []byte) ([]byte, error)); ok {
		return decrypt(node.GetSlot("source"), content)
	}
	if decrypt, ok := decryptor.(func([]byte) ([]byte, error)); ok {
		// Fall back to legacy signature
		return decrypt(content)
	}

	return nil, fmt.Errorf("decryptor must be func(string, []byte) ([]byte, error) or func([]byte) ([]byte, error)")
}

// NOTE: There is no DelegateOp. writ and lore share the same execution engine.
// When writ encounters a packages-manifest.yaml, the Package Graph Builder
// (internal/lore/graph) adds package installation nodes to the execution graph.
// There is no delegation or handoff between tools.
//
// The Package Graph Builder is NOT YET IMPLEMENTED.

// BackupOp moves the existing file at node's "path" slot to a timestamped backup.
// The backup path is stored in node.Annotations["backup_path"] after execution.
type BackupOp struct{}

func (o *BackupOp) Name() string         { return "backup" }
func (o *BackupOp) Category() OpCategory { return OpDirect }

func (o *BackupOp) Execute(ctx *Context, node Executable) error {
	if ctx.DryRun {
		return nil
	}

	suffix := ".writ-backup"
	if s, ok := ctx.Data["backup_suffix"]; ok {
		if str, ok := s.(string); ok {
			suffix = str
		}
	}

	target := node.GetSlot("path")
	timestamp := time.Now().Format("20060102-150405")
	backupPath := target + suffix + "." + timestamp

	if err := os.Rename(target, backupPath); err != nil {
		return fmt.Errorf("backup %s → %s: %w", target, backupPath, err)
	}

	// Store backup path in node annotations for receipt generation
	if n, ok := node.(*Node); ok && n.Annotations == nil {
		n.Annotations = make(map[string]string)
	}
	if n, ok := node.(*Node); ok {
		n.Annotations["backup_path"] = backupPath
	}
	return nil
}

// UnlinkOp removes a symlink at node's "path" slot.
// If ctx.Data["prune_empty_dirs"] is true and ctx.Data["prune_boundary"] is set,
// empty parent directories are removed up to the boundary.
type UnlinkOp struct{}

func (o *UnlinkOp) Name() string         { return "unlink" }
func (o *UnlinkOp) Category() OpCategory { return OpDirect }

func (o *UnlinkOp) Execute(ctx *Context, node Executable) error {
	if ctx.DryRun {
		return nil
	}

	target := node.GetSlot("path")

	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil // Already gone
	}
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s is not a symlink", target)
	}

	if err := os.Remove(target); err != nil {
		return err
	}

	pruneEmptyParents(ctx, target)
	return nil
}

// RemoveOp deletes the file at node.GetSlot("path").
// If ctx.Data["prune_empty_dirs"] is true and ctx.Data["prune_boundary"] is set,
// empty parent directories are removed up to the boundary.
type RemoveOp struct{}

func (o *RemoveOp) Name() string         { return "remove" }
func (o *RemoveOp) Category() OpCategory { return OpDirect }

func (o *RemoveOp) Execute(ctx *Context, node Executable) error {
	if ctx.DryRun {
		return nil
	}

	target := node.GetSlot("path")

	if _, err := os.Lstat(target); os.IsNotExist(err) {
		return nil // Already gone
	}

	if err := os.Remove(target); err != nil {
		return err
	}

	pruneEmptyParents(ctx, target)
	return nil
}

// pruneEmptyParents removes empty parent directories up to the prune boundary.
// Requires ctx.Data["prune_empty_dirs"] = true and ctx.Data["prune_boundary"] set.
// Silently stops on any error or non-empty directory.
func pruneEmptyParents(ctx *Context, path string) {
	if ctx.Data == nil {
		return
	}
	prune, _ := ctx.Data["prune_empty_dirs"].(bool)
	if !prune {
		return
	}
	boundary, _ := ctx.Data["prune_boundary"].(string)
	if boundary == "" {
		return
	}

	// Clean paths for consistent comparison
	boundary = filepath.Clean(boundary)
	dir := filepath.Dir(path)

	for {
		// Stop at or above boundary
		if dir == boundary || !isSubpath(dir, boundary) {
			return
		}

		// Try to remove (fails if not empty)
		if err := os.Remove(dir); err != nil {
			return // Not empty or permission error
		}

		// Move up
		dir = filepath.Dir(dir)
	}
}

// isSubpath returns true if path is under parent (not equal to).
func isSubpath(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	// Must not start with ".." and must not be "."
	return rel != "." && !filepath.IsAbs(rel) && (len(rel) < 2 || rel[:2] != "..")
}

// WriteOp writes content from node's "content" slot to node's "path" slot.
// Used by plan.file.write() for writing inline content directly.
type WriteOp struct{}

func (o *WriteOp) Name() string         { return "write" }
func (o *WriteOp) Category() OpCategory { return OpDirect }

func (o *WriteOp) Execute(ctx *Context, node Executable) error {
	content := node.GetSlot("content")
	if content == "" {
		return fmt.Errorf("write: no content specified in node slots")
	}

	if ctx.DryRun {
		return nil
	}

	target := node.GetSlot("path")

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	mode := node.GetMode()
	if mode == 0 {
		mode = 0644
	}

	return os.WriteFile(target, []byte(content), mode)
}

// ValidateOp checks a precondition and fails with a message if unmet.
// The check function is retrieved from ctx.Data["validators"][node.GetSlot("check")].
type ValidateOp struct{}

func (o *ValidateOp) Name() string         { return "validate" }
func (o *ValidateOp) Category() OpCategory { return OpDirect }

func (o *ValidateOp) Execute(ctx *Context, node Executable) error {
	checkName := node.GetSlot("check")
	if checkName == "" {
		return fmt.Errorf("validate: no check specified in node slots")
	}

	validators, ok := ctx.Data["validators"].(map[string]func() error)
	if !ok {
		return fmt.Errorf("validate: no validators configured in context")
	}

	validator, ok := validators[checkName]
	if !ok {
		return fmt.Errorf("validate: unknown check %q", checkName)
	}

	if err := validator(); err != nil {
		message := node.GetSlot("message")
		if message != "" {
			return fmt.Errorf("%s: %w", message, err)
		}
		return err
	}

	return nil
}

// MoveOp moves a file or directory from node's "source" slot to "path" slot using
// git mv when inside a git repository, falling back to os.Rename otherwise.
type MoveOp struct{}

func (o *MoveOp) Name() string         { return "move" }
func (o *MoveOp) Category() OpCategory { return OpDirect }

func (o *MoveOp) Execute(ctx *Context, node Executable) error {
	if ctx.DryRun {
		return nil
	}

	source := node.GetSlot("source")
	target := node.GetSlot("path")

	// Check if source exists
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("source does not exist: %w", err)
	}

	// Try git mv first (preserves history)
	gitMv, ok := ctx.Data["git_mv"].(func(src, dst string) error)
	if ok {
		if err := gitMv(source, target); err == nil {
			return nil
		}
		// Fall through to os.Rename if git mv fails
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	return os.Rename(source, target)
}

// FileOps returns all built-in file operations for registration.
//
// Transform operations (content in → content out):
//   - decrypt: decrypts encrypted content via ctx.Data["decryptor"]
//   - render: renders Go text/template content with ctx.Data
//
// Writer operations (content in → checksum out):
//   - copy: writes content to node's "path" slot
//
// Direct operations (no content flow):
//   - link: creates symlink from "path" → "source" slots
//   - write: writes "content" slot to "path" slot
//   - backup: moves "path" slot to timestamped backup
//   - unlink: removes symlink at "path" slot
//   - remove: deletes file at "path" slot
//   - validate: checks precondition from ctx.Data["validators"]
//   - move: moves "source" → "path" slots (git mv when possible)
//
// NOTE: Package operations (package-install, package-upgrade, package-remove,
// package-update) are provided by ops_package.go.
func FileOps() []Operation {
	return []Operation{
		&LinkOp{},
		&CopyOp{},
		&RenderOp{},
		&DecryptOp{},
		&BackupOp{},
		&UnlinkOp{},
		&RemoveOp{},
		&WriteOp{},
		&ValidateOp{},
		&MoveOp{},
	}
}

// AllOps returns all operations (file + package) for registration.
// Both writ and lore should use this to ensure the same operations are available.
func AllOps() []Operation {
	ops := FileOps()
	ops = append(ops, PackageOps()...)
	return ops
}
