// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Provider provides archive extraction actions.
//
// Compensable Forward methods return (string, map[string]any, error):
// the extraction directory, the compensation receipt, and an error.
// The map is opaque to the executor, meaningful only to the
// corresponding Compensate* Backward method.
//
//devlore:plannable
type Provider struct{}

// Extract extracts an archive (tar.gz or zip) from source into the prefix directory.
// The archive format is detected from the file extension.
// Returns compensation state with the list of created files.
//
// Slots:
//   - source: Path to the archive file (tar.gz, tgz, or zip)
//   - prefix: Directory to extract into
func (p *Provider) Extract(source, prefix string) (string, map[string]any, error) {
	if err := os.MkdirAll(prefix, 0755); err != nil {
		return "", nil, fmt.Errorf("create prefix dir: %w", err)
	}

	var created []string
	var err error

	lower := strings.ToLower(source)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		created, err = extractTarGz(source, prefix)
	case strings.HasSuffix(lower, ".zip"):
		created, err = extractZip(source, prefix)
	default:
		return "", nil, fmt.Errorf("unsupported archive format: %s", source)
	}

	if err != nil {
		return "", nil, err
	}

	return prefix, map[string]any{
		"dest":          prefix,
		"created_files": created,
	}, nil
}

// CompensateExtract removes files created during extraction, then cleans up
// empty directories under dest.
func (p *Provider) CompensateExtract(state any) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	created, _ := s["created_files"].([]string)

	// Remove files in reverse order (deepest first)
	for i := len(created) - 1; i >= 0; i-- {
		os.Remove(created[i])
	}

	// Clean up empty directories under dest
	dest, _ := s["dest"].(string)
	if dest != "" {
		removeEmptyDirs(dest)
	}
	return nil
}

// removeEmptyDirs walks dest bottom-up removing empty directories.
func removeEmptyDirs(root string) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		// Try to remove — fails silently if non-empty
		os.Remove(path)
		return nil
	})
}

func extractTarGz(source, prefix string) ([]string, error) {
	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	var created []string
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return created, fmt.Errorf("tar: %w", err)
		}

		target := filepath.Join(prefix, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(prefix)+string(os.PathSeparator)) {
			continue // skip entries that escape the prefix (zip slip protection)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return created, err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return created, err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return created, err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return created, err
			}
			out.Close()
			created = append(created, target)
		}
	}
	return created, nil
}

func extractZip(source, prefix string) ([]string, error) {
	r, err := zip.OpenReader(source)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var created []string
	for _, f := range r.File {
		target := filepath.Join(prefix, filepath.Clean(f.Name))
		if !strings.HasPrefix(target, filepath.Clean(prefix)+string(os.PathSeparator)) {
			continue // zip slip protection
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return created, err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return created, err
		}

		rc, err := f.Open()
		if err != nil {
			return created, err
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return created, err
		}

		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return created, err
		}
		out.Close()
		rc.Close()
		created = append(created, target)
	}
	return created, nil
}
