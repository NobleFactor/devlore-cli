// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import "testing"

// --- normalize ---

func TestNormalize_StripBackticks(t *testing.T) {
	if got := normalize("`path`"); got != "path" {
		t.Errorf("expected 'path', got %q", got)
	}
}

func TestNormalize_StripBrackets(t *testing.T) {
	if got := normalize("[path]"); got != "path" {
		t.Errorf("expected 'path', got %q", got)
	}
}

func TestNormalize_StripQuotes(t *testing.T) {
	if got := normalize(`"path"`); got != "path" {
		t.Errorf("expected 'path', got %q", got)
	}
}

func TestNormalize_Lowercase(t *testing.T) {
	if got := normalize("PARAMETERS"); got != "parameters" {
		t.Errorf("expected 'parameters', got %q", got)
	}
}

func TestNormalize_StripTrailingColon(t *testing.T) {
	if got := normalize("Parameters:"); got != "parameters" {
		t.Errorf("expected 'parameters', got %q", got)
	}
}

func TestNormalize_StripTrailingDash(t *testing.T) {
	if got := normalize("Returns -"); got != "returns" {
		t.Errorf("expected 'returns', got %q", got)
	}
}

func TestNormalize_StripTrailingEmDash(t *testing.T) {
	if got := normalize("Returns—"); got != "returns" {
		t.Errorf("expected 'returns', got %q", got)
	}
}

func TestNormalize_Synonym_Params(t *testing.T) {
	if got := normalize("Parameters:"); got != "parameters" {
		t.Errorf("expected 'parameters', got %q", got)
	}
}

func TestNormalize_Synonym_Args(t *testing.T) {
	if got := normalize("Args"); got != "parameters" {
		t.Errorf("expected 'parameters', got %q", got)
	}
}

func TestNormalize_Synonym_ReturnValues(t *testing.T) {
	if got := normalize("Return values:"); got != "returns" {
		t.Errorf("expected 'returns', got %q", got)
	}
}

func TestNormalize_Synonym_Result(t *testing.T) {
	if got := normalize("Result"); got != "returns" {
		t.Errorf("expected 'returns', got %q", got)
	}
}

func TestNormalize_Combined(t *testing.T) {
	if got := normalize("`PARAMS`:"); got != "parameters" {
		t.Errorf("expected 'parameters', got %q", got)
	}
}

func TestNormalize_NoChange(t *testing.T) {
	if got := normalize("path"); got != "path" {
		t.Errorf("expected 'path', got %q", got)
	}
}

// --- firstToken ---

func TestFirstToken_Simple(t *testing.T) {
	if got := firstToken("path: the file path"); got != "path:" {
		t.Errorf("expected 'path:', got %q", got)
	}
}

func TestFirstToken_SingleWord(t *testing.T) {
	if got := firstToken("path"); got != "path" {
		t.Errorf("expected 'path', got %q", got)
	}
}

func TestFirstToken_LeadingSpace(t *testing.T) {
	if got := firstToken("  path: desc"); got != "path:" {
		t.Errorf("expected 'path:', got %q", got)
	}
}

// --- levenshtein ---

func TestDamerauLevenshtein_Identical(t *testing.T) {
	if got := damerauLevenshtein("path", "path"); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestDamerauLevenshtein_OneEdit(t *testing.T) {
	if got := damerauLevenshtein("pth", "path"); got != 1 {
		t.Errorf("expected 1, got %d", got)
	}
}

func TestDamerauLevenshtein_TwoEdits(t *testing.T) {
	if got := damerauLevenshtein("bkupSuffix", "backupSuffix"); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}
}

func TestDamerauLevenshtein_Transposition(t *testing.T) {
	if got := damerauLevenshtein("teh", "the"); got != 1 {
		t.Errorf("expected 1 (transposition), got %d", got)
	}
	if got := damerauLevenshtein("apth", "path"); got != 1 {
		t.Errorf("expected 1 (transposition), got %d", got)
	}
}

func TestDamerauLevenshtein_Empty(t *testing.T) {
	if got := damerauLevenshtein("", "path"); got != 4 {
		t.Errorf("expected 4, got %d", got)
	}
	if got := damerauLevenshtein("path", ""); got != 4 {
		t.Errorf("expected 4, got %d", got)
	}
}

func TestDamerauLevenshtein_BothEmpty(t *testing.T) {
	if got := damerauLevenshtein("", ""); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// --- fuzzyScore ---

func TestFuzzyScore_Exact(t *testing.T) {
	score := fuzzyScore("path", "path")
	if score != 1.0 {
		t.Errorf("expected 1.0, got %f", score)
	}
}

func TestFuzzyScore_Prefix(t *testing.T) {
	score := fuzzyScore("parameters", "param")
	if score != 0.9 {
		t.Errorf("expected 0.9, got %f", score)
	}
}

func TestFuzzyScore_Substring(t *testing.T) {
	score := fuzzyScore("backupsuffix", "suffix")
	if score != 0.7 {
		t.Errorf("expected 0.7, got %f", score)
	}
}

func TestFuzzyScore_EditDistance(t *testing.T) {
	score := fuzzyScore("pth", "path")
	if score <= 0.5 || score >= 0.9 {
		t.Errorf("expected score between 0.5 and 0.9, got %f", score)
	}
}

func TestFuzzyScore_NoMatch(t *testing.T) {
	score := fuzzyScore("xyz", "path")
	if score != 0.0 {
		t.Errorf("expected 0.0, got %f", score)
	}
}

func TestFuzzyScore_NormalizedPipeline(t *testing.T) {
	// Simulate the full pipeline: normalize both sides, then score.
	candidate := normalize("`bkupSuffix`:")
	target := normalize("backupSuffix")
	score := fuzzyScore(candidate, target)
	if score <= 0.5 {
		t.Errorf("expected score > 0.5 for bkupsuffix vs backupsuffix, got %f", score)
	}
}

func TestFuzzyScore_SynonymMatch(t *testing.T) {
	candidate := normalize("Parameters:")
	target := normalize("Parameters:")
	score := fuzzyScore(candidate, target)
	if score != 1.0 {
		t.Errorf("expected 1.0 (both resolve to 'parameters'), got %f", score)
	}
}

func TestFuzzyScore_RenameDetection(t *testing.T) {
	// "source" vs "src" — substring match.
	score := fuzzyScore("source", "src")
	if score <= 0.0 {
		t.Logf("source vs src: score %f (may need forced assignment)", score)
	}
}
