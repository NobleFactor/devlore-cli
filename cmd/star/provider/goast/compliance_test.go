// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// TestCheckComplianceExemptsTestEntryPoints covers the one exception to "everything is linted".
//
// A test entry point's signature is fixed by the framework, so a Parameters section restates the same thing
// every time -- 756 times in this repository when the exception was approved on 2026-09-29. The exemption is
// narrow, and the cases below are what keeps it narrow: a helper in a test file is still checked, a
// Test-prefixed function in ordinary source is still checked, and the doc comment is required throughout.
//
// The file is written to disk and loaded through [Provider.LoadSourceFile], because that is the only path
// that sets `filename` -- `parseSourceFile` leaves it empty, so a test built on it would pass while the real
// linter did something else.
func TestCheckComplianceExemptsTestEntryPoints(t *testing.T) {

	for _, testCase := range []struct {
		name     string
		file     string
		source   string
		expected []string
	}{
		{
			name: "a test entry point needs no Parameters section",
			file: "thing_test.go",
			source: `package thing

import "testing"

// TestThing asserts the thing.
func TestThing(t *testing.T) {}
`,
			expected: nil,
		},
		{
			name: "benchmark, fuzz and example are entry points too",
			file: "thing_test.go",
			source: `package thing

import "testing"

// BenchmarkThing measures the thing.
func BenchmarkThing(b *testing.B) {}

// FuzzThing fuzzes the thing.
func FuzzThing(f *testing.F) {}

// ExampleThing shows the thing.
func ExampleThing() {}
`,
			expected: nil,
		},
		{
			name: "a test entry point still needs a doc comment",
			file: "thing_test.go",
			source: `package thing

import "testing"

func TestThing(t *testing.T) {}
`,
			expected: []string{"TestThing: missing doc comment"},
		},
		{
			name: "a helper in a test file is NOT exempt",
			file: "thing_test.go",
			source: `package thing

import "testing"

// lineOf returns one line.
func lineOf(t *testing.T, path string, number int) string { return "" }
`,
			expected: []string{
				"lineOf: missing Parameters section",
				"lineOf: missing Returns section",
			},
		},
		{
			name: "a Test-prefixed function in ordinary source is NOT exempt",
			file: "thing.go",
			source: `package thing

import "testing"

// TestHarness runs a harness.
func TestHarness(t *testing.T) {}
`,
			expected: []string{"TestHarness: missing Parameters section"},
		},
		{
			name: "an entry point that returns something still needs Returns",
			file: "thing_test.go",
			source: `package thing

import "testing"

// TestOdd is not a real entry point shape, but the Returns rule is untouched by the exemption.
func TestOdd(t *testing.T) error { return nil }
`,
			expected: []string{"TestOdd: missing Returns section"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			path := filepath.Join(t.TempDir(), testCase.file)
			if err := os.WriteFile(path, []byte(testCase.source), 0o644); err != nil {
				t.Fatalf("writing %s: %v", path, err)
			}

			// A real environment, not nil: NewProvider registers parameters on it and panics otherwise.
			// Same wiring as config_schema_test.go, which is the same wiring star main uses.
			provider := NewProvider(&op.RuntimeEnvironment{
				Application: &application.Application{Name: "test"},
			})

			sourceFile, err := provider.LoadSourceFile(path)
			if err != nil {
				t.Fatalf("loading %s: %v", path, err)
			}

			var got []string
			for _, violation := range sourceFile.CheckCompliance() {
				got = append(got, violation.Message)
			}

			if strings.Join(got, "; ") != strings.Join(testCase.expected, "; ") {
				t.Errorf("violations %v, want %v", got, testCase.expected)
			}
		})
	}
}
