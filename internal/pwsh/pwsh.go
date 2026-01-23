// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package pwsh provides a persistent PowerShell session for lore.
//
// # Architecture: Why a persistent session?
//
// Starlark is the orchestration layer (logic, control flow, variables).
// The execution backend differs by platform:
//
//   - Unix: stateless shell.run() is fine — each command is a new process.
//   - Windows: a persistent PowerShell session is the natural backend.
//
// PowerShell modules (Az, ActiveDirectory, PackageManagement) establish
// authenticated sessions on import. COM/.NET objects (WMI, Registry, IIS)
// are expensive to instantiate. DSC operations require compile→test→apply
// within a single session. Spawning `pwsh -Command` for each operation
// loses all of this state.
//
// This package is intended as a Starlark binding (pwsh.run, pwsh.set) —
// the Starlark script decides what to do, the PowerShell session handles
// how on Windows.
//
// # Usage
//
// Commands run directly in session scope. Variables, functions, and module
// imports persist across calls. If a command calls `exit N`, the session
// terminates and the exit code is captured from the process state.
//
//	ps, _ := pwsh.New()
//	defer ps.Close()
//
//	ps.Run("$greeting = 'Hello'")
//	result := ps.Run("Write-Output $greeting")
//	fmt.Println(result.Stdout) // "Hello"
package pwsh

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ansiRe matches ANSI escape sequences (CSI and OSC).
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07`)

// Result captures the output of a PowerShell command.
type Result struct {
	Command  string        `json:"command"`
	Stdout   string        `json:"stdout,omitempty"`
	Stderr   string        `json:"stderr,omitempty"`
	ExitCode int           `json:"exit_code"`
	Start    time.Time     `json:"start"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

// OK returns true if the command succeeded.
func (r *Result) OK() bool {
	return r.ExitCode == 0 && r.Error == ""
}

// Failed returns true if the command failed.
func (r *Result) Failed() bool {
	return !r.OK()
}

// String returns a human-readable summary.
func (r *Result) String() string {
	if r.OK() {
		return fmt.Sprintf("[ok] %s (%s)", truncate(r.Command, 50), r.Duration.Round(time.Millisecond))
	}
	if r.Error != "" {
		return fmt.Sprintf("[error] %s: %s", truncate(r.Command, 50), r.Error)
	}
	return fmt.Sprintf("[exit %d] %s", r.ExitCode, truncate(r.Command, 50))
}

// JSON returns the result as indented JSON.
func (r *Result) JSON() string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// Session is a persistent PowerShell session.
type Session struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	scanner *bufio.Scanner
	errBuf  strings.Builder
	mu      sync.Mutex
	audit   io.Writer
	history []*Result
	dead    bool // true if the PowerShell process has exited

	// Marker used to detect end of command output
	marker string
}

// New creates and starts a new PowerShell session.
// Returns an error if PowerShell is not installed or fails to start.
func New() (*Session, error) {
	// Find PowerShell
	pwshPath, err := findPowerShell()
	if err != nil {
		return nil, err
	}

	// Start PowerShell with no profile for clean environment
	cmd := exec.Command(pwshPath, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "-")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start PowerShell: %w", err)
	}

	s := &Session{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		scanner: bufio.NewScanner(stdout),
		history: make([]*Result, 0),
		marker:  "__LORE_END__",
	}

	// Start stderr reader
	go s.readStderr()

	// Initialize session with helper function
	s.init()

	return s, nil
}

