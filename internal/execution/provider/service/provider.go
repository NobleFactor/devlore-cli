// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
)

// Provider provides platform-agnostic service management.
// Platform detection happens at runtime — callers don't need to know
// whether launchd, systemd, or Windows services are being used.
type Provider struct{}

// Start starts a service.
func (p *Provider) Start(name string, output io.Writer) error {
	switch runtime.GOOS {
	case "darwin":
		return run(output, "launchctl", "start", name)
	case "linux":
		return run(output, "sudo", "systemctl", "start", name)
	case "windows":
		return run(output, "sc", "start", name)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// Stop stops a service.
func (p *Provider) Stop(name string, output io.Writer) error {
	switch runtime.GOOS {
	case "darwin":
		return run(output, "launchctl", "stop", name)
	case "linux":
		return run(output, "sudo", "systemctl", "stop", name)
	case "windows":
		return run(output, "sc", "stop", name)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// Restart restarts a service.
func (p *Provider) Restart(name string, output io.Writer) error {
	switch runtime.GOOS {
	case "darwin":
		return run(output, "launchctl", "kickstart", "-k", "gui/"+name)
	case "linux":
		return run(output, "sudo", "systemctl", "restart", name)
	case "windows":
		// Stop (ignore error if already stopped), then start
		_ = run(output, "sc", "stop", name)
		return run(output, "sc", "start", name)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// Enable enables a service to start at boot.
func (p *Provider) Enable(name string, output io.Writer) error {
	switch runtime.GOOS {
	case "darwin":
		return run(output, "launchctl", "enable", "gui/"+name)
	case "linux":
		return run(output, "sudo", "systemctl", "enable", name)
	case "windows":
		return run(output, "sc", "config", name, "start=auto")
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// Disable disables a service from starting at boot.
func (p *Provider) Disable(name string, output io.Writer) error {
	switch runtime.GOOS {
	case "darwin":
		return run(output, "launchctl", "disable", "gui/"+name)
	case "linux":
		return run(output, "sudo", "systemctl", "disable", name)
	case "windows":
		return run(output, "sc", "config", name, "start=disabled")
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// run executes a command with output directed to the writer.
func run(output io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}
