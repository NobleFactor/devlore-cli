// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCorpusLoads proves every packages-manifest in the repository parses.
//
// The corpus and the loader disagreed once: all five manifests under Home/ used bare strings, which
// cannot unmarshal into PackageEntry, and nothing noticed because no test loaded them. A manifest is
// a declaration nothing exercises until a deploy, so the drift was invisible until then. This walks
// the tree instead of naming files, so a new layer's manifest is covered the moment it is added.
func TestCorpusLoads(t *testing.T) {

	root := filepath.Join("..", "..")

	var found int

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return err
		}

		if info.IsDir() {

			// None of these is ours to validate: .git holds no manifests, testdata deliberately
			// carries malformed ones for the loader's own error paths, and schema/ holds
			// packages-manifest.json -- the JSON Schema, which shares the name but is not a manifest
			// and would pass vacuously, since an absent packages key yields an empty list.
			switch info.Name() {
			case ".git", "testdata", "schema":
				return filepath.SkipDir
			}

			return nil
		}

		name := info.Name()

		if name != "packages-manifest.yaml" && name != "packages-manifest.json" {
			return nil
		}

		found++

		if _, loadErr := Load(path); loadErr != nil {
			t.Errorf("%s does not load: %v", path, loadErr)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	if found == 0 {
		t.Fatal("no manifests found; the walk is looking in the wrong place")
	}

	t.Logf("loaded %d manifests", found)
}
