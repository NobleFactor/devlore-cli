// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starstats

import (
	"path/filepath"
	"sort"
	"testing"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../starcode/testdata")
	if err != nil {
		t.Fatalf("abs testdata: %v", err)
	}
	return dir
}

func captureFiles(t *testing.T, root, pattern string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, pattern))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	sort.Strings(matches)
	return matches
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLOC  int
		wantSLOC int
		wantComm int
		wantBl   int
	}{
		{
			name:     "empty",
			input:    "",
			wantLOC:  0,
			wantSLOC: 0,
			wantComm: 0,
			wantBl:   0,
		},
		{
			name:     "single code line",
			input:    "x = 1\n",
			wantLOC:  1,
			wantSLOC: 1,
			wantComm: 0,
			wantBl:   0,
		},
		{
			name:     "comment and code",
			input:    "# comment\nx = 1\n",
			wantLOC:  2,
			wantSLOC: 1,
			wantComm: 1,
			wantBl:   0,
		},
		{
			name:     "blank lines",
			input:    "x = 1\n\ny = 2\n",
			wantLOC:  3,
			wantSLOC: 2,
			wantComm: 0,
			wantBl:   1,
		},
		{
			name:     "mixed",
			input:    "# header\n\nx = 1\n# inline\ny = 2\n",
			wantLOC:  5,
			wantSLOC: 2,
			wantComm: 2,
			wantBl:   1,
		},
		{
			name:     "no trailing newline",
			input:    "x = 1",
			wantLOC:  1,
			wantSLOC: 1,
			wantComm: 0,
			wantBl:   0,
		},
		{
			name:     "only comments",
			input:    "# a\n# b\n# c\n",
			wantLOC:  3,
			wantSLOC: 0,
			wantComm: 3,
			wantBl:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, sloc, comm, bl := countLines([]byte(tt.input))
			if loc != tt.wantLOC || sloc != tt.wantSLOC || comm != tt.wantComm || bl != tt.wantBl {
				t.Errorf("countLines = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					loc, sloc, comm, bl,
					tt.wantLOC, tt.wantSLOC, tt.wantComm, tt.wantBl)
			}
		})
	}
}

func TestStatsSimple(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "*.star")

	stats, err := (&Provider{Root: root}).ComputeStats(files, true, true)
	if err != nil {
		t.Fatalf("ComputeStats: %v", err)
	}

	if stats.Totals.FileCount != len(files) {
		t.Errorf("FileCount = %d, want %d", stats.Totals.FileCount, len(files))
	}

	if stats.Totals.TotalLOC == 0 {
		t.Error("expected non-zero TotalLOC")
	}

	if stats.Totals.TotalBytes == 0 {
		t.Error("expected non-zero TotalBytes")
	}
}

func TestStatsEmptyFile(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "empty.star")

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	stats, err := (&Provider{Root: root}).ComputeStats(files, true, true)
	if err != nil {
		t.Fatalf("ComputeStats: %v", err)
	}

	if stats.Files[0].LOC != 0 {
		t.Errorf("empty file LOC = %d, want 0", stats.Files[0].LOC)
	}
	if stats.Files[0].Bytes != 0 {
		t.Errorf("empty file Bytes = %d, want 0", stats.Files[0].Bytes)
	}
}

func TestStatsBytesOnly(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "simple.star")

	stats, err := (&Provider{Root: root}).ComputeStats(files, true, false)
	if err != nil {
		t.Fatalf("ComputeStats: %v", err)
	}

	if stats.Files[0].Bytes == 0 {
		t.Error("expected non-zero Bytes")
	}
	if stats.Files[0].LOC != 0 {
		t.Error("LOC should be 0 when withLOC=false")
	}
}

func TestStatsLOCOnly(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "simple.star")

	stats, err := (&Provider{Root: root}).ComputeStats(files, false, true)
	if err != nil {
		t.Fatalf("ComputeStats: %v", err)
	}

	if stats.Files[0].LOC == 0 {
		t.Error("expected non-zero LOC")
	}
	if stats.Files[0].Bytes != 0 {
		t.Error("Bytes should be 0 when withBytes=false")
	}
}
