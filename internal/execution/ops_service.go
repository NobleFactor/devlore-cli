// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"fmt"
	"os/exec"
)

// =============================================================================
// Service Manager Operations
// =============================================================================
//
// Platform-specific service operations. The platform bindings (darwin.go,
// linux.go, windows.go) create nodes with the appropriate operation name
// (e.g., launchd-start, systemd-start, winservice-start). No runtime
// platform detection is needed - the binding layer handles platform selection.

// =============================================================================
// launchd (macOS)
// =============================================================================

// LaunchdStartOp starts a launchd service (macOS).
type LaunchdStartOp struct{}

func (o *LaunchdStartOp) Name() string { return "launchd-start" }

func (o *LaunchdStartOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("launchd-start: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] launchctl start %s\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] launchctl start %s\n", service)
	cmd := exec.Command("launchctl", "start", service)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// LaunchdStopOp stops a launchd service (macOS).
type LaunchdStopOp struct{}

func (o *LaunchdStopOp) Name() string { return "launchd-stop" }

func (o *LaunchdStopOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("launchd-stop: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] launchctl stop %s\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] launchctl stop %s\n", service)
	cmd := exec.Command("launchctl", "stop", service)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// LaunchdRestartOp restarts a launchd service (macOS).
type LaunchdRestartOp struct{}

func (o *LaunchdRestartOp) Name() string { return "launchd-restart" }

func (o *LaunchdRestartOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("launchd-restart: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] launchctl kickstart -k %s\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] launchctl kickstart -k %s\n", service)
	// kickstart -k kills and restarts the service
	cmd := exec.Command("launchctl", "kickstart", "-k", "gui/"+service)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// LaunchdEnableOp enables a launchd service at boot (macOS).
type LaunchdEnableOp struct{}

func (o *LaunchdEnableOp) Name() string { return "launchd-enable" }

func (o *LaunchdEnableOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("launchd-enable: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] launchctl enable gui/%s\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] launchctl enable gui/%s\n", service)
	cmd := exec.Command("launchctl", "enable", "gui/"+service)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// LaunchdDisableOp disables a launchd service at boot (macOS).
type LaunchdDisableOp struct{}

func (o *LaunchdDisableOp) Name() string { return "launchd-disable" }

func (o *LaunchdDisableOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("launchd-disable: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] launchctl disable gui/%s\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] launchctl disable gui/%s\n", service)
	cmd := exec.Command("launchctl", "disable", "gui/"+service)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// =============================================================================
// systemd (Linux)
// =============================================================================

// SystemdStartOp starts a systemd service (Linux).
type SystemdStartOp struct{}

func (o *SystemdStartOp) Name() string { return "systemd-start" }

func (o *SystemdStartOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("systemd-start: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] sudo systemctl start %s\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] sudo systemctl start %s\n", service)
	cmd := exec.Command("sudo", "systemctl", "start", service)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// SystemdStopOp stops a systemd service (Linux).
type SystemdStopOp struct{}

func (o *SystemdStopOp) Name() string { return "systemd-stop" }

func (o *SystemdStopOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("systemd-stop: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] sudo systemctl stop %s\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] sudo systemctl stop %s\n", service)
	cmd := exec.Command("sudo", "systemctl", "stop", service)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// SystemdRestartOp restarts a systemd service (Linux).
type SystemdRestartOp struct{}

func (o *SystemdRestartOp) Name() string { return "systemd-restart" }

func (o *SystemdRestartOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("systemd-restart: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] sudo systemctl restart %s\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] sudo systemctl restart %s\n", service)
	cmd := exec.Command("sudo", "systemctl", "restart", service)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// SystemdEnableOp enables a systemd service at boot (Linux).
type SystemdEnableOp struct{}

func (o *SystemdEnableOp) Name() string { return "systemd-enable" }

func (o *SystemdEnableOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("systemd-enable: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] sudo systemctl enable %s\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] sudo systemctl enable %s\n", service)
	cmd := exec.Command("sudo", "systemctl", "enable", service)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// SystemdDisableOp disables a systemd service at boot (Linux).
type SystemdDisableOp struct{}

func (o *SystemdDisableOp) Name() string { return "systemd-disable" }

func (o *SystemdDisableOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("systemd-disable: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] sudo systemctl disable %s\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] sudo systemctl disable %s\n", service)
	cmd := exec.Command("sudo", "systemctl", "disable", service)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// =============================================================================
// Windows Services
// =============================================================================

// WinServiceStartOp starts a Windows service.
type WinServiceStartOp struct{}

func (o *WinServiceStartOp) Name() string { return "winservice-start" }

func (o *WinServiceStartOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("winservice-start: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] sc start %s\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] sc start %s\n", service)
	cmd := exec.Command("sc", "start", service)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// WinServiceStopOp stops a Windows service.
type WinServiceStopOp struct{}

func (o *WinServiceStopOp) Name() string { return "winservice-stop" }

func (o *WinServiceStopOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("winservice-stop: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] sc stop %s\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] sc stop %s\n", service)
	cmd := exec.Command("sc", "stop", service)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// WinServiceRestartOp restarts a Windows service (stop + start).
type WinServiceRestartOp struct{}

func (o *WinServiceRestartOp) Name() string { return "winservice-restart" }

func (o *WinServiceRestartOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("winservice-restart: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] sc stop %s && sc start %s\n", service, service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] restarting %s\n", service)

	// Stop
	stopCmd := exec.Command("sc", "stop", service)
	stopCmd.Stdout = ctx.Logger
	stopCmd.Stderr = ctx.Logger
	_ = stopCmd.Run() // Ignore error if service is already stopped

	// Start
	startCmd := exec.Command("sc", "start", service)
	startCmd.Stdout = ctx.Logger
	startCmd.Stderr = ctx.Logger
	return startCmd.Run()
}

// WinServiceEnableOp configures a Windows service to start automatically.
type WinServiceEnableOp struct{}

func (o *WinServiceEnableOp) Name() string { return "winservice-enable" }

func (o *WinServiceEnableOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("winservice-enable: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] sc config %s start=auto\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] sc config %s start=auto\n", service)
	cmd := exec.Command("sc", "config", service, "start=auto")
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// WinServiceDisableOp configures a Windows service to be disabled.
type WinServiceDisableOp struct{}

func (o *WinServiceDisableOp) Name() string { return "winservice-disable" }

func (o *WinServiceDisableOp) Execute(ctx *Context, node Executable) error {
	service, _ := node.GetSlot("name").(string)
	if service == "" {
		return fmt.Errorf("winservice-disable: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] sc config %s start=disabled\n", service)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[service] sc config %s start=disabled\n", service)
	cmd := exec.Command("sc", "config", service, "start=disabled")
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// ServiceOps returns all service manager operations for registration.
func ServiceOps() []Operation {
	return []Operation{
		// launchd (macOS)
		&LaunchdStartOp{},
		&LaunchdStopOp{},
		&LaunchdRestartOp{},
		&LaunchdEnableOp{},
		&LaunchdDisableOp{},
		// systemd (Linux)
		&SystemdStartOp{},
		&SystemdStopOp{},
		&SystemdRestartOp{},
		&SystemdEnableOp{},
		&SystemdDisableOp{},
		// Windows Services
		&WinServiceStartOp{},
		&WinServiceStopOp{},
		&WinServiceRestartOp{},
		&WinServiceEnableOp{},
		&WinServiceDisableOp{},
	}
}
