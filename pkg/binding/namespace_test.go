// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package binding

import "testing"

func TestNamespace_String(t *testing.T) {

	tests := []struct {
		n    Namespace
		want string
	}{
		{NamespaceUnknown, "unknown"},
		{NamespaceDefault, "default"},
		{NamespaceConfig, "config"},
		{NamespaceEnv, "env"},
		{NamespaceFlag, "flag"},
		{NamespaceOverride, "override"},
	}

	for _, tc := range tests {
		if got := tc.n.String(); got != tc.want {
			t.Errorf("Namespace(%d).String() = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestNamespace_PrecedenceOrder(t *testing.T) {

	// Numeric values must ascend with precedence so callers can compare Namespaces directly.
	if !(NamespaceUnknown < NamespaceDefault &&
		NamespaceDefault < NamespaceConfig &&
		NamespaceConfig < NamespaceEnv &&
		NamespaceEnv < NamespaceFlag &&
		NamespaceFlag < NamespaceOverride) {
		t.Errorf("Namespace precedence order is not strictly ascending")
	}
}
