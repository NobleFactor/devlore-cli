// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Link creates a symlink from node's "path" slot pointing to "source" slot.
type Link struct{ Impl *Provider }

func (o *Link) Name() string { return "link" }

func (o *Link) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	source := node.GetSlot("source").(string)
	path := node.GetSlot("path").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] link %v %v\n", source, path)
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Link(source, path)
}

func (o *Link) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Copy writes content to node's "path" slot (consumer: reads content, checksums).
type Copy struct{ Impl *Provider }

func (o *Copy) Name() string { return "copy" }

func (o *Copy) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	path := node.GetSlot("path").(string)
	mode := node.GetMode()
	content, err := ctx.ContentFor(node)
	if err != nil {
		return nil, nil, err
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] copy %v\n", path)
		ctx.TargetChecksum = checksumBytes(content)
		return nil, nil, nil
	}
	checksum, err := o.Impl.Copy(path, mode, content)
	if err != nil {
		return nil, nil, err
	}
	ctx.TargetChecksum = checksum
	return nil, nil, nil
}

func (o *Copy) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Backup moves the existing file at node's "path" slot to a timestamped backup.
// The backup path is stored in node.Annotations["backup_path"] after execution.
type Backup struct{ Impl *Provider }

func (o *Backup) Name() string { return "backup" }

func (o *Backup) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	path := node.GetSlot("path").(string)
	backupSuffix, _ := node.GetSlot("backup_suffix").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] backup %v\n", path)
		return nil, nil, nil
	}
	backupPath, err := o.Impl.Backup(path, backupSuffix)
	if err != nil {
		return nil, nil, err
	}
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations["backup_path"] = backupPath
	return nil, nil, nil
}

func (o *Backup) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Unlink removes a symlink at node's "path" slot.
type Unlink struct{ Impl *Provider }

func (o *Unlink) Name() string { return "unlink" }

func (o *Unlink) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	path := node.GetSlot("path").(string)
	prune, _ := node.GetSlot("prune").(bool)
	pruneBoundary, _ := node.GetSlot("prune_boundary").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] unlink %v\n", path)
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Unlink(path, prune, pruneBoundary)
}

func (o *Unlink) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Remove deletes the file at node's "path" slot.
type Remove struct{ Impl *Provider }

func (o *Remove) Name() string { return "remove" }

func (o *Remove) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	path := node.GetSlot("path").(string)
	prune, _ := node.GetSlot("prune").(bool)
	pruneBoundary, _ := node.GetSlot("prune_boundary").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] remove %v\n", path)
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Remove(path, prune, pruneBoundary)
}

func (o *Remove) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Write writes content from node's "content" slot to node's "path" slot.
type Write struct{ Impl *Provider }

func (o *Write) Name() string { return "write" }

func (o *Write) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	content, _ := node.GetSlot("content").(string)
	path := node.GetSlot("path").(string)
	mode := node.GetMode()

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] write %v\n", path)
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Write(content, path, mode)
}

func (o *Write) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Move moves a file from node's "source" slot to "path" slot.
type Move struct{ Impl *Provider }

func (o *Move) Name() string { return "move" }

func (o *Move) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	source := node.GetSlot("source").(string)
	path := node.GetSlot("path").(string)
	gitMv, _ := node.GetSlot("git_mv").(func(src, dst string) error)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] move %v %v\n", source, path)
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Move(gitMv, source, path)
}

func (o *Move) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Mkdir creates a directory at node's "path" slot.
type Mkdir struct{ Impl *Provider }

func (o *Mkdir) Name() string { return "mkdir" }

func (o *Mkdir) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	path := node.GetSlot("path").(string)
	mode := node.GetMode()
	if mode == 0 {
		mode = 0755
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] mkdir %v\n", path)
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Mkdir(path, mode)
}

func (o *Mkdir) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Source reads a file and stores its content for downstream nodes.
type Source struct{ Impl *Provider }

func (o *Source) Name() string { return "source" }

func (o *Source) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	path := node.GetSlot("path").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] source %v\n", path)
		return nil, nil, nil
	}
	content, err := o.Impl.Source(path)
	if err != nil {
		return nil, nil, err
	}
	ctx.StoreContent(node, content)
	return nil, nil, nil
}

func (o *Source) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Register registers all file actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Link{Impl: p})
	reg.Register(&Copy{Impl: p})
	reg.Register(&Backup{Impl: p})
	reg.Register(&Unlink{Impl: p})
	reg.Register(&Remove{Impl: p})
	reg.Register(&Write{Impl: p})
	reg.Register(&Move{Impl: p})
	reg.Register(&Mkdir{Impl: p})
	reg.Register(&Source{Impl: p})
}
