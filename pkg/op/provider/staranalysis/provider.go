// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package staranalysis provides combined analysis of Starlark source files,
// combining stats, complexity scoring, indexing, and hotspot detection.
package staranalysis

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/starcomplexity"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/starindex"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/starstats"
)

// AnalysisConfig controls what the Analyze function produces.
type AnalysisConfig struct {
	Hotspots            bool
	CyclomaticThreshold int // default 10
	CognitiveThreshold  int // default 15
	WithIndex           bool
}

// Hotspot identifies a function that exceeds complexity thresholds.
type Hotspot struct {
	File       string
	Name       string
	Line       int
	Cyclomatic int
	Cognitive  int
	LOC        int
}

// AnalysisReport combines stats, complexity, hotspots, and optionally index.
type AnalysisReport struct {
	Stats      *starstats.Stats
	Complexity *starcomplexity.ComplexityReport
	Hotspots   []Hotspot
	Index      *starindex.Index // nil unless WithIndex
}

// Provider provides combined analysis of Starlark source files,
// combining stats, complexity scoring, indexing, and hotspot detection.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
	Root string
}

func NewProvider(ctx op.Context) *Provider {
	p := &Provider{ProviderBase: op.NewProviderBase(ctx)}
	if ctx.Root != nil {
		p.Root = ctx.Root.Name()
	}
	return p
}

// Analyze performs a combined analysis of all files.
//
// +devlore:struct_param cfg=AnalysisConfig
func (p *Provider) Analyze(files []string, cfg AnalysisConfig) (*AnalysisReport, error) {
	if cfg.CyclomaticThreshold <= 0 {
		cfg.CyclomaticThreshold = 10
	}
	if cfg.CognitiveThreshold <= 0 {
		cfg.CognitiveThreshold = 15
	}

	stats, err := (&starstats.Provider{Root: p.Root}).ComputeStats(files, true, true)
	if err != nil {
		return nil, err
	}

	complexity, err := (&starcomplexity.Provider{Root: p.Root}).ComputeComplexity(files)
	if err != nil {
		return nil, err
	}

	report := &AnalysisReport{
		Stats:      stats,
		Complexity: complexity,
	}

	if cfg.Hotspots {
		report.Hotspots = findHotspots(complexity, cfg.CyclomaticThreshold, cfg.CognitiveThreshold)
	}

	if cfg.WithIndex {
		idx, err := (&starindex.Provider{Root: p.Root}).IndexFiles(files, true, true)
		if err != nil {
			return nil, err
		}
		report.Index = idx
	}

	return report, nil
}

// findHotspots scans the complexity report for functions exceeding thresholds.
func findHotspots(cr *starcomplexity.ComplexityReport, cyclomaticThreshold, cognitiveThreshold int) []Hotspot {
	var hotspots []Hotspot
	for _, fc := range cr.Files {
		for _, fn := range fc.Functions {
			if fn.Cyclomatic >= cyclomaticThreshold || fn.Cognitive >= cognitiveThreshold {
				hotspots = append(hotspots, Hotspot{
					File:       fc.Path,
					Name:       fn.Name,
					Line:       fn.Line,
					Cyclomatic: fn.Cyclomatic,
					Cognitive:  fn.Cognitive,
					LOC:        fn.LOC,
				})
			}
		}
	}
	return hotspots
}
