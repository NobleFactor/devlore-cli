// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bindgen

import (
	"bufio"
	"context"
	"os/exec"
	"regexp"
	"strings"
)

// HelpParser extracts binding metadata from CLI --help output.
type HelpParser struct {
	command string
}

// NewHelpParser creates a parser for the given command.
func NewHelpParser(command string) *HelpParser {
	return &HelpParser{command: command}
}

// Parse runs --help and extracts a Command definition.
func (p *HelpParser) Parse(subcommand string) (*Command, error) {
	args := []string{"--help"}
	if subcommand != "" {
		args = []string{subcommand, "--help"}
	}

	cmd := exec.CommandContext(context.Background(), p.command, args...) //nolint:gosec // G204: command name from CLI tool registration
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Many CLIs exit non-zero for --help, check if we got output
		if len(output) == 0 {
			return nil, err
		}
	}

	return p.parseHelp(string(output), subcommand)
}

// parseHelp extracts command metadata from help text.
func (p *HelpParser) parseHelp(help, name string) (*Command, error) {
	cmd := &Command{
		Name:    name,
		Flags:   []*Flag{},
		Args:    []*Arg{},
		Returns: &Return{Type: "result", Fields: []string{"ok", "stdout", "stderr", "code"}},
	}

	// Extract description (usually first non-empty line or after "Description:")
	cmd.Description = p.extractDescription(help)

	// Extract flags
	cmd.Flags = p.extractFlags(help)

	return cmd, nil
}

// extractDescription gets the command description from help text.
func (p *HelpParser) extractDescription(help string) string {
	scanner := bufio.NewScanner(strings.NewReader(help))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and usage lines
		if line == "" || strings.HasPrefix(line, "Usage:") || strings.HasPrefix(line, "usage:") {
			continue
		}
		// First substantive line is often the description
		if !strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "Options") {
			return line
		}
		break
	}
	return ""
}

// extractFlags parses flag definitions from help text.
func (p *HelpParser) extractFlags(help string) []*Flag {
	var flags []*Flag

	// Common patterns:
	// -d, --detach           Run in background
	// --name string          Container name
	// -p, --publish list     Publish ports
	// --rm                   Remove after exit

	// Pattern for flags with short and long form
	shortLong := regexp.MustCompile(`^\s*-(\w),\s*--(\w[\w-]*)\s+(\w+)?\s*(.*)$`)
	// Pattern for long-only flags
	longOnly := regexp.MustCompile(`^\s*--(\w[\w-]*)\s+(\w+)?\s*(.*)$`)
	// Pattern for short-only flags
	shortOnly := regexp.MustCompile(`^\s*-(\w)\s+(\w+)?\s*(.*)$`)

	scanner := bufio.NewScanner(strings.NewReader(help))
	for scanner.Scan() {
		line := scanner.Text()

		if m := shortLong.FindStringSubmatch(line); m != nil {
			flags = append(flags, &Flag{
				Short:       m[1],
				Name:        m[2],
				Type:        p.inferType(m[3]),
				Description: strings.TrimSpace(m[4]),
			})
			continue
		}

		if m := longOnly.FindStringSubmatch(line); m != nil {
			flags = append(flags, &Flag{
				Name:        m[1],
				Type:        p.inferType(m[2]),
				Description: strings.TrimSpace(m[3]),
			})
			continue
		}

		if m := shortOnly.FindStringSubmatch(line); m != nil {
			flags = append(flags, &Flag{
				Short:       m[1],
				Type:        p.inferType(m[2]),
				Description: strings.TrimSpace(m[3]),
			})
		}
	}

	return flags
}

// inferType guesses the type from help text hints.
func (p *HelpParser) inferType(hint string) string {
	hint = strings.ToLower(hint)
	switch hint {
	case "":
		return "bool"
	case "string", "name", "value", "path", "url", "file", "dir":
		return "string"
	case "int", "number", "count", "n":
		return "int"
	case "list", "strings":
		return "string_list"
	case "map", "stringmap":
		return "string_map"
	default:
		return "string"
	}
}

// ListSubcommands attempts to extract subcommand names from help.
func (p *HelpParser) ListSubcommands() ([]string, error) {
	cmd := exec.CommandContext(context.Background(), p.command, "--help") //nolint:gosec // G204: command name from CLI tool registration
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return nil, err
	}

	return p.extractSubcommands(string(output)), nil
}

// extractSubcommands finds subcommand names in help text.
func (p *HelpParser) extractSubcommands(help string) []string {
	var cmds []string
	inCommands := false

	// Pattern for command listings like "  run       Run a container"
	cmdPattern := regexp.MustCompile(`^\s{2,}(\w[\w-]*)\s{2,}`)

	scanner := bufio.NewScanner(strings.NewReader(help))
	for scanner.Scan() {
		line := scanner.Text()
		lower := strings.ToLower(line)

		// Detect start of commands section
		if strings.Contains(lower, "commands:") || strings.Contains(lower, "available commands") {
			inCommands = true
			continue
		}

		// Detect end of commands section
		if inCommands && (strings.HasPrefix(line, "Options") || strings.HasPrefix(line, "Flags") || line == "") {
			if strings.TrimSpace(line) != "" {
				inCommands = false
			}
			continue
		}

		if inCommands {
			if m := cmdPattern.FindStringSubmatch(line); m != nil {
				cmds = append(cmds, m[1])
			}
		}
	}

	return cmds
}
