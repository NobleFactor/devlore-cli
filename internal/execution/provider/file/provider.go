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
// Compensable Forward methods return (string, map[string]any, error):
// the resource path, the compensation receipt, and an error. The map is
// opaque to the executor, meaningful only to the corresponding
// Compensate* Backward method.
//
//devlore:plannable
type Provider struct{}

// Link creates a symlink at path pointing to source. Idempotent: if the
// symlink already points correctly, it's a no-op (returns nil state).
//
// Parameters:
//   - source: Absolute path to the symlink target
//   - path: Absolute path where the symlink will be created
func (p *Provider) Link(source, path string) (string, map[string]any, error) {
	var state map[string]any

	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			existing, readErr := os.Readlink(path)
			if readErr == nil && existing == source {
				return path, nil, nil // Already correct — no change
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

	if err := os.Symlink(source, path); err != nil {
		return "", nil, err
	}
	return path, state, nil
}

// CompensateLink undoes a Link action using the captured state.
func (p *Provider) CompensateLink(state any) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	path, _ := s["path"].(string)
	if path == "" {
		return nil
	}
	existed, _ := s["existed_before"].(bool)
	if !existed {
		return os.Remove(path)
	}
	prevTarget, ok := s["previous_target"].(string)
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
//
// Parameters:
//   - path: Absolute path where the file will be written
//   - mode: File permission bits (e.g., 0o644)
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
func (p *Provider) CompensateCopy(state any) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	path, _ := s["path"].(string)
	if path == "" {
		return nil
	}
	existed, _ := s["existed_before"].(bool)
	if !existed {
		return os.Remove(path)
	}
	prev, ok := s["previous_content"].([]byte)
	if !ok {
		return nil // Can't restore without content
	}
	prevMode, _ := s["previous_mode"].(os.FileMode)
	if prevMode == 0 {
		prevMode = 0644
	}
	return os.WriteFile(path, prev, prevMode)
}

// Backup moves the file at path to a timestamped backup location.
// Returns the backup path and compensation state.
//
// Parameters:
//   - path: Absolute path to the file to back up
//   - backupSuffix: Suffix appended before the timestamp (default: .writ-backup)
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
func (p *Provider) CompensateBackup(state any) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	originalPath, _ := s["original_path"].(string)
	backupPath, _ := s["backup_path"].(string)
	if originalPath == "" || backupPath == "" {
		return nil
	}
	return os.Rename(backupPath, originalPath)
}

// Unlink removes a symlink at path. If prune is true and pruneBoundary
// is set, empty parent directories are removed up to the boundary.
// Returns compensation state with the symlink target for re-creation.
//
// Parameters:
//   - path: Absolute path to the symlink to remove
//   - prune: If true, remove empty parent directories after unlinking
//   - pruneBoundary: Stop pruning at this directory (prevents removing too much)
func (p *Provider) Unlink(path string, prune bool, pruneBoundary string) (string, map[string]any, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return path, nil, nil // Already gone — no change
	}
	if err != nil {
		return "", nil, err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return "", nil, fmt.Errorf("%s is not a symlink", path)
	}

	target, _ := os.Readlink(path)

	if err := os.Remove(path); err != nil {
		return "", nil, err
	}

	pruneParents(path, prune, pruneBoundary)

	state := map[string]any{
		"path":   path,
		"target": target,
	}
	return path, state, nil
}

// CompensateUnlink undoes an Unlink by re-creating the symlink.
func (p *Provider) CompensateUnlink(state any) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	path, _ := s["path"].(string)
	target, _ := s["target"].(string)
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
//
// Parameters:
//   - path: Absolute path to the file to delete
//   - prune: If true, remove empty parent directories after deletion
//   - pruneBoundary: Stop pruning at this directory (prevents removing too much)
func (p *Provider) Remove(path string, prune bool, pruneBoundary string) (string, map[string]any, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return path, nil, nil // Already gone — no change
	}
	if err != nil {
		return "", nil, err
	}

	state := map[string]any{"path": path}
	if info.Mode().IsRegular() {
		if content, readErr := os.ReadFile(path); readErr == nil {
			state["content"] = content
			state["mode"] = info.Mode().Perm()
		}
	}

	if err := os.Remove(path); err != nil {
		return "", nil, err
	}

	pruneParents(path, prune, pruneBoundary)
	return path, state, nil
}

