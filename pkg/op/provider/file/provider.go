// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file/ignore"
)

// Provider provides file system actions. Each method receives all
// inputs as parameters — no execution context, no node access.
//
// Compensable Forward methods return (string, map[string]any, error):
// the resource path, the compensation receipt, and an error. The map is
// opaque to the executor, meaningful only to the corresponding
// Compensate* Backward method.
//
// +devlore:access=both
type Provider struct {
	Root string // Working directory for Glob and WalkTree
}

// ── Compensable Pairs ────────────────────────────────────────────────

// Backup moves the file at path to a timestamped backup location.
// Returns the backup path and compensation state.
//
// Parameters:
//   - path: Absolute path to the file to back up
//   - backupSuffix: Suffix appended before the timestamp (default: .writ-backup)
func (p *Provider) Backup(path, backupSuffix string) (result string, state map[string]any, err error) {
	if backupSuffix == "" {
		backupSuffix = ".writ-backup"
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := path + backupSuffix + "." + timestamp

	checksum := checksumFile(path)

	if err := os.Rename(path, backupPath); err != nil {
		return "", nil, err
	}

	state = map[string]any{
		"original_path": path,
		"backup_path":   backupPath,
	}
	if checksum != "" {
		state["written_checksum"] = checksum
	}
	return backupPath, state, nil
}

// CompensateBackup undoes a Backup by moving the backup back to the
// original path. If a written_checksum is present, the backup file is
// verified before restoring; a mismatch indicates external modification
// and the compensation is skipped.
func (p *Provider) CompensateBackup(state any) error {
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	originalPath := op.StateString(s, "original_path")
	backupPath := op.StateString(s, "backup_path")
	if originalPath == "" || backupPath == "" {
		return nil
	}

	// Verify the backup hasn't been modified since we created it.
	if expected := op.StateString(s, "written_checksum"); expected != "" {
		actual := checksumFile(backupPath)
		if actual == "" {
			return fmt.Errorf("cannot read %s for verification", backupPath)
		}
		if actual != expected {
			return fmt.Errorf("%s has been modified (checksum mismatch)", backupPath)
		}
	}

	return os.Rename(backupPath, originalPath)
}

// Copy writes content to path with the given mode. Returns the destination
// path and compensation state.
//
// Parameters:
//   - path: Absolute path where the file will be written
//   - mode: File permission bits (e.g., 0o644)
func (p *Provider) Copy(path string, mode os.FileMode, content []byte) (result string, state map[string]any, err error) {
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
			return "", nil, err
		}
	} else {
		state = map[string]any{
			"path":           path,
			"existed_before": false,
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", nil, err
	}

	if mode == 0 {
		mode = 0o644
	}

	if err := os.WriteFile(path, content, mode); err != nil {
		return "", nil, err
	}

	state["written_checksum"] = checksumBytes(content)
	return path, state, nil
}

// CompensateCopy undoes a Copy action using the captured state.
// If a written_checksum is present, the file is verified before reverting;
// a mismatch indicates external modification and the compensation is skipped.
func (p *Provider) CompensateCopy(state any) error {
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	path := op.StateString(s, "path")
	if path == "" {
		return nil
	}

	// Verify the file hasn't been modified since we wrote it.
	if expected := op.StateString(s, "written_checksum"); expected != "" {
		current, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("cannot read %s for verification: %w", path, err)
		}
		if checksumBytes(current) != expected {
			return fmt.Errorf("%s has been modified (checksum mismatch)", path)
		}
	}

	if !op.StateBool(s, "existed_before") {
		return os.Remove(path)
	}
	prev := op.StateBytes(s, "previous_content")
	if prev == nil {
		return nil // Can't restore without content.
	}
	prevMode := op.StateFileMode(s, "previous_mode")
	if prevMode == 0 {
		prevMode = 0o644
	}
	return os.WriteFile(path, prev, prevMode)
}

// Link creates a symlink at path pointing to source. Idempotent: if the
// symlink already points correctly, it's a no-op (returns nil state).
//
// Parameters:
//   - source: Absolute path to the symlink target
//   - path: Absolute path where the symlink will be created
func (p *Provider) Link(source, path string) (result string, state map[string]any, err error) {
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
			return "", nil, err
		}
	} else {
		state = map[string]any{
			"path":           path,
			"existed_before": false,
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", nil, err
	}

	if err := os.Symlink(source, path); err != nil {
		return "", nil, err
	}
	return path, state, nil
}

