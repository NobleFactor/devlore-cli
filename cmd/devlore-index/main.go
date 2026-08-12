// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Command gen-index generates index.yaml manifests for knowledge domains
// in the devlore-registry.
//
// Usage:
//
//	go run ./cmd/gen-index --registry=/path/to/devlore-registry
//
// This tool scans knowledge/{domain}/ directories and generates an index.yaml
// for each domain listing all assets by type with metadata for discovery.
//
// The tool preserves existing metadata (purpose, source_system, description)
// when updating indexes. NewExecuting files are added with empty metadata fields that
// should be filled in manually.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/document"
)

// KnowledgeIndex represents the index.yaml manifest for a knowledge domain.
type KnowledgeIndex struct {
	Domain     string           `yaml:"domain"`
	Prompts    []PromptEntry    `yaml:"prompts,omitempty"`
	Schemas    []SchemaEntry    `yaml:"schemas,omitempty"`
	Examples   []ExampleEntry   `yaml:"examples,omitempty"`
	Transforms []TransformEntry `yaml:"transforms,omitempty"`
	Signatures []SignatureEntry `yaml:"signatures,omitempty"`
	Slots      []SlotEntry      `yaml:"slots,omitempty"`
}

// PromptEntry describes a prompt asset with discovery metadata.
type PromptEntry struct {
	Name        string `yaml:"name"`
	Purpose     string `yaml:"purpose,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// SchemaEntry describes a JSON schema asset with discovery metadata.
type SchemaEntry struct {
	Name        string `yaml:"name"`
	Purpose     string `yaml:"purpose,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// ExampleEntry describes an examples asset with discovery metadata.
type ExampleEntry struct {
	Name        string `yaml:"name"`
	Purpose     string `yaml:"purpose,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// TransformEntry describes a transform asset with discovery metadata.
type TransformEntry struct {
	Name         string `yaml:"name"`
	SourceSystem string `yaml:"source_system,omitempty"`
	Description  string `yaml:"description,omitempty"`
}

// SignatureEntry describes a signature asset with discovery metadata.
type SignatureEntry struct {
	Name        string `yaml:"name"`
	System      string `yaml:"system,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// SlotEntry describes a slots asset with discovery metadata.
type SlotEntry struct {
	Name        string `yaml:"name"`
	Purpose     string `yaml:"purpose,omitempty"`
	Description string `yaml:"description,omitempty"`
}

var assetTypes = []string{
	"prompts",
	"schemas",
	"examples",
	"transforms",
	"signatures",
	"slots",
}

func main() {
	registryPath := flag.String("registry", "", "Path to devlore-registry root")
	dryRun := flag.Bool("dry-run", false, "Print what would be written without writing")
	flag.Parse()

	resolved, err := resolveRegistryPath(*registryPath)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error: --registry path required (or set DEVLORE_REGISTRY)")
		os.Exit(1)
	}

	knowledgeDir := filepath.Join(resolved, "knowledge")
	if _, err := os.Stat(knowledgeDir); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: knowledge directory not found: %s\n", knowledgeDir)
		os.Exit(1)
	}

	entries, err := os.ReadDir(knowledgeDir)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error reading knowledge directory: %v\n", err)
		os.Exit(1)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		emitDomainIndex(entry.Name(), filepath.Join(knowledgeDir, entry.Name()), *dryRun)
	}
}

