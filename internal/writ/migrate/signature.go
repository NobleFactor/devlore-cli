// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/registry"
)

// Signature defines detection markers for a source system.
type Signature struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Markers     []Marker `yaml:"markers"`
}

// Marker defines a single detection rule.
type Marker struct {
	Type        string  `yaml:"type"`        // file, directory, file_pattern, directory_pattern, file_contains, file_magic
	Path        string  `yaml:"path"`        // for file, directory types
	Pattern     string  `yaml:"pattern"`     // for *_pattern types, or content pattern for file_contains
	Bytes       []byte  `yaml:"bytes"`       // for file_magic type
	Confidence  float64 `yaml:"confidence"`  // 0.0-1.0
	Description string  `yaml:"description"` // human-readable explanation
}

// DetectionResult holds the outcome of signature-based detection.
type DetectionResult struct {
	System     SourceSystem
	Confidence float64
	Matches    []MarkerMatch
}

// MarkerMatch records a single marker that matched.
type MarkerMatch struct {
	Marker     Marker
	MatchedAt  string // path or pattern that matched
	Confidence float64
}

// LoadSignatures loads all detection signatures from the registry.
// It reads the index.yaml manifest to discover available signature files,
// then loads and parses each one.
func LoadSignatures(client *registry.Client) ([]Signature, error) {
	knowledge := client.Knowledge("migration")

	// Load the index to discover available signatures
	index, err := knowledge.Index()
	if err != nil {
		return nil, fmt.Errorf("loading migration index: %w", err)
	}

	var signatures []Signature
	for _, name := range index.Signatures {
		data, err := knowledge.Signature(name)
		if err != nil {
			continue // skip missing signatures
		}

		var sig Signature
		if err := yaml.Unmarshal(data, &sig); err != nil {
			continue // skip malformed signatures
		}

		signatures = append(signatures, sig)
	}

	return signatures, nil
}

// DetectWithSignatures uses registry signatures to identify the source system.
// Returns all matches sorted by confidence (highest first).
func DetectWithSignatures(root string, signatures []Signature) []DetectionResult {
	var results []DetectionResult

	for _, sig := range signatures {
		matches := evaluateSignature(root, sig)
		if len(matches) > 0 {
			// Calculate aggregate confidence (highest match wins)
			maxConf := 0.0
			for _, m := range matches {
				if m.Confidence > maxConf {
					maxConf = m.Confidence
				}
			}

			results = append(results, DetectionResult{
				System:     SourceSystem(sig.Name),
				Confidence: maxConf,
				Matches:    matches,
			})
		}
	}

	// Sort by confidence descending
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Confidence > results[i].Confidence {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// evaluateSignature checks all markers against the root directory.
func evaluateSignature(root string, sig Signature) []MarkerMatch {
	var matches []MarkerMatch

	for _, marker := range sig.Markers {
		if match, ok := evaluateMarker(root, marker); ok {
			matches = append(matches, match)
		}
	}

	return matches
}

// evaluateMarker checks a single marker against the root directory.
func evaluateMarker(root string, marker Marker) (MarkerMatch, bool) {
	switch marker.Type {
	case "file":
		return evaluateFileMarker(root, marker)
	case "directory":
		return evaluateDirectoryMarker(root, marker)
	case "file_pattern":
		return evaluateFilePatternMarker(root, marker)
	case "directory_pattern":
		return evaluateDirectoryPatternMarker(root, marker)
	case "file_contains":
		return evaluateFileContainsMarker(root, marker)
	case "file_magic":
		return evaluateFileMagicMarker(root, marker)
	default:
		return MarkerMatch{}, false
	}
}

// evaluateFileMarker checks if a specific file exists.
func evaluateFileMarker(root string, marker Marker) (MarkerMatch, bool) {
	path := marker.Path

	// Handle glob patterns like "**/Hooks.toml"
	if strings.Contains(path, "*") {
		matches, _ := filepath.Glob(filepath.Join(root, path))
		if len(matches) == 0 {
			// Try recursive search for ** patterns
			if strings.HasPrefix(path, "**/") {
				filename := strings.TrimPrefix(path, "**/")
				if found, foundPath := findFileRecursive(root, filename); found {
					return MarkerMatch{
						Marker:     marker,
						MatchedAt:  foundPath,
						Confidence: marker.Confidence,
					}, true
				}
			}
			return MarkerMatch{}, false
		}
		return MarkerMatch{
			Marker:     marker,
			MatchedAt:  matches[0],
			Confidence: marker.Confidence,
		}, true
	}

	// Direct path check
	fullPath := filepath.Join(root, path)
	if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
		return MarkerMatch{
			Marker:     marker,
			MatchedAt:  fullPath,
			Confidence: marker.Confidence,
		}, true
	}

	return MarkerMatch{}, false
}

// evaluateDirectoryMarker checks if a specific directory exists.
func evaluateDirectoryMarker(root string, marker Marker) (MarkerMatch, bool) {
	fullPath := filepath.Join(root, marker.Path)
	if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
		return MarkerMatch{
			Marker:     marker,
			MatchedAt:  fullPath,
			Confidence: marker.Confidence,
		}, true
	}
	return MarkerMatch{}, false
}

