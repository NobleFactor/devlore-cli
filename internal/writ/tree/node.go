// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package tree

import (
	"path/filepath"
	"strings"
)

// PackagesManifestFiles are filenames that contain package specifications.
// These files are processed by the Package Graph Builder to produce package
// installation nodes in the execution graph.
//
// packages-manifest.{yaml,json} is the canonical format.
// packages.manifest is supported for backward compatibility.
var PackagesManifestFiles = []string{
	"packages-manifest.yaml",
	"packages-manifest.json",
	"packages.manifest", // legacy
}

// ProcessingPipeline determines operations from a filename.
// Extensions are processed outside-in (like .tar.gz).
//
// Examples:
//
//	"foo"                     → "foo",                     [link]
//	"foo.template"            → "foo",                     [expand, copy]
//	"foo.age"                 → "foo",                     [decrypt, copy]
//	"foo.template.age"        → "foo",                     [decrypt, expand, copy]
//	"packages-manifest.yaml"  → "packages-manifest.yaml",  [packages]
func ProcessingPipeline(filename string) (targetName string, ops Operations) {
	name := filename
	baseName := filepath.Base(name)

	// Check for packages-manifest files (processed by Package Graph Builder)
	for _, pf := range PackagesManifestFiles {
		if baseName == pf {
			return name, Operations{OpPackages}
		}
	}

	var pipeline Operations

	// Process extensions outside-in
	// .age is outermost (decrypt first)
	if strings.HasSuffix(name, ".age") {
		name = strings.TrimSuffix(name, ".age")
		pipeline = append(pipeline, OpDecrypt)
	}

	// .sops is outermost (decrypt first) — SOPS-encrypted files
	if strings.HasSuffix(name, ".sops") {
		name = strings.TrimSuffix(name, ".sops")
		pipeline = append(pipeline, OpDecrypt)
	}

	// .template is inner (expand after decrypt)
	if strings.HasSuffix(name, ".template") {
		name = strings.TrimSuffix(name, ".template")
		pipeline = append(pipeline, OpExpand)
	}

	// Determine final operation
	if len(pipeline) > 0 {
		// After decrypt or expand, we copy the result
		pipeline = append(pipeline, OpCopy)
	} else {
		// Plain file: just link
		pipeline = append(pipeline, OpLink)
	}

	return name, pipeline
}
