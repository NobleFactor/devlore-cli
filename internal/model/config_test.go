// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package model

import (
	"bufio"
	"strings"
	"testing"
)

// The provider table and the key reader are the extracted seams of promptForProvider. The prompt
// itself reads os.Stdin, probes an ambient Ollama, and persists the user's real configuration,
// so it is not exercised here; the orchestration rests on review (the sanctioned deviation in
// docs/plans/audit-remediation.md, phase 1b-ii).

func TestProviderChoices_MapEveryMenuEntry(t *testing.T) {

	tests := []struct {
		choice   string
		provider string
		model    string
	}{
		{"", "groq", "llama-3.3-70b-versatile"},
		{"1", "groq", "llama-3.3-70b-versatile"},
		{"2", "gemini", "gemini-2.5-flash"},
		{"3", "anthropic", "claude-sonnet-4-20250514"},
		{"4", "openai", "gpt-4o-mini"},
	}

	for _, test := range tests {
		t.Run("choice "+test.choice, func(t *testing.T) {

			option, known := providerChoices[test.choice]
			if !known {
				t.Fatalf("choice %q is not in the table", test.choice)
			}
			if option.Provider != test.provider {
				t.Errorf("Provider = %q, want %q", option.Provider, test.provider)
			}
			if option.Model != test.model {
				t.Errorf("Model = %q, want %q", option.Model, test.model)
			}
			if option.KeyPrompt == "" {
				t.Error("KeyPrompt is empty")
			}
		})
	}

	if len(providerChoices) != len(tests) {
		t.Errorf("table has %d entries, tests cover %d — keep them in step",
			len(providerChoices), len(tests))
	}
}

func TestProviderChoices_EmptySelectionAliasesTheRecommendation(t *testing.T) {

	if providerChoices[""] != providerChoices["1"] {
		t.Errorf("empty choice = %+v, want the %+v recommended by the menu",
			providerChoices[""], providerChoices["1"])
	}
}

func TestReadAPIKey_TrimsSurroundingWhitespace(t *testing.T) {

	reader := bufio.NewReader(strings.NewReader("  sk-test-123  \n"))

	key, err := readAPIKey(reader, "key: ")

	if err != nil {
		t.Fatalf("readAPIKey: %v", err)
	}
	if key != "sk-test-123" {
		t.Errorf("key = %q, want %q", key, "sk-test-123")
	}
}

func TestReadAPIKey_PropagatesAFailedRead(t *testing.T) {

	reader := bufio.NewReader(strings.NewReader("input without a trailing newline"))

	if _, err := readAPIKey(reader, "key: "); err == nil {
		t.Fatal("expected an error for input without a newline")
	}
}
