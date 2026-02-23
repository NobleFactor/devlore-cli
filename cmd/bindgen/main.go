// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// bindgen generates Starlark bindings from CLI metadata.
//
// Usage:
//
//	bindgen parse <command> [subcommands...]    Parse --help and output YAML
//	bindgen generate <definition.yaml>          Generate Go and Starlark code
//	bindgen scaffold <command>                  Parse --help and generate code directly
//
// Examples:
//
//	# Parse docker run --help and save definition
//	bindgen parse docker run build push > docker.yaml
//
//	# Edit docker.yaml to refine types, add descriptions, remove unwanted flags
//
//	# Generate bindings from refined definition
//	bindgen generate docker.yaml
//
//	# Quick scaffold (parse + generate, no manual refinement)
//	bindgen scaffold gh repo issue pr
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/internal/bindgen"
	"github.com/NobleFactor/devlore-cli/internal/bindgen/cobra"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "parse":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: bindgen parse <command> [subcommands...]")
			os.Exit(1)
		}
		cmdParse(os.Args[2], os.Args[3:])

	case "extract-cobra":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: bindgen extract-cobra <source-dir> [--verbose]")
			os.Exit(1)
		}
		verbose := len(os.Args) > 3 && os.Args[3] == "--verbose"
		cmdExtractCobra(os.Args[2], verbose)

	case "generate":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: bindgen generate <definition.yaml>")
			os.Exit(1)
		}
		cmdGenerate(os.Args[2])

	case "scaffold":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: bindgen scaffold <command> [subcommands...]")
			os.Exit(1)
		}
		cmdScaffold(os.Args[2], os.Args[3:])

	case "help", "-h", "--help":
		usage()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`bindgen - Generate Starlark bindings from CLI metadata

Usage:
  bindgen parse <command> [subcommands...]    Parse --help and output YAML definition
  bindgen extract-cobra <source-dir>          Extract from Go/Cobra source code
  bindgen generate <definition.yaml>          Generate Go and Starlark code from YAML
  bindgen scaffold <command> [subcommands...] Parse and generate directly (no YAML step)

Workflow (--help based):
  1. Parse: Extract CLI metadata from --help output
     $ bindgen parse docker run build push > docker.yaml

  2. Refine: Edit the YAML to improve types, descriptions, remove unwanted flags

  3. Generate: Produce Go bindings and Starlark stubs
     $ bindgen generate docker.yaml

Workflow (source introspection):
  1. Clone: Get the CLI source at a specific version
     $ git clone --depth 1 --branch v27.0.0 https://github.com/docker/cli

  2. Extract: Parse Go AST for cobra.Command definitions
     $ bindgen extract-cobra ./docker-cli/cli/command/ > docker.yaml

  3. Refine & Generate (same as above)

Output:
  parse         -> YAML to stdout
  extract-cobra -> YAML to stdout
  generate      -> <name>.go and <name>.star in current directory
  scaffold      -> <name>.go and <name>.star in current directory`)
}

// cmdParse extracts CLI metadata and outputs YAML.
func cmdParse(command string, subcommands []string) {
	parser := bindgen.NewHelpParser(command)

	def := &bindgen.BindingDef{
		Name:     command,
		Commands: make(map[string]*bindgen.Command),
	}

	// If no subcommands specified, try to discover them
	if len(subcommands) == 0 {
		discovered, err := parser.ListSubcommands()
		if err == nil && len(discovered) > 0 {
			fmt.Fprintf(os.Stderr, "Discovered subcommands: %v\n", discovered)
			subcommands = discovered
		}
	}

	// Parse each subcommand
	for _, sub := range subcommands {
		cmd, err := parser.Parse(sub)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse %s %s: %v\n", command, sub, err)
			continue
		}
		def.Commands[sub] = cmd
		fmt.Fprintf(os.Stderr, "Parsed %s %s: %d flags\n", command, sub, len(cmd.Flags))
	}

	// Output YAML
	if err := bindgen.SaveYAML(def, "/dev/stdout"); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing YAML: %v\n", err)
		os.Exit(1)
	}
}

