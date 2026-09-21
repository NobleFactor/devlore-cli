// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package platform

import "testing"

// TestWingetListedVersion_MatchesTheIdAsWingetDoes pins #900: winget matches ids case-insensitively and prints the
// catalog's casing, so a query for `Vim.Vim` is answered by a row naming `vim.vim`. The first listing is the one
// DANOBLE-WD11-3 printed on 2026-09-21, verbatim.
func TestWingetListedVersion_MatchesTheIdAsWingetDoes(t *testing.T) {

	vim := "Name                           Id      Version  Source\n" +
		"-------------------------------------------------------\n" +
		"Vim 9.2 (ARM64) (current user) vim.vim 9.2.1119 winget\n"
	two := "Name                Id                      Version Source\n" +
		"-----------------------------------------------------------\n" +
		"Microsoft PowerShell Microsoft.PowerShell    7.5.3   winget\n" +
		"Git                 Git.Git                 2.51.0  winget\n"
	header := "Name Id Version Source\n----------------------\n"

	cases := []struct {
		name, stdout, id, want string
	}{
		{"catalog casing differs from the query", vim, "Vim.Vim", "9.2.1119"},
		{"catalog casing equals the query", vim, "vim.vim", "9.2.1119"},
		{"first of two", two, "Microsoft.PowerShell", "7.5.3"},
		{"second of two, queried in another case", two, "git.git", "2.51.0"},
		{"absent from a listing of others", two, "Vim.Vim", ""},
		{"header and rule alone", header, "Vim.Vim", ""},
		{"empty output", "", "Vim.Vim", ""},
	}
	for _, c := range cases {
		if got := wingetListedVersion(c.stdout, c.id); got != c.want {
			t.Errorf("%s: wingetListedVersion(%q) = %q, want %q", c.name, c.id, got, c.want)
		}
	}
}
