// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package binding

import "testing"

func TestOrigin_String(t *testing.T) {

	tests := []struct {
		name   string
		origin Origin
		want   string
	}{
		{
			name:   "unknown renders as bare word",
			origin: Origin{},
			want:   "unknown",
		},
		{
			name:   "default with parameter name",
			origin: Origin{Namespace: NamespaceDefault, Name: "target_root"},
			want:   "default:target_root",
		},
		{
			name:   "config with map key",
			origin: Origin{Namespace: NamespaceConfig, Name: "target_root"},
			want:   "config:target_root",
		},
		{
			name:   "config with file:line",
			origin: Origin{Namespace: NamespaceConfig, Name: "config.star:12"},
			want:   "config:config.star:12",
		},
		{
			name:   "env program-specific",
			origin: Origin{Namespace: NamespaceEnv, Name: "DEVLORE_WRIT_TARGET_ROOT"},
			want:   "env:DEVLORE_WRIT_TARGET_ROOT",
		},
		{
			name:   "env global cascade",
			origin: Origin{Namespace: NamespaceEnv, Name: "DEVLORE_TARGET_ROOT"},
			want:   "env:DEVLORE_TARGET_ROOT",
		},
		{
			name:   "flag with parameter name",
			origin: Origin{Namespace: NamespaceFlag, Name: "layer"},
			want:   "flag:layer",
		},
		{
			name:   "override with map key",
			origin: Origin{Namespace: NamespaceOverride, Name: "force_field"},
			want:   "override:force_field",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.origin.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
