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

// Provider provides archive extraction operations.
type Provider struct{}

// Extract extracts an archive (tar.gz or zip) from source into the prefix directory.
// The archive format is detected from the file extension.
func (p *Provider) Extract(source, prefix string) error {
	if err := os.MkdirAll(prefix, 0755); err != nil {
		return fmt.Errorf("create prefix dir: %w", err)
	}

	lower := strings.ToLower(source)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return extractTarGz(source, prefix)
	case strings.HasSuffix(lower, ".zip"):
		return extractZip(source, prefix)
	default:
		return fmt.Errorf("unsupported archive format: %s", source)
	}
}

func extractTarGz(source, prefix string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}

		target := filepath.Join(prefix, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(prefix)+string(os.PathSeparator)) {
			continue // skip entries that escape the prefix (zip slip protection)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

func extractZip(source, prefix string) error {
	r, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(prefix, filepath.Clean(f.Name))
		if !strings.HasPrefix(target, filepath.Clean(prefix)+string(os.PathSeparator)) {
			continue // zip slip protection
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}
