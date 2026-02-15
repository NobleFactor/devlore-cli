// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

// Provider provides file system operations. Each method receives all
// inputs as parameters — no execution context, no node access.
type Provider struct{}

// Link creates a symlink at path pointing to source. Idempotent: if the
// symlink already points correctly, it's a no-op.
func (p *Provider) Link(source, path string) error {
	// Idempotent: check if symlink already points correctly
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			existing, err := os.Readlink(path)
			if err == nil && existing == source {
				return nil // Already correct
			}
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove existing: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	return os.Symlink(source, path)
}

// Copy writes content to path with the given mode. Returns the SHA256
// checksum of the written content.
func (p *Provider) Copy(path string, mode os.FileMode, content []byte) (string, error) {
	// Remove existing file/symlink if present
	if _, err := os.Lstat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return "", fmt.Errorf("remove existing: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("create parent dirs: %w", err)
	}

	if mode == 0 {
		mode = 0644
	}

	if err := os.WriteFile(path, content, mode); err != nil {
		return "", err
	}

	return checksumBytes(content), nil
}

// Render processes content as a Go text/template. Returns the rendered bytes.
func (p *Provider) Render(templateData map[string]any, source, path, project string, content []byte) ([]byte, error) {
	tmpl, err := template.New("render").Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	data := make(map[string]any)
	for k, v := range templateData {
		data[k] = v
	}
	data["Source"] = source
	data["Target"] = path
	data["Project"] = project

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	return buf.Bytes(), nil
}

// Backup moves the file at path to a timestamped backup location.
// Returns the backup path.
func (p *Provider) Backup(path, backupSuffix string) (string, error) {
	if backupSuffix == "" {
		backupSuffix = ".writ-backup"
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := path + backupSuffix + "." + timestamp

	if err := os.Rename(path, backupPath); err != nil {
		return "", fmt.Errorf("backup %s → %s: %w", path, backupPath, err)
	}

	return backupPath, nil
}

// Unlink removes a symlink at path. If prune is true and pruneBoundary
// is set, empty parent directories are removed up to the boundary.
func (p *Provider) Unlink(path string, prune bool, pruneBoundary string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil // Already gone
	}
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s is not a symlink", path)
	}

	if err := os.Remove(path); err != nil {
		return err
	}

	pruneParents(path, prune, pruneBoundary)
	return nil
}

// Remove deletes the file at path. If prune is true and pruneBoundary
// is set, empty parent directories are removed up to the boundary.
func (p *Provider) Remove(path string, prune bool, pruneBoundary string) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil // Already gone
	}

	if err := os.Remove(path); err != nil {
		return err
	}

	pruneParents(path, prune, pruneBoundary)
	return nil
}

// Write writes inline content to path with the given mode.
func (p *Provider) Write(content, path string, mode os.FileMode) error {
	if content == "" {
		return fmt.Errorf("write: no content specified")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	if mode == 0 {
		mode = 0644
	}

	return os.WriteFile(path, []byte(content), mode)
}

// Move moves a file from source to path. Uses gitMv if provided
// (preserves git history), falling back to os.Rename.
func (p *Provider) Move(gitMv func(src, dst string) error, source, path string) error {
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("source does not exist: %w", err)
	}

	if gitMv != nil {
		if err := gitMv(source, path); err == nil {
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	return os.Rename(source, path)
}

// Source reads a file and returns its contents.
func (p *Provider) Source(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Mkdir creates a directory (and parents) with the given mode.
func (p *Provider) Mkdir(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}
