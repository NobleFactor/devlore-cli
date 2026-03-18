// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn and returns what it wrote to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	orig := os.Stdout
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = orig

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	r.Close()

	return string(buf[:n])
}

func TestNewVersionCmd_FullOutput(t *testing.T) {
	info := VersionInfo{
		Version:   "1.2.3",
		Commit:    "abc1234",
		BuildDate: "2026-03-17T00:00:00Z",
	}

	cmd := NewVersionCmd(info)
	cmd.SetArgs(nil)

	output := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	for _, want := range []string{"1.2.3", "abc1234", "2026-03-17T00:00:00Z", "Go version:", "OS/Arch:"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q, got:\n%s", want, output)
		}
	}
}

func TestNewVersionCmd_ShortFlag(t *testing.T) {
	info := VersionInfo{
		Version:   "1.2.3",
		Commit:    "abc1234",
		BuildDate: "2026-03-17T00:00:00Z",
	}

	cmd := NewVersionCmd(info)
	cmd.SetArgs([]string{"--short"})

	output := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	trimmed := strings.TrimSpace(output)
	if trimmed != "1.2.3" {
		t.Errorf("short output = %q, want '1.2.3'", trimmed)
	}
}

func TestNewVersionCmd_DefaultsAreVisible(t *testing.T) {
	info := VersionInfo{
		Version:   "dev",
		Commit:    "none",
		BuildDate: "unknown",
	}

	cmd := NewVersionCmd(info)
	cmd.SetArgs(nil)

	output := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	for _, want := range []string{"dev", "none", "unknown"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing default %q, got:\n%s", want, output)
		}
	}
}
