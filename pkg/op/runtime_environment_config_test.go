// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op_test

import (
	"slices"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/devconfig"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestRuntimeEnvironmentConfig_Announced(t *testing.T) {

	names := devconfig.AnnouncedSectionNames()

	if !slices.Contains(names, "runtime") {
		t.Errorf("AnnouncedSectionNames() = %v; want it to contain \"runtime\"", names)
	}

	construct, ok := devconfig.ConstructorFor("runtime")
	if !ok {
		t.Fatal("ConstructorFor(runtime) ok = false; want true")
	}

	if got := construct().Name(); got != "runtime" {
		t.Errorf("announced constructor produced %q, want runtime", got)
	}
}

func TestNewRuntimeEnvironmentConfig_Floor(t *testing.T) {

	section := op.NewRuntimeEnvironmentConfig()

	if section.Name() != "runtime" {
		t.Errorf("Name() = %q, want runtime", section.Name())
	}

	if section.DryRun {
		t.Error("DryRun floor = true, want false")
	}

	if section.ConflictPolicy != op.ConflictStop {
		t.Errorf("ConflictPolicy floor = %v, want ConflictStop", section.ConflictPolicy)
	}

	if section.BackupSuffix != ".devlore-backup" {
		t.Errorf("BackupSuffix floor = %q, want .devlore-backup", section.BackupSuffix)
	}
}