// findPowerShell locates the PowerShell executable.
func findPowerShell() (string, error) {
	// Try pwsh first (PowerShell 7+)
	if path, err := exec.LookPath("pwsh"); err == nil {
		return path, nil
	}

	// Fall back to powershell (Windows PowerShell)
	if path, err := exec.LookPath("powershell"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("PowerShell not found. Install PowerShell 7+: https://aka.ms/powershell")
}

// init drains any startup output from the PowerShell process.
func (s *Session) init() {
	// Emit a ready marker to consume any startup ANSI sequences or banners.
	fmt.Fprintf(s.stdin, "Write-Output '%sREADY'\n", s.marker)

	for s.scanner.Scan() {
		line := stripANSI(s.scanner.Text())
		if strings.HasPrefix(line, s.marker+"READY") {
			break
		}
	}
}

// readStderr continuously reads from stderr.
func (s *Session) readStderr() {
	scanner := bufio.NewScanner(s.stderr)
	for scanner.Scan() {
		s.mu.Lock()
		s.errBuf.WriteString(scanner.Text())
		s.errBuf.WriteString("\n")
		s.mu.Unlock()
	}
}

// Audit sets the audit log writer.
func (s *Session) Audit(w io.Writer) *Session {
	s.audit = w
	return s
}

// History returns all commands run in this session.
func (s *Session) History() []*Result {
	return s.history
}

// Run executes a PowerShell command and returns the result.
// Commands run directly in session scope, so variables persist across calls.
// If the command calls `exit N`, the session terminates and the exit code
// is captured from the process state.
func (s *Session) Run(command string) *Result {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := &Result{
		Command: command,
		Start:   time.Now(),
	}

	if s.dead {
		result.Error = "session terminated"
		result.ExitCode = 1
		s.history = append(s.history, result)
		return result
	}

	// Clear stderr buffer
	s.errBuf.Reset()

	// Send command directly in session scope, then emit a marker with the
	// exit code. Using $global:LASTEXITCODE preserves native command exit
	// codes; $? reflects PowerShell command success.
	fmt.Fprintf(s.stdin, "%s\n", command)
	fmt.Fprintf(s.stdin, "Write-Output '%s'$(if ($?) { $global:LASTEXITCODE } else { if ($global:LASTEXITCODE) { $global:LASTEXITCODE } else { 1 } })\n", s.marker)

	// Read output until we see the marker, stripping ANSI escape sequences.
	// If the scanner hits EOF (process exited), capture exit from process state.
	var stdout strings.Builder
	for s.scanner.Scan() {
		line := stripANSI(s.scanner.Text())
		if strings.HasPrefix(line, s.marker) {
			exitCodeStr := strings.TrimPrefix(line, s.marker)
			fmt.Sscanf(exitCodeStr, "%d", &result.ExitCode)
			break
		}
		stdout.WriteString(line)
		stdout.WriteString("\n")
	}

	// If scanner stopped without finding marker, the process exited (e.g., `exit N`)
	if !strings.HasPrefix(stripANSI(s.scanner.Text()), s.marker) {
		s.dead = true
		if err := s.cmd.Wait(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			} else {
				result.ExitCode = 1
				result.Error = err.Error()
			}
		}
	}

	result.Duration = time.Since(result.Start)
	result.Stdout = strings.TrimSuffix(stdout.String(), "\n")
	result.Stderr = strings.TrimSuffix(s.errBuf.String(), "\n")

	s.history = append(s.history, result)

	if s.audit != nil {
		fmt.Fprintln(s.audit, result.JSON())
	}

	return result
}

// Set sets a PowerShell variable for subsequent commands.
func (s *Session) Set(name, value string) *Session {
	s.Run(fmt.Sprintf("$%s = %s", name, quotePowerShell(value)))
	return s
}

// Must runs a command and panics if it fails.
func (s *Session) Must(command string) *Result {
	result := s.Run(command)
	if result.Failed() {
		panic(fmt.Sprintf("command failed: %s\n%s", result.String(), result.Stderr))
	}
	return result
}

// Script runs multiple commands in sequence, stopping on first failure.
func (s *Session) Script(commands ...string) *Result {
	var last *Result
	for _, cmd := range commands {
		last = s.Run(cmd)
		if last.Failed() {
			return last
		}
	}
	return last
}

// Close terminates the PowerShell session.
func (s *Session) Close() error {
	s.stdin.Close()
	if s.dead {
		return nil
	}
	return s.cmd.Wait()
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// quotePowerShell quotes a string for safe use in PowerShell.
func quotePowerShell(s string) string {
	// Use single quotes and escape embedded single quotes
	escaped := strings.ReplaceAll(s, "'", "''")
	return "'" + escaped + "'"
}

// Available returns true if PowerShell is installed.
func Available() bool {
	_, err := findPowerShell()
	return err == nil
}

// Version returns the PowerShell version string.
func Version() (string, error) {
	pwshPath, err := findPowerShell()
	if err != nil {
		return "", err
	}

	cmd := exec.Command(pwshPath, "-NoProfile", "-Command", "$PSVersionTable.PSVersion.ToString()")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Ensure checks that PowerShell 7+ is installed, returns an error if not.
func Ensure() error {
	ver, err := Version()
	if err != nil {
		return fmt.Errorf("PowerShell not found: %w\nInstall: https://aka.ms/powershell", err)
	}

	// Check major version
	var major int
	fmt.Sscanf(ver, "%d", &major)
	if major < 7 {
		return fmt.Errorf("PowerShell 7+ required, found %s\nInstall: https://aka.ms/powershell", ver)
	}

	return nil
}

// Bootstrap installs PowerShell if not present.
// This is platform-specific and may require elevated privileges.
func Bootstrap() error {
	if Available() {
		return Ensure()
	}

	// Platform-specific installation
	switch {
	case fileExists("/opt/homebrew/bin/brew") || fileExists("/usr/local/bin/brew"):
		cmd := exec.Command("brew", "install", "powershell")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()

	case fileExists("/usr/bin/apt-get"):
		// Ubuntu/Debian - requires adding Microsoft repo first
		return fmt.Errorf("run: sudo apt-get install -y powershell (after adding Microsoft repo)")

	default:
		return fmt.Errorf("install PowerShell manually: https://aka.ms/powershell")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
