// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package credentials

import (
	"bytes"
	"os/exec"
	"runtime"
	"strings"
)

// serviceName is the reverse-DNS identifier for keychain entries.
const serviceName = "com.noblefactor.DevLore"

// detectHelper returns the native keychain method for this platform.
// Returns empty string if no native keychain is available.
func detectHelper() string {
	switch runtime.GOOS {
	case "darwin":
		// macOS always has the security command
		if _, err := exec.LookPath("security"); err == nil {
			return "security"
		}
	case "linux":
		// Check for secret-tool (libsecret)
		if _, err := exec.LookPath("secret-tool"); err == nil {
			return "secret-tool"
		}
	case "windows":
		// Check for PowerShell
		if _, err := exec.LookPath("powershell"); err == nil {
			return "powershell"
		}
	}
	return ""
}

// helperGet retrieves a credential from the native keychain.
func helperGet(helper, key string) (string, error) {
	switch helper {
	case "security":
		return macOSGet(key)
	case "secret-tool":
		return linuxGet(key)
	case "powershell":
		return windowsGet(key)
	default:
		return "", nil
	}
}

// helperStore saves a credential to the native keychain.
func helperStore(helper, key, secret string) error {
	switch helper {
	case "security":
		return macOSStore(key, secret)
	case "secret-tool":
		return linuxStore(key, secret)
	case "powershell":
		return windowsStore(key, secret)
	default:
		return nil
	}
}

// helperErase removes a credential from the native keychain.
func helperErase(helper, key string) error {
	switch helper {
	case "security":
		return macOSErase(key)
	case "secret-tool":
		return linuxErase(key)
	case "powershell":
		return windowsErase(key)
	default:
		return nil
	}
}

// macOS: use security command (Keychain)
func macOSGet(key string) (string, error) {
	cmd := exec.Command("security", "find-generic-password",
		"-s", serviceName,
		"-a", key,
		"-w")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func macOSStore(account, secret string) error {
	label := "DevLore: " + account
	// -U updates if exists, creates if not
	cmd := exec.Command("security", "add-generic-password",
		"-s", serviceName,
		"-a", account,
		"-l", label,
		"-w", secret,
		"-U")
	return cmd.Run()
}

func macOSErase(key string) error {
	cmd := exec.Command("security", "delete-generic-password",
		"-s", serviceName,
		"-a", key)
	return cmd.Run()
}

// Linux: use secret-tool (libsecret / GNOME Keyring / KWallet)
func linuxGet(key string) (string, error) {
	cmd := exec.Command("secret-tool", "lookup",
		"service", serviceName,
		"account", key)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func linuxStore(key, secret string) error {
	cmd := exec.Command("secret-tool", "store",
		"--label", "DevLore: "+key,
		"service", serviceName,
		"account", key)
	cmd.Stdin = strings.NewReader(secret)
	return cmd.Run()
}

func linuxErase(key string) error {
	cmd := exec.Command("secret-tool", "clear",
		"service", serviceName,
		"account", key)
	return cmd.Run()
}

// Windows: use PowerShell with Windows Credential Manager (PasswordVault)
func windowsGet(account string) (string, error) {
	script := `
[Windows.Security.Credentials.PasswordVault,Windows.Security.Credentials,ContentType=WindowsRuntime] | Out-Null
$vault = New-Object Windows.Security.Credentials.PasswordVault
try {
    $cred = $vault.Retrieve("` + serviceName + `", "` + account + `")
    $cred.RetrievePassword()
    Write-Output $cred.Password
} catch { exit 1 }
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func windowsStore(account, secret string) error {
	// Escape single quotes for PowerShell string
	escapedSecret := strings.ReplaceAll(secret, "'", "''")
	script := `
[Windows.Security.Credentials.PasswordVault,Windows.Security.Credentials,ContentType=WindowsRuntime] | Out-Null
$vault = New-Object Windows.Security.Credentials.PasswordVault
try { $vault.Remove($vault.Retrieve("` + serviceName + `", "` + account + `")) } catch {}
$cred = New-Object Windows.Security.Credentials.PasswordCredential("` + serviceName + `", "` + account + `", '` + escapedSecret + `')
$vault.Add($cred)
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	return cmd.Run()
}

func windowsErase(account string) error {
	script := `
[Windows.Security.Credentials.PasswordVault,Windows.Security.Credentials,ContentType=WindowsRuntime] | Out-Null
$vault = New-Object Windows.Security.Credentials.PasswordVault
try {
    $cred = $vault.Retrieve("` + serviceName + `", "` + account + `")
    $vault.Remove($cred)
} catch {}
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	return cmd.Run()
}
