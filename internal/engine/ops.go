// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package engine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

// LinkOp creates a symlink from node.Target pointing to node.Source.
type LinkOp struct{}

func (o *LinkOp) Name() string { return "link" }

func (o *LinkOp) Execute(ctx *Context, node *Node, state *PipelineState) error {
	if ctx.DryRun {
		return nil
	}

	// Idempotent: check if symlink already points correctly
	if info, err := os.Lstat(node.Target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			existing, err := os.Readlink(node.Target)
			if err == nil && existing == node.Source {
				return nil // Already correct
			}
		}
		// Remove existing (conflict should have been handled by preflight)
		if err := os.Remove(node.Target); err != nil {
			return fmt.Errorf("remove existing: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(node.Target), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	return os.Symlink(node.Source, node.Target)
}

// CopyOp writes state.Content to node.Target and sets state.TargetChecksum.
type CopyOp struct{}

func (o *CopyOp) Name() string { return "copy" }

func (o *CopyOp) Execute(ctx *Context, node *Node, state *PipelineState) error {
	if ctx.DryRun {
		state.TargetChecksum = checksumBytes(state.Content)
		return nil
	}

	// Remove existing file/symlink if present
	if _, err := os.Lstat(node.Target); err == nil {
		if err := os.Remove(node.Target); err != nil {
			return fmt.Errorf("remove existing: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(node.Target), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	mode := node.Mode
	if mode == 0 {
		mode = 0644
	}

	if err := os.WriteFile(node.Target, state.Content, mode); err != nil {
		return err
	}

	state.TargetChecksum = checksumBytes(state.Content)
	return nil
}

// ExpandOp processes state.Content as a Go text/template with ctx.Data as
// the template data. The expanded result replaces state.Content.
type ExpandOp struct{}

func (o *ExpandOp) Name() string { return "expand" }

func (o *ExpandOp) Execute(ctx *Context, node *Node, state *PipelineState) error {
	tmpl, err := template.New(node.ID).Parse(string(state.Content))
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	// Build template data from context
	data := make(map[string]any)
	for k, v := range ctx.Data {
		data[k] = v
	}
	// Add node-specific data
	data["Source"] = node.Source
	data["Target"] = node.Target
	data["Project"] = node.Project

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	state.Content = buf.Bytes()
	return nil
}

// DecryptOp decrypts state.Content using the SOPS API. The decryption
// configuration is expected in ctx.Data. The decrypted result replaces
// state.Content.
type DecryptOp struct{}

func (o *DecryptOp) Name() string { return "decrypt" }

func (o *DecryptOp) Execute(ctx *Context, node *Node, state *PipelineState) error {
	decryptor, ok := ctx.Data["decryptor"]
	if !ok {
		return fmt.Errorf("no decryptor configured in context")
	}

	// The decryptor is a function that takes encrypted bytes and returns plaintext.
	// This allows tools to provide their own decryption implementation
	// (SOPS, age, or any other backend) without the engine depending on
	// specific crypto libraries.
	decrypt, ok := decryptor.(func([]byte) ([]byte, error))
	if !ok {
		return fmt.Errorf("decryptor is not func([]byte) ([]byte, error)")
	}

	plaintext, err := decrypt(state.Content)
	if err != nil {
		return err
	}

	state.Content = plaintext
	return nil
}

// DelegateOp is a no-op that marks a node for cross-tool handoff.
type DelegateOp struct{}

func (o *DelegateOp) Name() string { return "delegate" }

func (o *DelegateOp) Execute(_ *Context, node *Node, state *PipelineState) error {
	state.Metadata["delegate_to"] = node.DelegateTo
	return nil
}

// BackupOp moves the existing file at node.Target to a timestamped backup.
type BackupOp struct{}

func (o *BackupOp) Name() string { return "backup" }

func (o *BackupOp) Execute(ctx *Context, node *Node, state *PipelineState) error {
	if ctx.DryRun {
		return nil
	}

	suffix := ".writ-backup"
	if s, ok := ctx.Data["backup_suffix"]; ok {
		if str, ok := s.(string); ok {
			suffix = str
		}
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := node.Target + suffix + "." + timestamp

	if err := os.Rename(node.Target, backupPath); err != nil {
		return fmt.Errorf("backup %s → %s: %w", node.Target, backupPath, err)
	}

	state.Metadata["backup_path"] = backupPath
	return nil
}

// UnlinkOp removes a symlink at node.Target.
type UnlinkOp struct{}

func (o *UnlinkOp) Name() string { return "unlink" }

func (o *UnlinkOp) Execute(ctx *Context, node *Node, _ *PipelineState) error {
	if ctx.DryRun {
		return nil
	}

	info, err := os.Lstat(node.Target)
	if os.IsNotExist(err) {
		return nil // Already gone
	}
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s is not a symlink", node.Target)
	}

	return os.Remove(node.Target)
}

// RemoveOp deletes the file at node.Target.
type RemoveOp struct{}

func (o *RemoveOp) Name() string { return "remove" }

func (o *RemoveOp) Execute(ctx *Context, node *Node, _ *PipelineState) error {
	if ctx.DryRun {
		return nil
	}

	if _, err := os.Lstat(node.Target); os.IsNotExist(err) {
		return nil // Already gone
	}

	return os.Remove(node.Target)
}

// FileOps returns all built-in file operations for registration.
func FileOps() []Operation {
	return []Operation{
		&LinkOp{},
		&CopyOp{},
		&ExpandOp{},
		&DecryptOp{},
		&DelegateOp{},
		&BackupOp{},
		&UnlinkOp{},
		&RemoveOp{},
	}
}