// CompensateLink undoes a Link action using the captured state.
func (p *Provider) CompensateLink(state any) error {
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	path := op.StateString(s, "path")
	if path == "" {
		return nil
	}
	if !op.StateBool(s, "existed_before") {
		return os.Remove(path)
	}
	prevTarget := op.StateString(s, "previous_target")
	if prevTarget == "" {
		// Was a non-symlink — can't restore, just remove the new symlink.
		return os.Remove(path)
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return os.Symlink(prevTarget, path)
}

// Move moves a file from source to path. Uses gitMv if provided
// (preserves git history), falling back to os.Rename.
// Returns compensation state with paths for reverse move.
//
// Parameters:
//   - source: Absolute path to the file to move
//   - path: Absolute destination path
func (p *Provider) Move(gitMv func(src, dst string) error, source, path string) (result string, compState map[string]any, err error) {
	if _, err := os.Stat(source); err != nil {
		return "", nil, err
	}

	// Capture content checksum before the rename.
	checksum := checksumFile(source)

	if gitMv != nil {
		if err := gitMv(source, path); err == nil {
			state := map[string]any{
				"source": source,
				"path":   path,
			}
			if checksum != "" {
				state["written_checksum"] = checksum
			}
			return path, state, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", nil, err
	}

	if err := os.Rename(source, path); err != nil {
		return "", nil, err
	}
	state := map[string]any{
		"source": source,
		"path":   path,
	}
	if checksum != "" {
		state["written_checksum"] = checksum
	}
	return path, state, nil
}

// CompensateMove undoes a Move by moving the file back from path to
// source. If a written_checksum is present, the destination file is
// verified before restoring; a mismatch indicates external modification
// and the compensation is skipped.
func (p *Provider) CompensateMove(state any) error {
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	source := op.StateString(s, "source")
	path := op.StateString(s, "path")
	if source == "" || path == "" {
		return nil
	}

	// Verify the file hasn't been modified since we moved it.
	if expected := op.StateString(s, "written_checksum"); expected != "" {
		actual := checksumFile(path)
		if actual == "" {
			return fmt.Errorf("cannot read %s for verification", path)
		}
		if actual != expected {
			return fmt.Errorf("%s has been modified (checksum mismatch)", path)
		}
	}

	if err := os.MkdirAll(filepath.Dir(source), 0o750); err != nil {
		return err
	}
	return os.Rename(path, source)
}

// Remove deletes the file at path. If prune is true and pruneBoundary
// is set, empty parent directories are removed up to the boundary.
// Returns compensation state with file content for re-creation.
//
// Parameters:
//   - path: Absolute path to the file to delete
//   - prune: If true, remove empty parent directories after deletion
//   - pruneBoundary: Stop pruning at this directory (prevents removing too much)
func (p *Provider) Remove(path string, prune bool, pruneBoundary string) (result string, compState map[string]any, err error) {
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
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	path := op.StateString(s, "path")
	if path == "" {
		return nil
	}
	content := op.StateBytes(s, "content")
	if content == nil {
		return nil // Can't restore without content.
	}
	mode := op.StateFileMode(s, "mode")
	if mode == 0 {
		mode = 0o644
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, content, mode)
}

// Unlink removes a symlink at path. If prune is true and pruneBoundary
// is set, empty parent directories are removed up to the boundary.
// Returns compensation state with the symlink target for re-creation.
//
// Parameters:
//   - path: Absolute path to the symlink to remove
//   - prune: If true, remove empty parent directories after unlinking
//   - pruneBoundary: Stop pruning at this directory (prevents removing too much)
func (p *Provider) Unlink(path string, prune bool, pruneBoundary string) (result string, compState map[string]any, err error) {
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

	target, err := os.Readlink(path)
	if err != nil {
		return "", nil, err
	}

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
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	path := op.StateString(s, "path")
	target := op.StateString(s, "target")
	if path == "" || target == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.Symlink(target, path)
}

// Write writes inline content to path with the given mode.
// Returns compensation state for undo.
//
// Parameters:
//   - content: String content to write to the file
//   - path: Absolute path where the file will be written
//   - mode: File permission bits (e.g., 0o644)
func (p *Provider) Write(content, path string, mode os.FileMode) (result string, compState map[string]any, err error) {
	if content == "" {
		return "", nil, fmt.Errorf("no content specified")
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

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", nil, err
	}

	if mode == 0 {
		mode = 0o644
	}

	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return "", nil, err
	}
	state["written_checksum"] = checksumBytes([]byte(content))
	return path, state, nil
}

// CompensateWrite undoes a Write action using the captured state.
// If a written_checksum is present, the file is verified before reverting;
// a mismatch indicates external modification and the compensation is skipped.
func (p *Provider) CompensateWrite(state any) error {
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	path := op.StateString(s, "path")
	if path == "" {
		return nil
	}

	// Verify the file hasn't been modified since we wrote it.
	if expected := op.StateString(s, "written_checksum"); expected != "" {
		current, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("cannot read %s for verification: %w", path, err)
		}
		if checksumBytes(current) != expected {
			return fmt.Errorf("%s has been modified (checksum mismatch)", path)
		}
	}

	if !op.StateBool(s, "existed_before") {
		return os.Remove(path)
	}
	prev := op.StateBytes(s, "previous_content")
	if prev == nil {
		return nil // Can't restore without content.
	}
	prevMode := op.StateFileMode(s, "previous_mode")
	if prevMode == 0 {
		prevMode = 0o644
	}
	return os.WriteFile(path, prev, prevMode)
}

