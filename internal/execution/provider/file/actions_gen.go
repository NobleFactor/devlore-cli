// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"fmt"
	"os"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Link creates a symlink from "path" slot pointing to "source" slot.
type Link struct{ Impl *Provider }

func (o *Link) Name() string { return "file.link" }

func (o *Link) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	source := slots["source"].(string)
	path := slots["path"].(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] link %v %v\n", source, path)
		return nil, nil, nil
	}
	state, err := o.Impl.Link(source, path)
	return nil, state, err
}

func (o *Link) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateLink(s)
}

// Copy writes content to "path" slot (consumer: reads content from slot, checksums).
type Copy struct{ Impl *Provider }

func (o *Copy) Name() string { return "file.copy" }

func (o *Copy) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	path := slots["path"].(string)
	mode, _ := slots["mode"].(os.FileMode)
	content, _ := slots["content"].([]byte)

	// If no content from upstream, read source file directly
	if content == nil {
		source, _ := slots["source"].(string)
		if source != "" {
			var err error
			content, err = os.ReadFile(source)
			if err != nil {
				return nil, nil, fmt.Errorf("read source %s: %w", source, err)
			}
			ctx.SourceChecksum = checksumBytes(content)
		}
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] copy %v\n", path)
		ctx.TargetChecksum = checksumBytes(content)
		return nil, nil, nil
	}
	checksum, state, err := o.Impl.Copy(path, mode, content)
	if err != nil {
		return nil, nil, err
	}
	ctx.TargetChecksum = checksum
	return nil, state, nil
}

func (o *Copy) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateCopy(s)
}

// Backup moves the existing file at "path" slot to a timestamped backup.
// Returns the backup path as Result.
type Backup struct{ Impl *Provider }

func (o *Backup) Name() string { return "file.backup" }

func (o *Backup) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	path := slots["path"].(string)
	backupSuffix, _ := slots["backup_suffix"].(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] backup %v\n", path)
		return nil, nil, nil
	}
	backupPath, state, err := o.Impl.Backup(path, backupSuffix)
	if err != nil {
		return nil, nil, err
	}
	return backupPath, state, nil
}

func (o *Backup) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateBackup(s)
}

// Unlink removes a symlink at "path" slot.
type Unlink struct{ Impl *Provider }

func (o *Unlink) Name() string { return "file.unlink" }

func (o *Unlink) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	path := slots["path"].(string)
	prune, _ := slots["prune"].(bool)
	pruneBoundary, _ := slots["prune_boundary"].(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] unlink %v\n", path)
		return nil, nil, nil
	}
	state, err := o.Impl.Unlink(path, prune, pruneBoundary)
	return nil, state, err
}

func (o *Unlink) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateUnlink(s)
}

// Remove deletes the file at "path" slot.
type Remove struct{ Impl *Provider }

func (o *Remove) Name() string { return "file.remove" }

func (o *Remove) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	path := slots["path"].(string)
	prune, _ := slots["prune"].(bool)
	pruneBoundary, _ := slots["prune_boundary"].(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] remove %v\n", path)
		return nil, nil, nil
	}
	state, err := o.Impl.Remove(path, prune, pruneBoundary)
	return nil, state, err
}

func (o *Remove) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateRemove(s)
}

// Write writes content from "content" slot to "path" slot.
type Write struct{ Impl *Provider }

func (o *Write) Name() string { return "file.write" }

func (o *Write) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	content, _ := slots["content"].(string)
	path := slots["path"].(string)
	mode, _ := slots["mode"].(os.FileMode)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] write %v\n", path)
		return nil, nil, nil
	}
	state, err := o.Impl.Write(content, path, mode)
	return nil, state, err
}

func (o *Write) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateWrite(s)
}

// Move moves a file from "source" slot to "path" slot.
type Move struct{ Impl *Provider }

func (o *Move) Name() string { return "file.move" }

func (o *Move) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	source := slots["source"].(string)
	path := slots["path"].(string)
	gitMv, _ := slots["git_mv"].(func(src, dst string) error)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] move %v %v\n", source, path)
		return nil, nil, nil
	}
	state, err := o.Impl.Move(gitMv, source, path)
	return nil, state, err
}

func (o *Move) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateMove(s)
}

// Mkdir creates a directory at "path" slot.
type Mkdir struct{ Impl *Provider }

func (o *Mkdir) Name() string { return "file.mkdir" }

func (o *Mkdir) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	path := slots["path"].(string)
	mode, _ := slots["mode"].(os.FileMode)
	if mode == 0 {
		mode = 0755
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] mkdir %v\n", path)
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Mkdir(path, mode)
}

func (o *Mkdir) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
	return nil
}

// Source reads a file and returns its content as Result for downstream nodes.
type Source struct{ Impl *Provider }

func (o *Source) Name() string { return "file.source" }

func (o *Source) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	path := slots["path"].(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] source %v\n", path)
		return nil, nil, nil
	}
	content, err := o.Impl.Source(path)
	if err != nil {
		return nil, nil, err
	}
	return content, nil, nil
}

func (o *Source) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
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