// cmdGenerate produces code from a YAML definition.
func cmdGenerate(yamlPath string) {
	def, err := bindgen.LoadYAML(yamlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", yamlPath, err)
		os.Exit(1)
	}

	// Generate Go code
	goGen := bindgen.NewGoGenerator()
	goCode, err := goGen.Generate(def)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating Go code: %v\n", err)
		os.Exit(1)
	}

	goPath := def.Name + "_gen.go"
	if err := os.WriteFile(goPath, []byte(goCode), 0644); err != nil { //nolint:gosec // G705: template output is for code generation
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", goPath, err)
		os.Exit(1)
	}
	fmt.Printf("Generated %s\n", goPath)

	// Generate Starlark stubs
	stubGen := bindgen.NewStubGenerator()
	stubCode, err := stubGen.Generate(def)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating Starlark stubs: %v\n", err)
		os.Exit(1)
	}

	stubPath := def.Name + "_gen.star"
	if err := os.WriteFile(stubPath, []byte(stubCode), 0644); err != nil { //nolint:gosec // G705: template output is for code generation
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", stubPath, err)
		os.Exit(1)
	}
	fmt.Printf("Generated %s\n", stubPath)
}

// cmdScaffold parses and generates in one step.
func cmdScaffold(command string, subcommands []string) {
	parser := bindgen.NewHelpParser(command)

	def := &bindgen.BindingDef{
		Name:     command,
		Commands: make(map[string]*bindgen.Command),
	}

	// If no subcommands specified, try to discover them
	if len(subcommands) == 0 {
		discovered, err := parser.ListSubcommands()
		if err == nil && len(discovered) > 0 {
			fmt.Fprintf(os.Stderr, "Discovered subcommands: %v\n", discovered)
			subcommands = discovered
		}
	}

	// Parse each subcommand
	for _, sub := range subcommands {
		cmd, err := parser.Parse(sub)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse %s %s: %v\n", command, sub, err)
			continue
		}
		def.Commands[sub] = cmd
		fmt.Fprintf(os.Stderr, "Parsed %s %s: %d flags\n", command, sub, len(cmd.Flags))
	}

	if len(def.Commands) == 0 {
		fmt.Fprintln(os.Stderr, "No commands parsed successfully")
		os.Exit(1)
	}

	// Generate Go code
	goGen := bindgen.NewGoGenerator()
	goCode, err := goGen.Generate(def)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating Go code: %v\n", err)
		os.Exit(1)
	}

	goPath := filepath.Join(".", def.Name+"_gen.go")
	if err := os.WriteFile(goPath, []byte(goCode), 0644); err != nil { //nolint:gosec // G705: template output is for code generation
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", goPath, err)
		os.Exit(1)
	}
	fmt.Printf("Generated %s\n", goPath)

	// Generate Starlark stubs
	stubGen := bindgen.NewStubGenerator()
	stubCode, err := stubGen.Generate(def)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating Starlark stubs: %v\n", err)
		os.Exit(1)
	}

	stubPath := filepath.Join(".", def.Name+"_gen.star")
	if err := os.WriteFile(stubPath, []byte(stubCode), 0644); err != nil { //nolint:gosec // G705: template output is for code generation
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", stubPath, err)
		os.Exit(1)
	}
	fmt.Printf("Generated %s\n", stubPath)
}

// cmdExtractCobra extracts commands from Go/Cobra source code.
func cmdExtractCobra(sourceDir string, verbose bool) {
	extractor := cobra.NewExtractor(verbose)

	def, err := extractor.ExtractDir(sourceDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting from %s: %v\n", sourceDir, err)
		os.Exit(1)
	}

	// Set a meaningful name based on the directory
	def.Name = filepath.Base(filepath.Dir(sourceDir))
	if def.Name == "." || def.Name == "" {
		def.Name = "extracted"
	}

	commands, flags := extractor.Stats()
	fmt.Fprintf(os.Stderr, "Extracted %d commands, %d flags from %s\n", commands, flags, sourceDir)

	// Output YAML to stdout
	if err := bindgen.SaveYAML(def, "/dev/stdout"); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing YAML: %v\n", err)
		os.Exit(1)
	}
}
