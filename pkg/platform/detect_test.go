// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"runtime"
	"testing"
)

// region Detect smoke

// TestDetectReturnsHostPlatform exercises the [Detect] dispatch. The build-tagged detect_<os>.go file
// for the running host runs; the assertions are limited to what we can know cross-platform — the
// returned platform's OS matches runtime.GOOS on supported hosts, or [Detect] errors on unsupported
// ones.
func TestDetectReturnsHostPlatform(t *testing.T) {

	got, err := Detect()

	switch runtime.GOOS {
	case "linux", "darwin", "windows":
		if err != nil {
			t.Fatalf("Detect on %s returned error: %v", runtime.GOOS, err)
		}
		if got == nil {
			t.Fatal("Detect returned nil platform on supported host")
		}
		if got.OS() != runtime.GOOS {
			t.Errorf("Detect.OS() = %q, want %q", got.OS(), runtime.GOOS)
		}
		if got.Arch() == "" {
			t.Error("Detect.Arch() is empty; expected runtime.GOARCH default")
		}
		if got.DefaultPackageManager() == nil {
			t.Error("Detect.DefaultPackageManager() is nil")
		}
	default:
		if err == nil {
			t.Fatalf("Detect on unsupported OS %q returned nil error", runtime.GOOS)
		}
	}
}

// endregion
