// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// ScriptAnalysis captures information extracted from a lifecycle script.
type ScriptAnalysis struct {
	RelPath       string            `json:"rel_path" yaml:"rel_path"`
	Name          string            `json:"name" yaml:"name"`
	Phase         string            `json:"phase" yaml:"phase"`
	PlatformGuard string            `json:"platform_guard,omitempty" yaml:"platform_guard,omitempty"`
	LineCount     int               `json:"line_count" yaml:"line_count"`
	Resolved      []DetectedInstall `json:"resolved,omitempty" yaml:"resolved,omitempty"`
	Unresolved    []DetectedInstall `json:"unresolved,omitempty" yaml:"unresolved,omitempty"`
	Observations  []string          `json:"observations,omitempty" yaml:"observations,omitempty"`
}

// packagePattern defines a regex pattern for detecting package manager usage.
type packagePattern struct {
	regex   *regexp.Regexp
	manager string
	multi   bool // true if the match can contain multiple space-separated packages
}

var (
	rePlatformGuard = regexp.MustCompile(`require_(linux|darwin|unix|windows|debian)`)
	reGitClone      = regexp.MustCompile(`git\s+clone\s+\S+`)
	reMakeInstall   = regexp.MustCompile(`make\s+install`)
	reCurlPipe      = regexp.MustCompile(`curl\s+.*\|\s*(sh|bash)`)

	packagePatterns = []packagePattern{
		{regexp.MustCompile(`brew\s+install\s+(?:--\S+\s+)*(\S+)`), "brew", false},
		{regexp.MustCompile(`(?:apt|apt-get)\s+install\s+(?:-\S+\s+)*(.+)`), "apt", true},
		{regexp.MustCompile(`cargo\s+install\s+(\S+)`), "cargo", false},
		{regexp.MustCompile(`dnf\s+install\s+(?:-\S+\s+)*(.+)`), "dnf", true},
		{regexp.MustCompile(`pacman\s+-S\s+(?:--\S+\s+)*(.+)`), "pacman", true},
		{regexp.MustCompile(`pip[3]?\s+install\s+(\S+)`), "pip", false},
		{regexp.MustCompile(`npm\s+install\s+-g\s+(\S+)`), "npm", false},
		{regexp.MustCompile(`go\s+install\s+(\S+)`), "go", false},
		{regexp.MustCompile(`winget\s+install\s+(\S+)`), "winget", false},
		{regexp.MustCompile(`choco\s+install\s+(\S+)`), "choco", false},
	}
)

// AnalyzeScripts examines lifecycle script entries and extracts information
// about what they install and how.
func AnalyzeScripts(entries []InventoryEntry, idx SignatureIndex) []ScriptAnalysis {
	var analyses []ScriptAnalysis
	for _, e := range entries {
		if e.Class != ClassLifecycleScript {
			continue
		}
		analyses = append(analyses, analyzeScript(e, idx))
	}
	return analyses
}

func analyzeScript(e InventoryEntry, idx SignatureIndex) ScriptAnalysis {
	name := strings.TrimSuffix(e.RelPath, "/")
	parts := strings.Split(name, string(os.PathSeparator))
	baseName := parts[len(parts)-1]

	analysis := ScriptAnalysis{
		RelPath: e.RelPath,
		Name:    baseName,
		Phase:   phaseFromName(baseName),
	}

	f, err := os.Open(e.AbsPath)
	if err != nil {
		analysis.Observations = append(analysis.Observations, "Could not read script: "+err.Error())
		return analysis
	}
	defer f.Close()

	result := scanScript(f)
	analysis.LineCount = result.lineCount
	analysis.PlatformGuard = result.platformGuard

	// Resolve detected installs against signature index
	for _, install := range result.installs {
		install.LorePackage = idx.Resolve(install.Manager, install.Name)
		if install.IsResolved() {
			analysis.Resolved = append(analysis.Resolved, install)
		} else {
			analysis.Unresolved = append(analysis.Unresolved, install)
		}
	}

	analysis.Observations = buildObservations(analysis, result)

	return analysis
}

func phaseFromName(name string) string {
	switch {
	case strings.HasPrefix(name, "Install-"):
		return "install"
	case strings.HasPrefix(name, "Initialize-"):
		return "initialize"
	default:
		return ""
	}
}

// scanResult accumulates findings from scanning a script.
type scanResult struct {
	installs      []DetectedInstall
	managers      map[string]bool
	platformGuard string
	lineCount     int
	hasGit        bool
	hasMake       bool
	hasCurl       bool
}

func newScanResult() *scanResult {
	return &scanResult{
		managers: make(map[string]bool),
	}
}

func (r *scanResult) packageManager() string {
	var mgrs []string
	for m := range r.managers {
		mgrs = append(mgrs, m)
	}

	switch {
	case len(mgrs) == 1:
		return mgrs[0]
	case len(mgrs) > 1:
		return strings.Join(mgrs, "/")
	case r.hasGit && r.hasMake:
		return "source"
	case r.hasCurl:
		return "curl"
	default:
		return ""
	}
}

func buildObservations(analysis ScriptAnalysis, result *scanResult) []string {
	var obs []string

	if len(analysis.Resolved) > 0 {
		var names []string
		for _, r := range analysis.Resolved {
			names = append(names, r.LorePackage)
		}
		obs = append(obs, "Lore packages available: "+strings.Join(unique(names), ", "))
	}

	if len(analysis.Unresolved) > 0 {
		var names []string
		for _, u := range analysis.Unresolved {
			names = append(names, u.Name)
		}
		obs = append(obs, "Unknown packages: "+strings.Join(names, ", "))
	}

	pm := result.packageManager()
	if len(analysis.Resolved) == 0 && len(analysis.Unresolved) == 0 && pm != "" {
		obs = append(obs, "Uses "+pm+" for installation")
	}

	return obs
}

func scanScript(f *os.File) *scanResult {
	r := newScanResult()
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		r.lineCount++
		line := scanner.Text()
		r.scanLine(line, r.lineCount)
	}

	return r
}

func (r *scanResult) scanLine(line string, lineNum int) {
	if m := rePlatformGuard.FindStringSubmatch(line); m != nil {
		r.platformGuard = m[1]
	}

	for _, p := range packagePatterns {
		if m := p.regex.FindStringSubmatch(line); m != nil {
			r.managers[p.manager] = true
			r.addInstalls(p.manager, m[1], p.multi, line, lineNum)
		}
	}

	r.hasGit = r.hasGit || reGitClone.MatchString(line)
	r.hasMake = r.hasMake || reMakeInstall.MatchString(line)
	r.hasCurl = r.hasCurl || reCurlPipe.MatchString(line)
}

func (r *scanResult) addInstalls(manager, match string, multi bool, line string, lineNum int) {
	line = strings.TrimSpace(line)

	if !multi {
		r.installs = append(r.installs, DetectedInstall{
			Line:    lineNum,
			Manager: manager,
			Name:    match,
			Command: line,
		})
		return
	}

	for _, pkg := range strings.Fields(match) {
		if strings.HasPrefix(pkg, "-") || pkg == "\\" {
			continue
		}
		r.installs = append(r.installs, DetectedInstall{
			Line:    lineNum,
			Manager: manager,
			Name:    pkg,
			Command: line,
		})
	}
}

func unique(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}
