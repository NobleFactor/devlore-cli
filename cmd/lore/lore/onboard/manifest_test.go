// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package onboard

import (
	"strings"
	"testing"
)

// The expectations in this file are hand-derived from the manifest generator's behavior, not
// captured from its output. Every byte asserted below — the `# PkgPath:` header, the `#\n`
// separators, the blank line between the header and the commands, the two-space annotation
// indent, and the trailing newline after each command — was read off the implementation and
// written down independently, so a change in behavior fails these tests rather than being
// absorbed by them.

// region generateManifest

func TestGenerateManifest(t *testing.T) {

	tests := []struct {
		name      string
		discovery *discoveryResult
		slots     []ExtractedSlot
		want      string
	}{
		{
			name: "product header, platform-qualified command, canonical name already present",
			discovery: &discoveryResult{
				Product: &ProductInfo{
					Name:          "ripgrep",
					CanonicalName: "ripgrep",
					Vendor:        "BurntSushi",
					Version:       "14.1.0",
				},
			},
			slots: []ExtractedSlot{
				{Name: "install_command", Value: "brew install ripgrep", Platform: "Darwin"},
			},
			want: "# PkgPath: ripgrep\n" +
				"# Vendor: BurntSushi\n" +
				"# Version: 14.1.0\n" +
				"#\n" +
				"\n" +
				"# Platform: Darwin\n" +
				"brew install ripgrep\n" +
				"\n",
		},
		{
			name:      "no product and no slots yields only the section separator",
			discovery: &discoveryResult{},
			slots:     nil,
			want:      "\n",
		},
		{
			name: "vendor and version are omitted when empty",
			discovery: &discoveryResult{
				Product: &ProductInfo{Name: "fd", CanonicalName: "fd"},
			},
			slots: nil,
			want: "# PkgPath: fd\n" +
				"#\n" +
				"\n",
		},
		{
			name: "a complex rating emits the warning block and every concern",
			discovery: &discoveryResult{
				Complexity: &ComplexityInfo{
					Rating:   "complex",
					Concerns: []string{"requires sudo", "builds a kernel module"},
				},
			},
			slots: nil,
			want: "# WARNING: Complex installation\n" +
				"#   - requires sudo\n" +
				"#   - builds a kernel module\n" +
				"#\n" +
				"\n",
		},
		{
			name: "a non-complex rating emits nothing",
			discovery: &discoveryResult{
				Complexity: &ComplexityInfo{Rating: "moderate", Concerns: []string{"ignored"}},
			},
			slots: nil,
			want:  "\n",
		},
		{
			name: "the placeholder fires when the canonical name is absent from the document",
			discovery: &discoveryResult{
				Product: &ProductInfo{Name: "rg", CanonicalName: "ripgrep"},
			},
			slots: nil,
			want: "# PkgPath: rg\n" +
				"#\n" +
				"\n" +
				"# TODO: Add installation method for ripgrep\n" +
				"# ripgrep\n",
		},
		{
			name:      "platform \"all\" is not qualified",
			discovery: &discoveryResult{},
			slots: []ExtractedSlot{
				{Name: "install_command", Value: "apt install ripgrep", Platform: "all"},
			},
			want: "\napt install ripgrep\n\n",
		},
		{
			name:      "an empty platform is not qualified",
			discovery: &discoveryResult{},
			slots: []ExtractedSlot{
				{Name: "install_command", Value: "cargo install ripgrep"},
			},
			want: "\ncargo install ripgrep\n\n",
		},
		{
			name:      "annotations follow their command, indented two spaces",
			discovery: &discoveryResult{},
			slots: []ExtractedSlot{
				{
					Name:        "package_manager",
					Value:       "brew",
					Annotations: []string{"requires a tap", "v2 only"},
				},
			},
			want: "\nbrew\n  # requires a tap\n  # v2 only\n\n",
		},
		{
			// Pins the dropped `len(slot.Annotations) > 0` guard: ranging an empty slice is
			// already a no-op, so removing the guard must not change a byte.
			name:      "a slot with no annotations emits only its command",
			discovery: &discoveryResult{},
			slots: []ExtractedSlot{
				{Name: "install_command", Value: "apk add ripgrep"},
			},
			want: "\napk add ripgrep\n\n",
		},
		{
			name:      "slots that are neither install_command nor package_manager are skipped",
			discovery: &discoveryResult{},
			slots: []ExtractedSlot{
				{Name: "config_path", Value: "/etc/ripgrep"},
				{Name: "install_command", Value: "dnf install ripgrep"},
				{Name: "license", Value: "MIT"},
			},
			want: "\ndnf install ripgrep\n\n",
		},
		{
			name:      "multiple matching slots each get their own block",
			discovery: &discoveryResult{},
			slots: []ExtractedSlot{
				{Name: "install_command", Value: "brew install fd", Platform: "Darwin"},
				{Name: "package_manager", Value: "apt", Platform: "Linux"},
			},
			want: "\n" +
				"# Platform: Darwin\n" +
				"brew install fd\n" +
				"\n" +
				"# Platform: Linux\n" +
				"apt\n" +
				"\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			got := generateManifest(test.discovery, test.slots)

			if got != test.want {
				t.Errorf("generateManifest() mismatch\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

// endregion

// region writeProductHeader

func TestWriteProductHeader(t *testing.T) {

	tests := []struct {
		name      string
		discovery *discoveryResult
		want      string
	}{
		{
			name:      "a nil product writes nothing",
			discovery: &discoveryResult{},
			want:      "",
		},
		{
			name: "name only, followed by the separator",
			discovery: &discoveryResult{
				Product: &ProductInfo{Name: "fd"},
			},
			want: "# PkgPath: fd\n#\n",
		},
		{
			name: "vendor is written when present",
			discovery: &discoveryResult{
				Product: &ProductInfo{Name: "fd", Vendor: "sharkdp"},
			},
			want: "# PkgPath: fd\n# Vendor: sharkdp\n#\n",
		},
		{
			name: "version is written when present",
			discovery: &discoveryResult{
				Product: &ProductInfo{Name: "fd", Version: "10.2.0"},
			},
			want: "# PkgPath: fd\n# Version: 10.2.0\n#\n",
		},
		{
			name: "vendor precedes version",
			discovery: &discoveryResult{
				Product: &ProductInfo{Name: "fd", Vendor: "sharkdp", Version: "10.2.0"},
			},
			want: "# PkgPath: fd\n# Vendor: sharkdp\n# Version: 10.2.0\n#\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			var sb strings.Builder
			writeProductHeader(&sb, test.discovery)

			if got := sb.String(); got != test.want {
				t.Errorf("writeProductHeader() mismatch\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

// endregion

// region writeComplexityWarning

func TestWriteComplexityWarning(t *testing.T) {

	tests := []struct {
		name      string
		discovery *discoveryResult
		want      string
	}{
		{
			name:      "nil complexity writes nothing",
			discovery: &discoveryResult{},
			want:      "",
		},
		{
			name: "a simple rating writes nothing",
			discovery: &discoveryResult{
				Complexity: &ComplexityInfo{Rating: "simple", Concerns: []string{"ignored"}},
			},
			want: "",
		},
		{
			name: "a moderate rating writes nothing",
			discovery: &discoveryResult{
				Complexity: &ComplexityInfo{Rating: "moderate", Concerns: []string{"ignored"}},
			},
			want: "",
		},
		{
			name: "a complex rating with no concerns still writes the banner",
			discovery: &discoveryResult{
				Complexity: &ComplexityInfo{Rating: "complex"},
			},
			want: "# WARNING: Complex installation\n#\n",
		},
		{
			name: "every concern is written, indented",
			discovery: &discoveryResult{
				Complexity: &ComplexityInfo{
					Rating:   "complex",
					Concerns: []string{"requires sudo", "builds a kernel module"},
				},
			},
			want: "# WARNING: Complex installation\n" +
				"#   - requires sudo\n" +
				"#   - builds a kernel module\n" +
				"#\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			var sb strings.Builder
			writeComplexityWarning(&sb, test.discovery)

			if got := sb.String(); got != test.want {
				t.Errorf("writeComplexityWarning() mismatch\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

// endregion

// region writeInstallCommands

func TestWriteInstallCommands(t *testing.T) {

	tests := []struct {
		name  string
		slots []ExtractedSlot
		want  string
	}{
		{
			name:  "no slots writes nothing",
			slots: nil,
			want:  "",
		},
		{
			name: "install_command is written",
			slots: []ExtractedSlot{
				{Name: "install_command", Value: "brew install fd"},
			},
			want: "brew install fd\n\n",
		},
		{
			name: "package_manager is written",
			slots: []ExtractedSlot{
				{Name: "package_manager", Value: "brew"},
			},
			want: "brew\n\n",
		},
		{
			name: "any other slot name is skipped",
			slots: []ExtractedSlot{
				{Name: "config_path", Value: "/etc/fd"},
			},
			want: "",
		},
		{
			name: "a named platform is qualified",
			slots: []ExtractedSlot{
				{Name: "install_command", Value: "brew install fd", Platform: "Darwin"},
			},
			want: "# Platform: Darwin\nbrew install fd\n\n",
		},
		{
			name: "platform \"all\" is not qualified",
			slots: []ExtractedSlot{
				{Name: "install_command", Value: "apt install fd", Platform: "all"},
			},
			want: "apt install fd\n\n",
		},
		{
			name: "annotations are indented two spaces and follow the command",
			slots: []ExtractedSlot{
				{Name: "install_command", Value: "brew install fd", Annotations: []string{"a", "b"}},
			},
			want: "brew install fd\n  # a\n  # b\n\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			var sb strings.Builder
			writeInstallCommands(&sb, test.slots)

			if got := sb.String(); got != test.want {
				t.Errorf("writeInstallCommands() mismatch\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

// endregion
