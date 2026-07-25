// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import "testing"

// --- IsEncrypted ---

func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			"sops json metadata",
			[]byte(`{"secret":"ENC[AES256_GCM,data:abc]","sops":{"kms":[],"age":[]}}`),
			true,
		},
		{
			"sops yaml metadata",
			[]byte("secret: ENC[AES256_GCM,data:abc]\nsops:\n    age: []\n"),
			true,
		},
		{
			"age armored header",
			[]byte("-----BEGIN AGE ENCRYPTED FILE-----\nYWdlLWVuY3J5cHRpb24=\n-----END AGE ENCRYPTED FILE-----\n"),
			true,
		},
		{
			"age binary format",
			[]byte("age-encryption.org/v1\n-> X25519 abc\ndata\n"),
			true,
		},
		{
			"plaintext yaml",
			[]byte("name: sshd\nport: 22\n"),
			false,
		},
		{
			"plaintext json",
			[]byte(`{"name":"sshd","port":22}`),
			false,
		},
		{
			"plain string",
			[]byte("hello world"),
			false,
		},
		{
			"empty data",
			[]byte{},
			false,
		},
		{
			"nil data",
			nil,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEncrypted(tt.data); got != tt.want {
				t.Errorf("IsEncrypted() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- IsSecretFile ---

func TestIsSecretFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{"sops yaml", "config.sops.yaml", true},
		{"sops json", "secrets.sops.json", true},
		{"bare sops", "credentials.sops", true},
		{"uppercase sops yaml", "Config.SOPS.YAML", true},
		{"mixed case sops json", "Secrets.Sops.Json", true},
		{"uppercase bare sops", "CREDENTIALS.SOPS", true},
		{"plain yaml", "config.yaml", false},
		{"plain json", "config.json", false},
		{"sops in middle", "sops.config.yaml", false},
		{"no extension", "secrets", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSecretFile(tt.filename); got != tt.want {
				t.Errorf("IsSecretFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}
