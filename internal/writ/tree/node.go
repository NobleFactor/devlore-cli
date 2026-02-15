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
var PackagesManifestFiles = []string{
	"packages-manifest.yaml",
	"packages-manifest.json",
}

// ProcessingPipeline determines the action pipeline from a filename.
// Extensions are processed outside-in (like .tar.gz).
//
// Examples:
//
//	"foo"                     → "foo",                     ["link"]
//	"foo.template"            → "foo",                     ["render", "copy"]
//	"foo.sops"                → "foo",                     ["decrypt", "copy"]
//	"foo.template.sops"       → "foo",                     ["decrypt", "render", "copy"]
//	"packages-manifest.yaml"  → "packages-manifest.yaml",  ["manifest-resolve"]
func ProcessingPipeline(filename string) (targetName string, actions []string) {
	name := filename
	baseName := filepath.Base(name)

	// packages-manifest → manifest-resolve
	for _, pf := range PackagesManifestFiles {
		if baseName == pf {
			return name, []string{"manifest-resolve"}
		}
	}

	var pipeline []string

	if strings.HasSuffix(name, ".sops") {
		name = strings.TrimSuffix(name, ".sops")
		pipeline = append(pipeline, "decrypt")
	}

	if strings.HasSuffix(name, ".template") {
		name = strings.TrimSuffix(name, ".template")
		pipeline = append(pipeline, "render")
	}

	if len(pipeline) > 0 {
		pipeline = append(pipeline, "copy")
	} else {
		pipeline = []string{"link"}
	}

	return name, pipeline
}
