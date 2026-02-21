// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Provider provides file system actions. Each method receives all
// inputs as parameters — no execution context, no node access.
//
// Compensable Forward methods return (result, map[string]any, error).
// The map is the compensation receipt — opaque to the executor,
// meaningful only to the corresponding Compensate* Backward method.
//
//devlore:plannable
type Provider struct{}

// Link creates a symlink at path pointing to source. Idempotent: if the
// symlink already points correctly, it's a no-op (returns nil state).
func (p *Provider) Link(source, path string) (map[string]any, error) {
	var state map[string]any

	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			existing, readErr := os.Readlink(path)
			if readErr == nil && existing == source {
				return nil, nil // Already correct — no change
			}
			state = map[string]any{
				"path":           path,
				"existed_before": true,
			}
			if readErr == nil {
				state["previous_target"] = existing
			}
		} else {
			state = map[string]any{
				"path":           path,
				"existed_before": true,
			}
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove existing: %w", err)
		}
	} else {
		state = map[string]any{
			"path":           path,
			"existed_before": false,
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create parent dirs: %w", err)
	}

	if err := os.Symlink(source, path); err != nil {
		return nil, err
	}
	return state, nil
}

// CompensateLink undoes a Link action using the captured state.
func (p *Provider) CompensateLink(state map[string]any) error {
	path, _ := state["path"].(string)
	if path == "" {
		return nil
	}
	existed, _ := state["existed_before"].(bool)
	if !existed {
		return os.Remove(path)
	}
	prevTarget, ok := state["previous_target"].(string)
	if !ok || prevTarget == "" {
		// Was a non-symlink — can't restore, just remove the new symlink
		return os.Remove(path)
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return os.Symlink(prevTarget, path)
}

// Copy writes content to path with the given mode. Returns the SHA256
// checksum of the written content and compensation state.
func (p *Provider) Copy(path string, mode os.FileMode, content []byte) (string, map[string]any, error) {
	var state map[string]any

	if info, err := os.Lstat(path); err == nil {
		state = map[string]any{
			"path":           path,
			"existed_before": true,
		}
		if info.Mode().IsRegular() {
			if prev, readErr := os.ReadFile(path); readErr == nil {
				state["previous_content"] = prev
				state["previous_mode"] = info.Mode().Perm()
			}
		}
		if err := os.Remove(path); err != nil {
			return "", nil, fmt.Errorf("remove existing: %w", err)
		}
	} else {
		state = map[string]any{
			"path":           path,
			"existed_before": false,
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", nil, fmt.Errorf("create parent dirs: %w", err)
	}

	if mode == 0 {
		mode = 0644
	}

	if err := os.WriteFile(path, content, mode); err != nil {
		return "", nil, err
	}

	return checksumBytes(content), state, nil
}

// CompensateCopy undoes a Copy action using the captured state.
func (p *Provider) CompensateCopy(state map[string]any) error {
	path, _ := state["path"].(string)
	if path == "" {
		return nil
	}
	existed, _ := state["existed_before"].(bool)
	if !existed {
		return os.Remove(path)
	}
	prev, ok := state["previous_content"].([]byte)
	if !ok {
		return nil // Can't restore without content
	}
	prevMode, _ := state["previous_mode"].(os.FileMode)
	if prevMode == 0 {
		prevMode = 0644
	}
	return os.WriteFile(path, prev, prevMode)
}

// Backup moves the file at path to a timestamped backup location.
// Returns the backup path and compensation state.
func (p *Provider) Backup(path, backupSuffix string) (string, map[string]any, error) {
	if backupSuffix == "" {
		backupSuffix = ".writ-backup"
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := path + backupSuffix + "." + timestamp

	if err := os.Rename(path, backupPath); err != nil {
		return "", nil, fmt.Errorf("backup %s → %s: %w", path, backupPath, err)
	}

	state := map[string]any{
		"original_path": path,
		"backup_path":   backupPath,
	}
	return backupPath, state, nil
}

// CompensateBackup undoes a Backup by moving the backup back to the
// original path.
func (p *Provider) CompensateBackup(state map[string]any) error {
	originalPath, _ := state["original_path"].(string)
	backupPath, _ := state["backup_path"].(string)
	if originalPath == "" || backupPath == "" {
		return nil
	}
	return os.Rename(backupPath, originalPath)
}

// Unlink removes a symlink at path. If prune is true and pruneBoundary
// is set, empty parent directories are removed up to the boundary.
// Returns compensation state with the symlink target for re-creation.
func (p *Provider) Unlink(path string, prune bool, pruneBoundary string) (map[string]any, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil // Already gone — no change
	}
	if err != nil {
		return nil, err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return nil, fmt.Errorf("%s is not a symlink", path)
	}

	target, _ := os.Readlink(path)

	if err := os.Remove(path); err != nil {
		return nil, err
	}

	pruneParents(path, prune, pruneBoundary)

	state := map[string]any{
		"path":   path,
		"target": target,
	}
	return state, nil
}

// CompensateUnlink undoes an Unlink by re-creating the symlink.
func (p *Provider) CompensateUnlink(state map[string]any) error {
	path, _ := state["path"].(string)
	target, _ := state["target"].(string)
	if path == "" || target == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.Symlink(target, path)
}

// Remove deletes the file at path. If prune is true and pruneBoundary
// is set, empty parent directories are removed up to the boundary.
// Returns compensation state with file content for re-creation.
func (p *Provider) Remove(path string, prune bool, pruneBoundary string) (map[string]any, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil // Already gone — no change
	}
	if err != nil {
		return nil, err
	}

	state := map[string]any{"path": path}
	if info.Mode().IsRegular() {
		if content, readErr := os.ReadFile(path); readErr == nil {
			state["content"] = content
			state["mode"] = info.Mode().Perm()
		}
	}

	if err := os.Remove(path); err != nil {
		return nil, err
	}

	pruneParents(path, prune, pruneBoundary)
	return state, nil
}

// CompensateRemove undoes a Remove by re-creating the file with saved
// content and mode.
func (p *Provider) CompensateRemove(state map[string]any) error {
	path, _ := state["path"].(string)
	if path == "" {
		return nil
	}
	content, ok := state["content"].([]byte)
	if !ok {
		return nil // Can't restore without content
	}
	mode, _ := state["mode"].(os.FileMode)
	if mode == 0 {
		mode = 0644
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, mode)
}

// Write writes inline content to path with the given mode.
// Returns compensation state for undo.
func (p *Provider) Write(content, path string, mode os.FileMode) (map[string]any, error) {
	if content == "" {
		return nil, fmt.Errorf("write: no content specified")
	}

	var state map[string]any
	if info, err := os.Lstat(path); err == nil {
		state = map[string]any{
			"path":           path,
			"existed_before": true,
		}
		if info.Mode().IsRegular() {
			if prev, readErr := os.ReadFile(path); readErr == nil {
				state["previous_content"] = prev
				state["previous_mode"] = info.Mode().Perm()
			}
		}
	} else {
		state = map[string]any{
			"path":           path,
			"existed_before": false,
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create parent dirs: %w", err)
	}

	if mode == 0 {
		mode = 0644
	}

	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return nil, err
	}
	return state, nil
}

// CompensateWrite undoes a Write action using the captured state.
func (p *Provider) CompensateWrite(state map[string]any) error {
	path, _ := state["path"].(string)
	if path == "" {
		return nil
	}
	existed, _ := state["existed_before"].(bool)
	if !existed {
		return os.Remove(path)
	}
	prev, ok := state["previous_content"].([]byte)
	if !ok {
		return nil // Can't restore without content
	}
	prevMode, _ := state["previous_mode"].(os.FileMode)
	if prevMode == 0 {
		prevMode = 0644
	}
	return os.WriteFile(path, prev, prevMode)
}

// Move moves a file from source to path. Uses gitMv if provided
// (preserves git history), falling back to os.Rename.
// Returns compensation state with paths for reverse move.
func (p *Provider) Move(gitMv func(src, dst string) error, source, path string) (map[string]any, error) {
	if _, err := os.Stat(source); err != nil {
		return nil, fmt.Errorf("source does not exist: %w", err)
	}

	if gitMv != nil {
		if err := gitMv(source, path); err == nil {
			return map[string]any{
				"source": source,
				"path":   path,
			}, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create parent dirs: %w", err)
	}

	if err := os.Rename(source, path); err != nil {
		return nil, err
	}
	return map[string]any{
		"source": source,
		"path":   path,
	}, nil
}

// CompensateMove undoes a Move by moving the file back from path to
// source.
func (p *Provider) CompensateMove(state map[string]any) error {
	source, _ := state["source"].(string)
	path, _ := state["path"].(string)
	if source == "" || path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
		return err
	}
	return os.Rename(path, source)
}

// Source reads a file and returns its contents.
func (p *Provider) Source(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Mkdir creates a directory (and parents) with the given mode.
func (p *Provider) Mkdir(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}

// Exists returns true if path exists on the filesystem.
func (p *Provider) Exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// IsDir returns true if path exists and is a directory.
func (p *Provider) IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
