// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package binding

import "testing"

func TestVariable_String(t *testing.T) {

	tests := []struct {
		name string
		v    Variable
		want string
	}{
		{
			name: "string value with env origin",
			v: Variable{
				Name:   "target_root",
				Value:  "/tmp/x",
				Origin: Origin{Namespace: NamespaceEnv, Name: "DEVLORE_WRIT_TARGET_ROOT"},
			},
			want: "target_root = /tmp/x [env:DEVLORE_WRIT_TARGET_ROOT]",
		},
		{
			name: "int value with default origin",
			v: Variable{
				Name:   "chmod",
				Value:  420,
				Origin: Origin{Namespace: NamespaceDefault, Name: "chmod"},
			},
			want: "chmod = 420 [default:chmod]",
		},
		{
			name: "bool value with flag origin",
			v: Variable{
				Name:   "verbose",
				Value:  true,
				Origin: Origin{Namespace: NamespaceFlag, Name: "verbose"},
			},
			want: "verbose = true [flag:verbose]",
		},
		{
			name: "value containing spaces — brackets keep origin separable",
			v: Variable{
				Name:   "project_path",
				Value:  "/Users/me/my docs/x",
				Origin: Origin{Namespace: NamespaceConfig, Name: "project_path"},
			},
			want: "project_path = /Users/me/my docs/x [config:project_path]",
		},
		{
			name: "nil value renders as Go's <nil>",
			v: Variable{
				Name:   "optional_thing",
				Value:  nil,
				Origin: Origin{Namespace: NamespaceDefault, Name: "optional_thing"},
			},
			want: "optional_thing = <nil> [default:optional_thing]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