// evaluateFilePatternMarker checks for files matching a pattern.
func evaluateFilePatternMarker(root string, marker Marker) (MarkerMatch, bool) {
	pattern := marker.Pattern
	entries, err := os.ReadDir(root)
	if err != nil {
		return MarkerMatch{}, false
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if matchPattern(e.Name(), pattern) {
			return MarkerMatch{
				Marker:     marker,
				MatchedAt:  e.Name(),
				Confidence: marker.Confidence,
			}, true
		}
	}

	return MarkerMatch{}, false
}

// evaluateDirectoryPatternMarker checks for directories matching a pattern.
func evaluateDirectoryPatternMarker(root string, marker Marker) (MarkerMatch, bool) {
	pattern := marker.Pattern
	entries, err := os.ReadDir(root)
	if err != nil {
		return MarkerMatch{}, false
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if matchPattern(e.Name(), pattern) {
			return MarkerMatch{
				Marker:     marker,
				MatchedAt:  e.Name(),
				Confidence: marker.Confidence,
			}, true
		}
	}

	return MarkerMatch{}, false
}

// evaluateFileContainsMarker checks if a file contains a pattern.
func evaluateFileContainsMarker(root string, marker Marker) (MarkerMatch, bool) {
	fullPath := filepath.Join(root, marker.Path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return MarkerMatch{}, false
	}

	if strings.Contains(string(data), marker.Pattern) {
		return MarkerMatch{
			Marker:     marker,
			MatchedAt:  fullPath,
			Confidence: marker.Confidence,
		}, true
	}

	return MarkerMatch{}, false
}

// evaluateFileMagicMarker checks files for magic byte signatures.
func evaluateFileMagicMarker(root string, marker Marker) (MarkerMatch, bool) {
	// Walk directory looking for files with matching magic bytes
	var found string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		buf := make([]byte, len(marker.Bytes))
		n, err := f.Read(buf)
		if err != nil || n < len(marker.Bytes) {
			return nil
		}

		match := true
		for i, b := range marker.Bytes {
			if buf[i] != b {
				match = false
				break
			}
		}

		if match {
			found = path
			return filepath.SkipAll
		}
		return nil
	})

	if found != "" {
		return MarkerMatch{
			Marker:     marker,
			MatchedAt:  found,
			Confidence: marker.Confidence,
		}, true
	}

	return MarkerMatch{}, false
}

// matchPattern matches a name against a glob-like pattern.
// Supports * wildcard and {a,b,c} alternations.
func matchPattern(name, pattern string) bool {
	// Handle {a,b,c} alternations
	if strings.Contains(pattern, "{") {
		re := patternToRegex(pattern)
		matched, _ := regexp.MatchString(re, name)
		return matched
	}

	// Simple glob matching
	matched, _ := filepath.Match(pattern, name)
	return matched
}

// patternToRegex converts a glob pattern with {a,b} to regex.
func patternToRegex(pattern string) string {
	// Escape regex special chars except * and {}
	re := regexp.QuoteMeta(pattern)

	// Convert \* back to .*
	re = strings.ReplaceAll(re, `\*`, `.*`)

	// Convert \{a,b,c\} to (a|b|c)
	re = strings.ReplaceAll(re, `\{`, `(`)
	re = strings.ReplaceAll(re, `\}`, `)`)
	re = strings.ReplaceAll(re, `,`, `|`)

	return "^" + re + "$"
}

// findFileRecursive searches for a file by name recursively.
func findFileRecursive(root, name string) (bool, string) {
	var found string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Name() == name {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found != "", found
}
