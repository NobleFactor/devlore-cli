// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package docgen generates Docker-style CLI reference documentation
// in Markdown with Astro frontmatter from Cobra command trees.
package docgen

import (
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// PageData holds all data needed to render a CLI reference page.
type PageData struct {
	Title       string
	Description string
	Tool        string
	Command     string
	Parent      string
	Generated   string
	Version     string
	Usage       string
	Long        string
	Options     []FlagInfo
	GlobalFlags []FlagInfo
	Examples    string
	ParentCmd   *ParentInfo
	Children    []ChildInfo
}

// FlagInfo holds display information for a single flag.
type FlagInfo struct {
	Name        string
	Default     string
	Description string
}

// ParentInfo holds a reference to the parent command.
type ParentInfo struct {
	Name        string
	Description string
	Path        string
}

// ChildInfo holds a reference to a child command.
type ChildInfo struct {
	Name        string
	Description string
	Path        string
}

var pageTemplate = template.Must(template.New("page").Parse(`---
title: "{{ .Title }}"
description: "{{ .Description }}"
tool: "{{ .Tool }}"
command: "{{ .Command }}"
parent: "{{ .Parent }}"
generated: "{{ .Generated }}"
version: "{{ .Version }}"
---

# {{ .Title }}

{{ .Description }}

## Usage

` + "```" + `
{{ .Usage }}
` + "```" + `
{{ if .Long }}
## Description

{{ .Long }}
{{ end }}{{ if .Options }}
## Options

| Option | Default | Description |
|--------|---------|-------------|
{{ range .Options }}| ` + "`{{ .ReceiverName }}`" + ` | ` + "`{{ .Default }}`" + ` | {{ .Description }} |
{{ end }}{{ end }}{{ if .GlobalFlags }}
## Global Options

| Option | Default | Description |
|--------|---------|-------------|
{{ range .GlobalFlags }}| ` + "`{{ .ReceiverName }}`" + ` | ` + "`{{ .Default }}`" + ` | {{ .Description }} |
{{ end }}{{ end }}{{ if .Examples }}
## Examples

` + "```bash" + `
{{ .Examples }}
` + "```" + `
{{ end }}{{ if .ParentCmd }}
## Parent Command

| Command | Description |
|---------|-------------|
| [{{ .ParentCmd.ReceiverName }}]({{ .ParentCmd.Path }}) | {{ .ParentCmd.Description }} |
{{ end }}{{ if .Children }}
## Child Commands

| Command | Description |
|---------|-------------|
{{ range .Children }}| [{{ .ReceiverName }}]({{ .Path }}) | {{ .Description }} |
{{ end }}{{ end }}`))

// BuildPageData extracts page data from a Cobra command.
func BuildPageData(cmd *cobra.Command, toolName, version string) PageData {
	fullName := fullCommandName(cmd)
	parent := parentCommandName(cmd)

	data := PageData{
		Title:       fullName,
		Description: cmd.Short,
		Tool:        toolName,
		Command:     cmd.Name(),
		Parent:      parent,
		Generated:   time.Now().UTC().Format(time.RFC3339),
		Version:     version,
		Usage:       cmd.UseLine(),
		Long:        strings.TrimSpace(cmd.Long),
		Options:     collectFlags(cmd.LocalFlags()),
		GlobalFlags: collectInheritedFlags(cmd),
		Examples:    trimExampleLines(cmd.Example),
	}

	if cmd.HasParent() && cmd.Parent().Name() != "" {
		data.ParentCmd = &ParentInfo{
			Name:        fullCommandName(cmd.Parent()),
			Description: cmd.Parent().Short,
			Path:        commandPath(cmd.Parent(), toolName),
		}
	}

	for _, child := range cmd.Commands() {
		if shouldSkip(child) {
			continue
		}
		data.Children = append(data.Children, ChildInfo{
			Name:        fullCommandName(child),
			Description: child.Short,
			Path:        commandPath(child, toolName),
		})
	}

	return data
}

func fullCommandName(cmd *cobra.Command) string {
	var parts []string
	for c := cmd; c != nil; c = c.Parent() {
		parts = append([]string{c.Name()}, parts...)
	}
	return strings.Join(parts, " ")
}

func parentCommandName(cmd *cobra.Command) string {
	if cmd.HasParent() {
		return fullCommandName(cmd.Parent())
	}
	return ""
}

func commandPath(cmd *cobra.Command, _ string) string {
	var parts []string
	for c := cmd; c != nil; c = c.Parent() {
		parts = append([]string{c.Name()}, parts...)
	}
	return "/cli/" + strings.Join(parts, "/") + "/"
}

func collectFlags(flags *pflag.FlagSet) []FlagInfo {
	var result []FlagInfo
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		name := formatFlagName(f)
		result = append(result, FlagInfo{
			Name:        name,
			Default:     formatDefault(f),
			Description: f.Usage,
		})
	})
	return result
}

func collectInheritedFlags(cmd *cobra.Command) []FlagInfo {
	return collectFlags(cmd.InheritedFlags())
}

func formatFlagName(f *pflag.Flag) string {
	if f.Shorthand != "" {
		return fmt.Sprintf("--%s, -%s", f.Name, f.Shorthand)
	}
	return "--" + f.Name
}

func formatDefault(f *pflag.Flag) string {
	if f.DefValue == "" {
		return `""`
	}
	return f.DefValue
}

func trimExampleLines(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}
