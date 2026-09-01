// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"filippo.io/age"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Config contains all resolved settings for a lifecycle operation.
// This is the common base; specific operations extend it.
type Config struct {
	// Tool is "writ" or "lore".
	Tool string

	// Sources
	LayerSources []tree.LayerSource
	SourceRoot   string // single-repo mode (when no layers configured)
	TargetRoot   string

	// Selection
	Projects []string
	Segments segment.Segments

	// Behavior
	DryRun         bool
	Verbose        bool
	ConflictPolicy op.ConflictPolicy

	// Data for templates
	TemplateData map[string]any

	// Identities for decryption
	Identities []age.Identity

	// Signing key (optional)
	SigningKey *age.X25519Identity
}

// DeployConfig contains all settings for a deploy operation.
type DeployConfig struct {
	Config

	// AllowDirty permits planning against layers with uncommitted changes.
	AllowDirty bool
}

// UpgradeConfig contains all settings for an upgrade operation.
type UpgradeConfig struct {
	Config

	// Force upgrades even if target has local modifications.
	Force bool
}

// ReconcileConfig contains all settings for a reconcile report.
type ReconcileConfig struct {
	Config
}

// DecommissionConfig contains all settings for a decommission operation.
type DecommissionConfig struct {
	Config

	// Prune empty parent directories after file removal.
	Prune bool
}

// AdoptConfig contains all settings for an adopt operation.
type AdoptConfig struct {
	Config

	// Files to adopt.
	Files []string

	// Layer to adopt into (personal, team, base).
	Layer string

	// LayerPath is the resolved path to the layer directory.
	LayerPath string

	// Project to adopt into.
	Project string

	// FromReceipt adopts from a lore receipt.
	FromReceipt bool
}

// SegmentMap returns the segments as a string map for template data.
func (c *Config) SegmentMap() map[string]string {
	m := make(map[string]string)
	for _, seg := range c.Segments {
		if seg.Value != "" {
			m[seg.Name] = seg.Value
		}
	}
	return m
}
