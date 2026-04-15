// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package archive provides archive extraction actions for the operation graph.
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// Provider provides archive extraction actions.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

func NewProvider(ctx *op.ExecutionContext) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// --- Compensable Pairs ---

// Extract extracts an archive (tar.gz or zip) from source into the directory at prefixPath.
//
// Identity for the prefix directory is constructed by [Provider.ExtractPlanned]. The archive format is detected
// from the source file's extension.
//
// Parameters:
//   - source: [file.Resource] identifying the archive file (tar.gz, tgz, or zip).
//   - prefixPath: the extraction directory path. Coerced to a [file.Resource] via [Provider.ExtractPlanned].
//
// Returns:
//   - *file.Resource: the extraction directory resource with populated metadata.
//   - Tombstone: compensation state with the list of created files.
//   - error: any error from extraction.
func (p *Provider) Extract(source *file.Resource, prefixPath string) (*file.Resource, Tombstone, error) {

	prefix, err := p.ExtractPlanned(source, prefixPath)
	if err != nil {
		return nil, Tombstone{}, err
	}

	if err := os.MkdirAll(prefix.SourcePath.Abs(), 0o750); err != nil {
		return nil, Tombstone{}, fmt.Errorf("create prefix dir: %w", err)
	}

	var created []string

	lower := strings.ToLower(source.SourcePath.Abs())
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		created, err = extractTarGz(source.SourcePath.Abs(), prefix.SourcePath.Abs())
	case strings.HasSuffix(lower, ".zip"):
		created, err = extractZip(source.SourcePath.Abs(), prefix.SourcePath.Abs())
	default:
		return nil, Tombstone{}, fmt.Errorf("unsupported archive format: %s", source.SourcePath.Abs())
	}

	if err != nil {
		return nil, Tombstone{}, err
	}

	if err := prefix.Resolve(); err != nil {
		return prefix, Tombstone{}, err
	}

	return prefix, Tombstone{
		Dest:         prefix.SourcePath.Abs(),
		CreatedFiles: created,
	}, nil
}

// ExtractPlanned is the Planned companion for [Provider.Extract]. Pure: no I/O, no target state.
//
// Parameters:
//   - source: ignored; present to match [Provider.Extract]'s signature exactly.
//   - prefixPath: the extraction directory path whose identity should be constructed.
//
// Returns:
//   - *file.Resource: the prefix directory resource with URI set and metadata empty.
//   - error: any error from resource construction.
func (p *Provider) ExtractPlanned(_ *file.Resource, prefixPath string) (*file.Resource, error) {
	return file.NewResource(p.ExecutionContext(), prefixPath)
}

// CompensateExtract removes files created during extraction, then cleans up
// empty directories under dest.
func (p *Provider) CompensateExtract(state Tombstone) error {
	if state.Dest == "" {
		return nil
	}

	// Remove files in reverse order (deepest first).
	for i := len(state.CreatedFiles) - 1; i >= 0; i-- {
		_ = os.Remove(state.CreatedFiles[i])
	}

	// Clean up empty directories under dest.
	return removeEmptyDirs(state.Dest)
}

// removeEmptyDirs walks dest bottom-up removing empty directories.
func removeEmptyDirs(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil //nolint:nilerr // intentional: skip unsupported entries
		}
		// Try to remove — fails silently if non-empty
		_ = os.Remove(path)
		return nil
	})
}

func extractTarGz(source, prefix string) (created []string, err error) {

	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer iox.Close(&err, f)

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer func() {
		if closeErr := gz.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("gzip close: %w", closeErr)
		}
	}()

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
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode&0o777)); err != nil {
				return created, err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return created, err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode&0o777))
			if err != nil {
				return created, err
			}
			if _, err := io.Copy(out, io.LimitReader(tr, 1<<30)); err != nil {
				return created, errors.Join(err, out.Close())
			}
			if err := out.Close(); err != nil {
				return created, err
			}
			created = append(created, target)
		}
	}
	return created, nil
}

func extractZip(source, prefix string) (created []string, err error) {

	r, err := zip.OpenReader(source)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := r.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("zip close: %w", closeErr)
		}
	}()

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

		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return created, err
		}

		rc, err := f.Open()
		if err != nil {
			return created, err
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			return created, errors.Join(err, rc.Close())
		}

		if _, err := io.Copy(out, io.LimitReader(rc, 1<<30)); err != nil {
			return created, errors.Join(err, out.Close(), rc.Close())
		}
		if err := out.Close(); err != nil {
			return created, errors.Join(err, rc.Close())
		}
		_ = rc.Close()
		created = append(created, target)
	}
	return created, nil
}