// CompensateRemove undoes a Remove by re-creating the file with saved
// content and mode.
func (p *Provider) CompensateRemove(state any) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	path, _ := s["path"].(string)
	if path == "" {
		return nil
	}
	content, ok := s["content"].([]byte)
	if !ok {
		return nil // Can't restore without content
	}
	mode, _ := s["mode"].(os.FileMode)
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
//
// Parameters:
//   - content: String content to write to the file
//   - path: Absolute path where the file will be written
//   - mode: File permission bits (e.g., 0o644)
func (p *Provider) Write(content, path string, mode os.FileMode) (string, map[string]any, error) {
	if content == "" {
		return "", nil, fmt.Errorf("write: no content specified")
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
		return "", nil, fmt.Errorf("create parent dirs: %w", err)
	}

	if mode == 0 {
		mode = 0644
	}

	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return "", nil, err
	}
	return path, state, nil
}

// CompensateWrite undoes a Write action using the captured state.
func (p *Provider) CompensateWrite(state any) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	path, _ := s["path"].(string)
	if path == "" {
		return nil
	}
	existed, _ := s["existed_before"].(bool)
	if !existed {
		return os.Remove(path)
	}
	prev, ok := s["previous_content"].([]byte)
	if !ok {
		return nil // Can't restore without content
	}
	prevMode, _ := s["previous_mode"].(os.FileMode)
	if prevMode == 0 {
		prevMode = 0644
	}
	return os.WriteFile(path, prev, prevMode)
}

// Move moves a file from source to path. Uses gitMv if provided
// (preserves git history), falling back to os.Rename.
// Returns compensation state with paths for reverse move.
//
// Parameters:
//   - source: Absolute path to the file to move
//   - path: Absolute destination path
func (p *Provider) Move(gitMv func(src, dst string) error, source, path string) (string, map[string]any, error) {
	if _, err := os.Stat(source); err != nil {
		return "", nil, fmt.Errorf("source does not exist: %w", err)
	}

	if gitMv != nil {
		if err := gitMv(source, path); err == nil {
			return path, map[string]any{
				"source": source,
				"path":   path,
			}, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", nil, fmt.Errorf("create parent dirs: %w", err)
	}

	if err := os.Rename(source, path); err != nil {
		return "", nil, err
	}
	return path, map[string]any{
		"source": source,
		"path":   path,
	}, nil
}

// CompensateMove undoes a Move by moving the file back from path to
// source.
func (p *Provider) CompensateMove(state any) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	source, _ := s["source"].(string)
	path, _ := s["path"].(string)
	if source == "" || path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
		return err
	}
	return os.Rename(path, source)
}

// Source reads a file and returns its contents.
//
// Parameters:
//   - path: Absolute path to the file to read
func (p *Provider) Source(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Mkdir creates a directory (and parents) with the given mode.
//
// Parameters:
//   - path: Absolute path of the directory to create
//   - mode: Directory permission bits (e.g., 0o755)
func (p *Provider) Mkdir(path string, mode os.FileMode) (string, error) {
	return path, os.MkdirAll(path, mode)
}

// Exists returns true if path exists on the filesystem.
//
// Parameters:
//   - path: Absolute path to check
func (p *Provider) Exists(path string) (bool, error) {
	_, err := os.Lstat(path)
	return err == nil, nil
}

// IsDir returns true if path exists and is a directory.
//
// Parameters:
//   - path: Absolute path to check
func (p *Provider) IsDir(path string) (bool, error) {
	info, err := os.Stat(path)
	return err == nil && info.IsDir(), nil
}
