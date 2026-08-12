// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"slices"
	"testing"
)

func TestWithCommonProject(t *testing.T) {

	cases := []struct {
		name     string
		projects []string
		want     []string
	}{
		{"injected first", []string{"noblefactor", "thenobles"}, []string{"common", "noblefactor", "thenobles"}},
		{"not duplicated", []string{"common", "noblefactor"}, []string{"common", "noblefactor"}},
		{"empty means every project, unchanged", nil, nil},
	}
	for _, c := range cases {
		if got := withCommonProject(c.projects); !slices.Equal(got, c.want) {
			t.Errorf("%s: withCommonProject(%v) = %v, want %v", c.name, c.projects, got, c.want)
		}
	}
}