// resolveRegistryPath resolves the devlore-registry root.
//
// An explicit path wins verbatim. Otherwise the conventional sibling checkouts and the
// DEVLORE_REGISTRY environment variable are probed, in that order, for a directory containing
// knowledge/.
//
// Parameters:
//   - `explicit`: the --registry flag value; empty when unset.
//
// Returns:
//   - `string`: the resolved registry root.
//   - `error`: non-nil when no candidate holds a knowledge directory.
func resolveRegistryPath(explicit string) (string, error) {

	if explicit != "" {
		return explicit, nil
	}

	candidates := []string{
		"../devlore-registry",
		"../../devlore-registry",
		os.Getenv("DEVLORE_REGISTRY"),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(filepath.Join(candidate, "knowledge")); err == nil && info.IsDir() { //nolint:gosec // G703: path from local filesystem
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no registry candidate holds a knowledge directory")
}

// emitDomainIndex builds one domain's index and either prints it (dry run) or writes it.
//
// A build failure warns and leaves the domain's existing index untouched; a write failure
// reports and returns, so one bad domain does not abort the sweep.
//
// Parameters:
//   - `domain`: the domain name.
//   - `domainPath`: the domain's directory.
//   - `dryRun`: print instead of writing.
func emitDomainIndex(domain, domainPath string, dryRun bool) {

	index, err := buildIndex(domain, domainPath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: error building index for %s: %v\n", domain, err)
		return
	}

	if dryRun {
		fmt.Printf("=== %s/index.yaml ===\n", domain)
		data, err := yaml.Marshal(index)
		if err != nil {
			log.Fatalf("marshal index for %s: %v", domain, err)
		}
		fmt.Println(string(data))
		return
	}

	if err := writeIndex(domainPath, index); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error writing index for %s: %v\n", domain, err)
		return
	}
	fmt.Printf("wrote %s/index.yaml\n", domain)
}

func buildIndex(domain, domainPath string) (*KnowledgeIndex, error) { //nolint:unparam // error return reserved for future use
	// Load existing index to preserve metadata
	existing := loadExistingIndex(domainPath)

	index := &KnowledgeIndex{
		Domain: domain,
	}

	for _, assetType := range assetTypes {
		files, err := listFiles(filepath.Join(domainPath, assetType))
		if err != nil {
			continue
		}

		switch assetType {
		case "prompts":
			index.Prompts = mergeEntries(files, existing.Prompts, func(n string) PromptEntry { return PromptEntry{Name: n} })
		case "schemas":
			index.Schemas = mergeEntries(files, existing.Schemas, func(n string) SchemaEntry { return SchemaEntry{Name: n} })
		case "examples":
			index.Examples = mergeEntries(files, existing.Examples, func(n string) ExampleEntry { return ExampleEntry{Name: n} })
		case "transforms":
			index.Transforms = mergeEntries(files, existing.Transforms, func(n string) TransformEntry { return TransformEntry{Name: n} })
		case "signatures":
			index.Signatures = mergeEntries(files, existing.Signatures, func(n string) SignatureEntry { return SignatureEntry{Name: n} })
		case "slots":
			index.Slots = mergeEntries(files, existing.Slots, func(n string) SlotEntry { return SlotEntry{Name: n} })
		}
	}

	return index, nil
}

// loadExistingIndex loads an existing index.yaml from the domain directory. Returns an empty index if not found.
//
// Parameters:
//   - domainPath: directory containing the domain's index.yaml
//
// Returns:
//   - *KnowledgeIndex: parsed index, or empty index if missing
func loadExistingIndex(domainPath string) *KnowledgeIndex {

	indexPath := filepath.Join(domainPath, "index.yaml")

	index, err := document.ReadFile[KnowledgeIndex](indexPath)
	if err != nil {
		return &KnowledgeIndex{}
	}

	return index
}

func listFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "index.yaml" {
			continue
		}
		files = append(files, name)
	}

	sort.Strings(files)
	return files, nil
}

// named is the constraint for entry types with a ReceiverName field.
type named interface {
	PromptEntry | SchemaEntry | ExampleEntry | TransformEntry | SignatureEntry | SlotEntry
	getName() string
}

func (e PromptEntry) getName() string    { return e.Name }
func (e SchemaEntry) getName() string    { return e.Name }
func (e ExampleEntry) getName() string   { return e.Name }
func (e TransformEntry) getName() string { return e.Name }
func (e SignatureEntry) getName() string { return e.Name }
func (e SlotEntry) getName() string      { return e.Name }

// mergeEntries preserves existing metadata for files that still exist,
// adds new files via newFn, and removes entries for deleted files.
func mergeEntries[T named](files []string, existing []T, newFn func(string) T) []T {
	existingMap := make(map[string]T)
	for _, e := range existing {
		existingMap[e.getName()] = e
	}

	var result []T
	for _, f := range files {
		if e, ok := existingMap[f]; ok {
			result = append(result, e)
		} else {
			result = append(result, newFn(f))
		}
	}
	return result
}

// writeIndex writes the knowledge index to index.yaml in the domain directory.
//
// Parameters:
//   - domainPath: directory to write index.yaml into
//   - index: knowledge index to serialize
//
// Returns:
//   - error: marshal or write error
func writeIndex(domainPath string, index *KnowledgeIndex) error {

	header := "# Auto-generated file list by: go run ./cmd/gen-index\n" +
		"# Metadata (purpose, source_system, description) is preserved and should be edited manually.\n"

	return document.Write(filepath.Join(domainPath, "index.yaml"), index, document.WithHeader(header))
}
