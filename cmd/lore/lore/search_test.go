// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package lore

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/internal/lorepackage"
)

// TestSearchRows_KeepsMultiByteDescriptions is #741: the hand-rolled table truncated descriptions by byte
// count -- desc[:47] -- and cut multi-byte characters mid-rune. The row is built without truncating, and
// the shared table formatter aligns by rune, so a description survives whole.
//
// The description here is 47 bytes of ASCII followed by a multi-byte character, so the old cut landed
// inside it. Red before the fix by construction: searchRows did not exist, and the loop that replaced it
// truncated.
func TestSearchRows_KeepsMultiByteDescriptions(t *testing.T) {

	description := strings.Repeat("x", 47) + "é and more"
	if len(description) <= 47 || len([]rune(description)) == len(description) {
		t.Fatalf("test fixture must be longer than 47 bytes and carry a multi-byte rune")
	}

	rows := searchRows([]lorepackage.SearchResultItem{{
		Name:        "docker",
		Version:     "27.0",
		Description: description,
		Installed:   true,
	}})

	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Description != description {
		t.Fatalf("description was altered:\n got %q\nwant %q", rows[0].Description, description)
	}
	if rows[0].Name != "docker" || !rows[0].Installed {
		t.Fatalf("row does not carry the item: %+v", rows[0])
	}

	// And through the table: no byte of the description is lost in rendering either.
	var buf bytes.Buffer
	pipeline, err := cli.BuildPipeline(cli.SinkOptions{Format: "table"}, &buf)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}
	if err := pipeline.Emit(rows); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if !strings.Contains(buf.String(), description) {
		t.Fatalf("the table cut the description:\n%s", buf.String())
	}
}
