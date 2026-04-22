// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"reflect"
	"testing"
)

// --- guessDirName ---

func TestGuessDirName(t *testing.T) {

	tests := []struct {
		name       string
		repository string
		want       string
		wantErr    bool
	}{
		{name: "https with path and .git", repository: "https://example.com/foo/repo.git", want: "repo"},
		{name: "https with .git and trailing slash", repository: "https://example.com/repo.git/", want: "repo"},
		{name: "https no path suffix", repository: "https://example.com/repo.git", want: "repo"},
		{name: "https without .git", repository: "https://example.com/org/repo", want: "repo"},
		{name: "https host only", repository: "https://example.com/", want: "example.com"},
		{name: "scp-style with .git", repository: "git@github.com:org/repo.git", want: "repo"},
		{name: "scp-style without .git", repository: "git@github.com:org/repo", want: "repo"},
		{name: "scp-style colon-as-separator, no slash", repository: "git@host:repo.git", want: "repo"},
		{name: "absolute local path with .git", repository: "/local/path/to/repo.git", want: "repo"},
		{name: "relative local path with .git", repository: "relative/path/repo.git", want: "repo"},
		{name: "file URI with .git", repository: "file:///path/to/repo.git", want: "repo"},
		{name: "file URI localhost host", repository: "file://localhost/path/to/repo.git", want: "repo"},
		{name: "ssh with auth and trailing slash and .git", repository: "ssh://user:pass@host.xz/path/to/repo.git/", want: "repo"},
		{name: "port-style trailing 2222.git", repository: "host.xz:2222.git", want: "2222"},
		{name: "port-style no path strips port", repository: "host.xz:2222", want: "host.xz"},
		{name: "url with /.git subdir path", repository: "https://example.com/foo/.git", want: "foo"},
		{name: "backwards compat /foo/bar:2222.git", repository: "/foo/bar:2222.git", want: "2222"},

		{name: "empty", repository: "", wantErr: true},
		{name: "slash only", repository: "/", wantErr: true},
		{name: "dot-git only", repository: ".git", wantErr: true},
		{name: "scheme only", repository: "https://", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := guessDirName(tt.repository)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("guessDirName(%q) = %q, nil; want error", tt.repository, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("guessDirName(%q): unexpected error: %v", tt.repository, err)
			}
			if got != tt.want {
				t.Errorf("guessDirName(%q) = %q, want %q", tt.repository, got, tt.want)
			}
		})
	}
}

// --- buildCloneArgs ---