// ── Standalone Methods ───────────────────────────────────────────────

// Exists returns true if path exists on the filesystem.
//
// Parameters:
//   - path: Absolute path to check
func (p *Provider) Exists(path string) (bool, error) {
	_, err := os.Lstat(path)
	return err == nil, nil
}

// Glob returns file paths matching a pattern relative to Root.
//
// Parameters:
//   - pattern: Glob pattern (e.g., "*.go", "**/*.yaml")
//   - gitignore: If true, filter results using gitignore rules
func (p *Provider) Glob(pattern string, gitignore bool) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	if !gitignore || p.Root == "" {
		return matches, nil
	}

	tracker, trackerErr := ignore.NewTracker(p.Root)
	if trackerErr != nil {
		return matches, nil //nolint:nilerr // graceful degradation: return unfiltered if gitignore unavailable
	}

	var filtered []string
	for _, m := range matches {
		relPath, relErr := filepath.Rel(tracker.Root(), m)
		if relErr != nil {
			filtered = append(filtered, m)
			continue
		}
		info, statErr := os.Stat(m)
		isDir := statErr == nil && info.IsDir()
		ignored, _ := tracker.IsIgnored(relPath, isDir)
		if !ignored {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}

// IsDir returns true if path exists and is a directory.
//
// Parameters:
//   - path: Absolute path to check
func (p *Provider) IsDir(path string) (bool, error) {
	info, err := os.Stat(path)
	return err == nil && info.IsDir(), nil
}

// IsFile returns true if path exists and is a regular file.
//
// Parameters:
//   - path: Absolute path to check
func (p *Provider) IsFile(path string) (bool, error) {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular(), nil
}

// Join joins path components using the OS path separator.
//
// Parameters:
//   - parts: Path components to join
func (p *Provider) Join(parts ...string) string {
	return filepath.Join(parts...)
}

// Mkdir creates a directory (and parents) with the given mode.
//
// Parameters:
//   - path: Absolute path of the directory to create
//   - mode: Directory permission bits (e.g., 0o755)
func (p *Provider) Mkdir(path string, mode os.FileMode) (string, error) {
	return path, os.MkdirAll(path, mode)
}

// Name returns the last element of path (the file or directory name).
//
// Parameters:
//   - path: Path to extract the name from
func (p *Provider) Name(path string) string {
	return filepath.Base(path)
}

// Parent returns the directory containing path.
//
// Parameters:
//   - path: Path to get the parent of
func (p *Provider) Parent(path string) string {
	return filepath.Dir(path)
}

// Read reads a file and returns its contents.
//
// Parameters:
//   - path: Absolute path to the file to read
func (p *Provider) Read(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// RemoveAll removes path and any children it contains.
//
// Parameters:
//   - path: Path to remove recursively
func (p *Provider) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// WalkTree performs a depth-first traversal of root, calling fn for each
// entry. If gitignore is true, ignored files and directories are skipped.
//
// Parameters:
//   - root: Directory to walk
//   - fn: Callback receiving (relativePath, isDir)
//   - gitignore: If true, respect gitignore rules
func (p *Provider) WalkTree(root string, fn func(string, bool) error, gitignore bool) error {
	opts := ignore.WalkOptions{
		Root:     root,
		Callback: fn,
	}

	if gitignore {
		tracker, err := ignore.NewTracker(root)
		if err != nil {
			return err
		}
		opts.Tracker = tracker
	}

	return ignore.WalkTree(opts)
}
