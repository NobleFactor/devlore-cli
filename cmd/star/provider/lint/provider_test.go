// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package lint

import (
	"os"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestDefaultGolangciConfigTracksRepoConfig asserts the embedded seed template stays in lockstep with the
// repository's own .golangci.yaml.
//
// The repository config is the canonical, hardened through the lint ladder; the embedded template seeds
// new repos ("NobleFactor defaults") and must match it semantically, minus the exclusions declared
// repo-specific below. A failure means a config change did not flow into the template, or a new
// repo-specific exclusion is missing from the declaration.
func TestDefaultGolangciConfigTracksRepoConfig(t *testing.T) {

	// Exclusion rules whose `path` names devlore-cli-only packages; they never ship in the template.
	repoSpecificRulePaths := map[string]bool{
		"pkg/op/provider/(json|yaml)/": true,
	}

	repoBytes, err := os.ReadFile("../../../../.golangci.yaml")
	if err != nil {
		t.Fatalf("reading repository config: %v", err)
	}

	var repoConfig, templateConfig map[string]any
	if err := yaml.Unmarshal(repoBytes, &repoConfig); err != nil {
		t.Fatalf("parsing repository config: %v", err)
	}
	if err := yaml.Unmarshal([]byte(defaultGolangciConfig), &templateConfig); err != nil {
		t.Fatalf("parsing embedded template: %v", err)
	}

	linters, ok := repoConfig["linters"].(map[string]any)
	if !ok {
		t.Fatal("repository config has no linters mapping")
	}
	exclusions, ok := linters["exclusions"].(map[string]any)
	if !ok {
		t.Fatal("repository config has no linters.exclusions mapping")
	}
	rules, ok := exclusions["rules"].([]any)
	if !ok {
		t.Fatal("repository config has no linters.exclusions.rules sequence")
	}

	var shared []any
	for _, rule := range rules {
		mapping, ok := rule.(map[string]any)
		if !ok {
			t.Fatalf("exclusion rule is not a mapping: %v", rule)
		}
		if path, _ := mapping["path"].(string); repoSpecificRulePaths[path] {
			continue
		}
		shared = append(shared, rule)
	}
	exclusions["rules"] = shared

	if !reflect.DeepEqual(repoConfig, templateConfig) {
		t.Fatal("defaultGolangciConfig has drifted from .golangci.yaml: " +
			"sync the embedded template with the repository config (minus the declared repo-specific exclusions)")
	}
}