func TestBuildCloneArgs(t *testing.T) {

	const (
		repo = "https://example.com/org/repo.git"
		dir  = "/tmp/repo"
	)

	tests := []struct {
		name              string
		bare              bool
		branch            string
		depth             int
		filter            string
		noCheckout        bool
		noTags            bool
		origin            string
		recurseSubmodules bool
		singleBranch      bool
		kwargs            map[string]any
		want              []string
	}{
		{
			name: "all defaults",
			want: []string{"clone", repo, dir},
		},
		{
			name: "bare only",
			bare: true,
			want: []string{"clone", "--bare", repo, dir},
		},
		{
			name:   "branch only",
			branch: "main",
			want:   []string{"clone", "--branch", "main", repo, dir},
		},
		{
			name:  "depth only",
			depth: 1,
			want:  []string{"clone", "--depth", "1", repo, dir},
		},
		{
			name:  "depth zero not emitted",
			depth: 0,
			want:  []string{"clone", repo, dir},
		},
		{
			name:   "filter only",
			filter: "blob:none",
			want:   []string{"clone", "--filter=blob:none", repo, dir},
		},
		{
			name:       "no-checkout only",
			noCheckout: true,
			want:       []string{"clone", "--no-checkout", repo, dir},
		},
		{
			name:   "no-tags only",
			noTags: true,
			want:   []string{"clone", "--no-tags", repo, dir},
		},
		{
			name:   "origin only",
			origin: "upstream",
			want:   []string{"clone", "--origin", "upstream", repo, dir},
		},
		{
			name:              "recurse-submodules only",
			recurseSubmodules: true,
			want:              []string{"clone", "--recurse-submodules", repo, dir},
		},
		{
			name:         "single-branch only",
			singleBranch: true,
			want:         []string{"clone", "--single-branch", repo, dir},
		},
		{
			name:              "all known options together",
			bare:              true,
			branch:            "main",
			depth:             1,
			filter:            "blob:none",
			noCheckout:        true,
			noTags:            true,
			origin:            "upstream",
			recurseSubmodules: true,
			singleBranch:      true,
			want: []string{
				"clone",
				"--bare",
				"--branch", "main",
				"--depth", "1",
				"--filter=blob:none",
				"--no-checkout",
				"--no-tags",
				"--origin", "upstream",
				"--recurse-submodules",
				"--single-branch",
				repo, dir,
			},
		},
		{
			name:   "kwargs bool true emits flag",
			kwargs: map[string]any{"shallow_submodules": true},
			want:   []string{"clone", "--shallow-submodules", repo, dir},
		},
		{
			name:   "kwargs bool false omits flag",
			kwargs: map[string]any{"shallow_submodules": false},
			want:   []string{"clone", repo, dir},
		},
		{
			name:   "kwargs string emits flag=value",
			kwargs: map[string]any{"template": "/etc/git-template"},
			want:   []string{"clone", "--template=/etc/git-template", repo, dir},
		},
		{
			name:   "kwargs empty string omits flag",
			kwargs: map[string]any{"template": ""},
			want:   []string{"clone", repo, dir},
		},
		{
			name:   "kwargs int emits flag=value",
			kwargs: map[string]any{"jobs": 4},
			want:   []string{"clone", "--jobs=4", repo, dir},
		},
		{
			name:   "kwargs int64 emits flag=value",
			kwargs: map[string]any{"jobs": int64(4)},
			want:   []string{"clone", "--jobs=4", repo, dir},
		},
		{
			name:   "kwargs float emits flag=value",
			kwargs: map[string]any{"timeout": 1.5},
			want:   []string{"clone", "--timeout=1.5", repo, dir},
		},
		{
			name:   "kwargs nil is skipped",
			kwargs: map[string]any{"template": nil},
			want:   []string{"clone", repo, dir},
		},
		{
			name: "kwargs sorted alphabetically",
			kwargs: map[string]any{
				"zebra":  true,
				"alpha":  "one",
				"middle": 42,
			},
			want: []string{
				"clone",
				"--alpha=one",
				"--middle=42",
				"--zebra",
				repo, dir,
			},
		},
		{
			name:   "kwargs snake_case becomes kebab-case",
			kwargs: map[string]any{"no_single_branch": true},
			want:   []string{"clone", "--no-single-branch", repo, dir},
		},
		{
			name:   "known options emitted before kwargs",
			depth:  1,
			kwargs: map[string]any{"template": "/etc/git-template"},
			want: []string{
				"clone",
				"--depth", "1",
				"--template=/etc/git-template",
				repo, dir,
			},
		},
		{
			name:   "nil kwargs map behaves as empty",
			kwargs: nil,
			want:   []string{"clone", repo, dir},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCloneArgs(
				repo,
				dir,
				tt.bare,
				tt.branch,
				tt.depth,
				tt.filter,
				tt.noCheckout,
				tt.noTags,
				tt.origin,
				tt.recurseSubmodules,
				tt.singleBranch,
				tt.kwargs,
			)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildCloneArgs\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

// --- cleanControlChars ---

func TestCleanControlChars(t *testing.T) {

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple word passes through", input: "repo", want: "repo"},
		{name: "leading whitespace stripped", input: "   repo", want: "repo"},
		{name: "trailing whitespace stripped", input: "repo   ", want: "repo"},
		{name: "internal whitespace collapsed", input: "a    b", want: "a b"},
		{name: "control chars become spaces", input: "a\x01\x02b", want: "a b"},
		{name: "tabs and newlines collapse", input: "a\t\n\rb", want: "a b"},
		{name: "all whitespace becomes empty", input: "   \t\n  ", want: ""},
		{name: "empty stays empty", input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanControlChars(tt.input)
			if got != tt.want {
				t.Errorf("cleanControlChars(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
