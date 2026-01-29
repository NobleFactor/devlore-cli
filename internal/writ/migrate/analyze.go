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
	RelPath        string   // Relative path from source root
	Name           string   // Base filename
	Phase          string   // "install" or "initialize"
	PackageNames   []string // Extracted package names
	PackageManager string   // Detected package manager
	PlatformGuard  string   // Platform guard if detected
	LineCount      int
	Observations   []string // Grounded observations about what the script does
}

var (
	reBrewInstall   = regexp.MustCompile(`brew\s+install\s+(?:--\S+\s+)*(\S+)`)
	reAptInstall    = regexp.MustCompile(`(?:apt|apt-get)\s+install\s+(?:-\S+\s+)*(.+)`)
	reCargoInstall  = regexp.MustCompile(`cargo\s+install\s+(\S+)`)
	rePortInstall   = regexp.MustCompile(`port\s+install\s+(\S+)`)
	rePipInstall    = regexp.MustCompile(`pip[3]?\s+install\s+(\S+)`)
	rePlatformGuard = regexp.MustCompile(`require_(linux|darwin|unix|windows|debian)`)
	reCurlPipe      = regexp.MustCompile(`curl\s+.*\|\s*(sh|bash)`)
	reGitClone      = regexp.MustCompile(`git\s+clone\s+\S+`)
	reMakeInstall   = regexp.MustCompile(`make\s+install`)
)

// AnalyzeScripts examines lifecycle script entries and extracts information
// about what they install and how.
func AnalyzeScripts(entries []InventoryEntry) []ScriptAnalysis {
	var analyses []ScriptAnalysis
	for _, e := range entries {
		if e.Class != ClassLifecycleScript {
			continue
		}
		analysis := analyzeScript(e)
		analyses = append(analyses, analysis)
	}
	return analyses
}

func analyzeScript(e InventoryEntry) ScriptAnalysis {
	name := strings.TrimSuffix(e.RelPath, "/")
	parts := strings.Split(name, string(os.PathSeparator))
	baseName := parts[len(parts)-1]

	analysis := ScriptAnalysis{
		RelPath: e.RelPath,
		Name:    baseName,
	}

	// Determine phase from prefix
	if strings.HasPrefix(baseName, "Install-") {
		analysis.Phase = "install"
	} else if strings.HasPrefix(baseName, "Initialize-") {
		analysis.Phase = "initialize"
	}

	// Parse the script file
	f, err := os.Open(e.AbsPath)
	if err != nil {
		analysis.Observations = append(analysis.Observations, "Could not read script: "+err.Error())
		return analysis
	}
	defer f.Close()

	var (
		packages  []string
		managers  []string
		lineCount int
		hasGit    bool
		hasMake   bool
		hasCurl   bool
	)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineCount++
		line := scanner.Text()

		// Platform guard
		if m := rePlatformGuard.FindStringSubmatch(line); m != nil {
			analysis.PlatformGuard = m[1]
		}

		// Package managers
		if m := reBrewInstall.FindStringSubmatch(line); m != nil {
			packages = appendUnique(packages, m[1])
			managers = appendUnique(managers, "brew")
		}
		if m := reAptInstall.FindStringSubmatch(line); m != nil {
			// apt install can have multiple packages on one line
			for _, pkg := range strings.Fields(m[1]) {
				if strings.HasPrefix(pkg, "-") || pkg == "\\" {
					continue
				}
				packages = appendUnique(packages, pkg)
			}
			managers = appendUnique(managers, "apt")
		}
		if m := reCargoInstall.FindStringSubmatch(line); m != nil {
			packages = appendUnique(packages, m[1])
			managers = appendUnique(managers, "cargo")
		}
		if m := rePortInstall.FindStringSubmatch(line); m != nil {
			packages = appendUnique(packages, m[1])
			managers = appendUnique(managers, "port")
		}
		if m := rePipInstall.FindStringSubmatch(line); m != nil {
			packages = appendUnique(packages, m[1])
			managers = appendUnique(managers, "pip")
		}
		if reCurlPipe.MatchString(line) {
			hasCurl = true
		}
		if reGitClone.MatchString(line) {
			hasGit = true
		}
		if reMakeInstall.MatchString(line) {
			hasMake = true
		}
	}

	analysis.LineCount = lineCount
	analysis.PackageNames = packages

	// Determine primary package manager
	switch {
	case len(managers) == 1:
		analysis.PackageManager = managers[0]
	case len(managers) > 1:
		analysis.PackageManager = strings.Join(managers, "/")
	case hasGit && hasMake:
		analysis.PackageManager = "source"
	case hasCurl:
		analysis.PackageManager = "curl"
	}

	// Generate observations
	if len(packages) > 0 {
		obs := "Installs " + strings.Join(packages, ", ")
		if analysis.PackageManager != "" {
			obs += " via " + analysis.PackageManager
		}
		analysis.Observations = append(analysis.Observations, obs)
	} else if analysis.PackageManager != "" {
		analysis.Observations = append(analysis.Observations,
			"Uses "+analysis.PackageManager+" for installation")
	}

	return analysis
}

func appendUnique(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}
