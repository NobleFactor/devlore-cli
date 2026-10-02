// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build windows

package adopt

import (
	"path/filepath"
	"strings"
	"testing"
)

// --- Collect ---

// TestCollect_ALayerOnAnotherDriveRefusesTheItem proves an item is refused when its scope's root and the layer share
// no directory, so no run can be confined to both: the layer on another drive.
//
// Parameters:
//   - `t`: the test harness.
func TestCollect_ALayerOnAnotherDriveRefusesTheItem(t *testing.T) {

	root := t.TempDir()
	item := filepath.Join(root, "ProgramData", "app", "settings.conf")
	writeFileForTest(t, item)

	// The judgment is lexical, so the other drive need not exist; only it must differ from the item's.
	drive := `Z:`
	if strings.EqualFold(filepath.VolumeName(root), drive) {
		drive = `Y:`
	}

	groups := Collect(&Config{
		Files:      []string{item},
		TargetRoot: root,
		Scopes:     []Scope{{Name: "programdata", Directory: "ProgramData", Root: filepath.Join(root, "ProgramData")}},
		LayerPath:  drive + `\repos\Personal`,
		Project:    "adopted",
	})

	if len(groups) != 0 {
		t.Errorf("groups = %v, want the item refused", groups)
	}
}
