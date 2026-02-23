// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package secrets

import "testing"

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name string
		path string
		data []byte
		want string
	}{
		{"yaml extension", "config.yaml", nil, "yaml"},
		{"yml extension", "config.yml", nil, "yaml"},
		{"json extension", "config.json", nil, "json"},
		{"env extension", "config.env", nil, "dotenv"},
		{"ini extension", "config.ini", nil, "ini"},
		{
			"yaml.sops inner extension",
			"config.yaml.sops",
			nil,
			"yaml",
		},
		{
			"json.sops inner extension",
			"config.json.sops",
			nil,
			"json",
		},
		{
			"env.sops inner extension",
			"config.env.sops",
			nil,
			"dotenv",
		},
		{
			"ini.sops inner extension",
			"config.ini.sops",
			nil,
			"ini",
		},
		{
			"json content detection",
			"secretblob",
			[]byte(`{"sops":{"age":[]}}`),
			"json",
		},
		{
			"age armor content detection",
			"secretblob",
			[]byte("-----BEGIN AGE ENCRYPTED FILE-----\ndata\n"),
			"binary",
		},
		{
			"unknown extension empty data",
			"secretblob.bin",
			nil,
			"binary",
		},
		{
			"unknown extension nonempty data",
			"secretblob.bin",
			[]byte("opaque binary blob"),
			"binary",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectFormat(tt.path, tt.data); got != tt.want {
				t.Errorf("detectFormat(%q, ...) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
