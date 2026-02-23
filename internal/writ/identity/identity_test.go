// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error: %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "tilde slash expands to home dir",
			path: "~/config/file",
			want: filepath.Join(home, "config", "file"),
		},
		{
			name: "absolute path unchanged",
			path: "/absolute/path",
			want: "/absolute/path",
		},
		{
			name: "relative path unchanged",
			path: "relative/path",
			want: "relative/path",
		},
		{
			name: "empty string unchanged",
			path: "",
			want: "",
		},
		{
			name: "tilde without slash unchanged",
			path: "~nothome",
			want: "~nothome",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.path)
			if got != tt.want {
				t.Errorf("expandPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestGenerateIdentity(t *testing.T) {
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() error: %v", err)
	}
	if id == nil {
		t.Fatal("GenerateIdentity() returned nil identity")
	}
}

func TestGenerateIdentityUnique(t *testing.T) {
	id1, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() first call error: %v", err)
	}
	id2, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() second call error: %v", err)
	}
	r1 := ToRecipient(id1)
	r2 := ToRecipient(id2)
	if r1 == r2 {
		t.Errorf("two generated identities have the same recipient: %q", r1)
	}
}

func TestToRecipient(t *testing.T) {
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() error: %v", err)
	}
	recipient := ToRecipient(id)
	if recipient == "" {
		t.Fatal("ToRecipient() returned empty string")
	}
	if !strings.HasPrefix(recipient, "age1") {
		t.Errorf("ToRecipient() = %q, want prefix %q", recipient, "age1")
	}
}

func TestParseRecipients(t *testing.T) {
	// Generate a valid recipient string for testing.
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() error: %v", err)
	}
	validRecipient := ToRecipient(id)

	t.Run("valid age recipient", func(t *testing.T) {
		result, err := ParseRecipients([]string{validRecipient})
		if err != nil {
			t.Fatalf("ParseRecipients() error: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("ParseRecipients() returned %d recipients, want 1", len(result))
		}
	})

	t.Run("empty strings skipped", func(t *testing.T) {
		result, err := ParseRecipients([]string{"", "  "})
		if err != nil {
			t.Fatalf("ParseRecipients() error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("ParseRecipients() returned %d recipients, want 0", len(result))
		}
	})

	t.Run("invalid recipient returns error", func(t *testing.T) {
		_, err := ParseRecipients([]string{"not-a-recipient"})
		if err == nil {
			t.Fatal("ParseRecipients() expected error for invalid recipient, got nil")
		}
	})

	t.Run("nonexistent file path returns error", func(t *testing.T) {
		_, err := ParseRecipients([]string{"/nonexistent/file"})
		if err == nil {
			t.Fatal("ParseRecipients() expected error for nonexistent file, got nil")
		}
		if !strings.Contains(err.Error(), "load recipients from") {
			t.Errorf("ParseRecipients() error = %q, want it to contain %q", err.Error(), "load recipients from")
		}
	})
}
