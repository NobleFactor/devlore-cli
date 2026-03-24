// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import "strings"

// fuzzyScore computes a match score between a candidate string and a target.
// Both should be pre-normalized. Returns a score between 0.0 (no match) and 1.0 (exact).
func fuzzyScore(candidate, target string) float64 {
	if candidate == target {
		return 1.0
	}
	if strings.HasPrefix(candidate, target) {
		return 0.9
	}
	if strings.Contains(candidate, target) {
		return 0.7
	}
	maxLen := len(candidate)
	if len(target) > maxLen {
		maxLen = len(target)
	}
	if maxLen == 0 {
		return 0.0
	}
	dist := damerauLevenshtein(candidate, target)
	score := 1.0 - float64(dist)/float64(maxLen)
	if score > 0.5 {
		return score
	}
	return 0.0
}

// normalize prepares a string for fuzzy matching: strips demarcation, lowercases,
// strips trailing punctuation, and resolves synonyms.
func normalize(s string) string {
	s = stripDemarcation(s)
	s = strings.ToLower(s)
	s = stripTrailingPunctuation(s)
	s = strings.TrimSpace(s)
	s = resolveSynonym(s)
	return s
}

// firstToken extracts the first whitespace-delimited token from a string.
func firstToken(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

// stripDemarcation removes backticks, square brackets, and quotes from a string.
func stripDemarcation(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '`', '[', ']', '"', '\'':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// stripTrailingPunctuation removes trailing colons, dashes, em-dashes, and periods.
func stripTrailingPunctuation(s string) string {
	return strings.TrimRight(s, ":.-—–")
}

// synonyms maps alternative terms to canonical forms. Hardcoded for now; configurable later.
var synonyms = map[string]string{
	"params":        "parameters",
	"arguments":     "parameters",
	"args":          "parameters",
	"inputs":        "parameters",
	"return values": "returns",
	"return":        "returns",
	"result":        "returns",
	"results":       "returns",
	"output":        "returns",
	"outputs":       "returns",
	"deprecation":   "deprecated",
}

// resolveSynonym maps a normalized string to its canonical form if a synonym exists.
func resolveSynonym(s string) string {
	if canonical, ok := synonyms[s]; ok {
		return canonical
	}
	return s
}

// damerauLevenshtein computes the Damerau-Levenshtein distance between two strings.
// Supports insertions, deletions, substitutions, and transpositions of adjacent characters.
func damerauLevenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Full matrix needed for transposition lookback.
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
	}
	for i := 0; i <= la; i++ {
		d[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		d[0][j] = j
	}

	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min3(
				d[i-1][j]+1,      // deletion
				d[i][j-1]+1,      // insertion
				d[i-1][j-1]+cost, // substitution
			)
			// Transposition.
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				trans := d[i-2][j-2] + cost
				if trans < d[i][j] {
					d[i][j] = trans
				}
			}
		}
	}
	return d[la][lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
