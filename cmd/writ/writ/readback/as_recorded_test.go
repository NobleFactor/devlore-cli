// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package readback_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// TestAsRecorded_Link pins #883: a link is as recorded when its literal endpoint is the recorded source, whether or
// not that source exists; a link elsewhere and a non-link are not.
func TestAsRecorded_Link(t *testing.T) {

	root := t.TempDir()
	source := filepath.Join(root, "layer", "file")
	target := filepath.Join(root, "home", ".file")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	entry := readback.Entry{Target: target, Source: source, Action: string(file.Link)}

	if entry.AsRecorded() {
		t.Error("an absent target reads as recorded")
	}

	if err := os.Symlink(filepath.Join("..", "layer", "file"), target); err != nil {
		t.Fatal(err)
	}
	if !entry.AsRecorded() {
		t.Error("a dangling link whose endpoint is the recorded source does not read as recorded (#883)")
	}

	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !entry.AsRecorded() {
		t.Error("a resolving link to the recorded source does not read as recorded")
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "layer", "other"), target); err != nil {
		t.Fatal(err)
	}
	if entry.AsRecorded() {
		t.Error("a link elsewhere reads as recorded")
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("hand-written"), 0o644); err != nil {
		t.Fatal(err)
	}
	if entry.AsRecorded() {
		t.Error("a regular file where the link was reads as recorded")
	}
}

// TestAsRecorded_Copy pins the copy half: as recorded when the target's digest is the recorded one; not when the
// content moved, and never when the record carries no digest to vouch with.
func TestAsRecorded_Copy(t *testing.T) {

	root := t.TempDir()
	target := filepath.Join(root, ".file")
	content := []byte("rendered")
	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatal(err)
	}

	entry := readback.Entry{Target: target, Action: string(file.Copy), RecordedDigest: readback.ContentDigest(content)}
	if !entry.AsRecorded() {
		t.Error("a copy with the recorded content does not read as recorded")
	}

	if err := os.WriteFile(target, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if entry.AsRecorded() {
		t.Error("an edited copy reads as recorded")
	}

	if (readback.Entry{Target: target, Action: string(file.Copy)}).AsRecorded() {
		t.Error("a record with no digest vouches for a copy")
	}
}
