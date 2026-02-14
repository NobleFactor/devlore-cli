// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Code generated from gen-receiver templates; DO NOT EDIT.

package execution

import "fmt"

// FileLinkOp creates a symlink from node's "path" slot pointing to "source" slot.
type FileLinkOp struct{ impl *FileService }

func (o *FileLinkOp) Name() string { return "link" }

func (o *FileLinkOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	source := node.GetSlot("source").(string)
	path := node.GetSlot("path").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] link %v %v\n", source, path)
		return nil, nil, nil
	}
	return nil, nil, o.impl.Link(source, path)
}

func (o *FileLinkOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// FileCopyOp writes content to node's "path" slot (consumer: reads content, checksums).
type FileCopyOp struct{ impl *FileService }

func (o *FileCopyOp) Name() string { return "copy" }

func (o *FileCopyOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	path := node.GetSlot("path").(string)
	mode := node.GetMode()
	content, err := ctx.ContentFor(node)
	if err != nil {
		return nil, nil, err
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] copy %v\n", path)
		ctx.TargetChecksum = ChecksumBytes(content)
		return nil, nil, nil
	}
	checksum, err := o.impl.Copy(path, mode, content)
	if err != nil {
		return nil, nil, err
	}
	ctx.TargetChecksum = checksum
	return nil, nil, nil
}

func (o *FileCopyOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// FileRenderOp processes content as a Go text/template (transformer: reads content, stores result).
type FileRenderOp struct{ impl *FileService }

func (o *FileRenderOp) Name() string { return "render" }

func (o *FileRenderOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	source, _ := node.GetSlot("source").(string)
	path, _ := node.GetSlot("path").(string)
	project := node.GetProject()
	templateData := make(map[string]any)
	for k, v := range ctx.Data {
		templateData[k] = v
	}
	content, err := ctx.ContentFor(node)
	if err != nil {
		return nil, nil, err
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] render %v %v\n", source, path)
		return nil, nil, nil
	}
	result, err := o.impl.Render(templateData, source, path, project, content)
	if err != nil {
		return nil, nil, err
	}
	ctx.StoreContent(node, result)
	return nil, nil, nil
}

func (o *FileRenderOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// FileBackupOp moves the existing file at node's "path" slot to a timestamped backup.
// The backup path is stored in node.Annotations["backup_path"] after execution.
type FileBackupOp struct{ impl *FileService }

func (o *FileBackupOp) Name() string { return "backup" }

func (o *FileBackupOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	path := node.GetSlot("path").(string)
	backupSuffix, _ := node.GetSlot("backup_suffix").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] backup %v\n", path)
		return nil, nil, nil
	}
	backupPath, err := o.impl.Backup(path, backupSuffix)
	if err != nil {
		return nil, nil, err
	}
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations["backup_path"] = backupPath
	return nil, nil, nil
}

func (o *FileBackupOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// FileUnlinkOp removes a symlink at node's "path" slot.
type FileUnlinkOp struct{ impl *FileService }

func (o *FileUnlinkOp) Name() string { return "unlink" }

func (o *FileUnlinkOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	path := node.GetSlot("path").(string)
	prune, _ := node.GetSlot("prune").(bool)
	pruneBoundary, _ := node.GetSlot("prune_boundary").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] unlink %v\n", path)
		return nil, nil, nil
	}
	return nil, nil, o.impl.Unlink(path, prune, pruneBoundary)
}

func (o *FileUnlinkOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// FileRemoveOp deletes the file at node's "path" slot.
type FileRemoveOp struct{ impl *FileService }

func (o *FileRemoveOp) Name() string { return "remove" }

func (o *FileRemoveOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	path := node.GetSlot("path").(string)
	prune, _ := node.GetSlot("prune").(bool)
	pruneBoundary, _ := node.GetSlot("prune_boundary").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] remove %v\n", path)
		return nil, nil, nil
	}
	return nil, nil, o.impl.Remove(path, prune, pruneBoundary)
}

func (o *FileRemoveOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// FileWriteOp writes content from node's "content" slot to node's "path" slot.
type FileWriteOp struct{ impl *FileService }

func (o *FileWriteOp) Name() string { return "write" }

func (o *FileWriteOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	content, _ := node.GetSlot("content").(string)
	path := node.GetSlot("path").(string)
	mode := node.GetMode()

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] write %v\n", path)
		return nil, nil, nil
	}
	return nil, nil, o.impl.Write(content, path, mode)
}

func (o *FileWriteOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// FileMoveOp moves a file from node's "source" slot to "path" slot.
type FileMoveOp struct{ impl *FileService }

func (o *FileMoveOp) Name() string { return "move" }

func (o *FileMoveOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	source := node.GetSlot("source").(string)
	path := node.GetSlot("path").(string)
	gitMv, _ := node.GetSlot("git_mv").(func(src, dst string) error)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] move %v %v\n", source, path)
		return nil, nil, nil
	}
	return nil, nil, o.impl.Move(gitMv, source, path)
}

func (o *FileMoveOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// FileOps returns all file actions backed by the given FileService.
func FileOps(impl *FileService) []Action {
	return []Action{
		&FileLinkOp{impl: impl},
		&FileCopyOp{impl: impl},
		&FileRenderOp{impl: impl},
		&FileBackupOp{impl: impl},
		&FileUnlinkOp{impl: impl},
		&FileRemoveOp{impl: impl},
		&FileWriteOp{impl: impl},
		&FileMoveOp{impl: impl},
	}
}
