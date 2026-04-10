// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package tree

import (
	"path/filepath"
	"strings"
)

// PackagesManifestFiles are filenames that contain package specifications.
// These files are processed by the PkgPath Graph Builder to produce package
// installation nodes in the execution graph.
var PackagesManifestFiles = []string{
	"packages-manifest.yaml",
	"packages-manifest.json",
}

// ProcessingPipeline determines the action pipeline from a filename.
// Extensions are processed outside-in (like .tar.gz).
//
// Examples:
//
//	"foo"                     → "foo",                     ["file.link"]
//	"foo.template"            → "foo",                     ["template.render_bytes", "file.copy"]
//	"foo.sops"                → "foo",                     ["encryption.decrypt", "file.copy"]
//	"foo.template.sops"       → "foo",                     ["encryption.decrypt", "template.render_bytes", "file.copy"]
//	"packages-manifest.yaml"  → "packages-manifest.yaml",  ["manifest.resolve"]
func ProcessingPipeline(filename string) (targetName string, actions []string) {
	name := filename
	baseName := filepath.Base(name)

	// packages-manifest → manifest.resolve
	for _, pf := range PackagesManifestFiles {
		if baseName == pf {
			return name, []string{"manifest.resolve"}
		}
	}

	var pipeline []string

	if strings.HasSuffix(name, ".sops") {
		name = strings.TrimSuffix(name, ".sops")
		pipeline = append(pipeline, "encryption.decrypt")
	}

	if strings.HasSuffix(name, ".template") {
		name = strings.TrimSuffix(name, ".template")
		pipeline = append(pipeline, "template.render_bytes")
	}

	if len(pipeline) > 0 {
		pipeline = append(pipeline, "file.copy")
	} else {
		pipeline = []string{"file.link"}
	}

	return name, pipeline
}
