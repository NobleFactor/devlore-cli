// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package regexp provides regular expression operations for the operation graph.
package regexp

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides regular expression operations with compiled pattern caching.
// +devlore:access=both
type Provider struct {
	op.ProviderBase
	cache sync.Map // pattern string → *regexp.Regexp
}

// compile returns a compiled regexp, caching it for reuse.
func (p *Provider) compile(pattern string) (*regexp.Regexp, error) {
	if v, ok := p.cache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("regexp compile: %w", err)
	}
	p.cache.Store(pattern, re)
	return re, nil
}

// Match reports whether the string contains any match of the pattern.
func (p *Provider) Match(pattern, s string) (bool, error) {
	re, err := p.compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(s), nil
}

// Find returns the first match of the pattern in the string.
// Returns an empty string if no match is found.
func (p *Provider) Find(pattern, s string) (string, error) {
	re, err := p.compile(pattern)
	if err != nil {
		return "", err
	}
	return re.FindString(s), nil
}

// FindAll returns all non-overlapping matches of the pattern.
// The count parameter limits the number of matches; -1 means no limit.
func (p *Provider) FindAll(pattern, s string, count int) ([]string, error) {
	re, err := p.compile(pattern)
	if err != nil {
		return nil, err
	}
	return re.FindAllString(s, count), nil
}

// FindSubmatch returns the first match and its submatches.
// Returns nil if no match is found.
func (p *Provider) FindSubmatch(pattern, s string) ([]string, error) {
	re, err := p.compile(pattern)
	if err != nil {
		return nil, err
	}
	return re.FindStringSubmatch(s), nil
}

// FindAllSubmatch returns all matches with their submatches.
// The count parameter limits the number of matches; -1 means no limit.
func (p *Provider) FindAllSubmatch(pattern, s string, count int) ([][]string, error) {
	re, err := p.compile(pattern)
	if err != nil {
		return nil, err
	}
	return re.FindAllStringSubmatch(s, count), nil
}

// Replace replaces all matches of the pattern with the replacement string.
// The replacement can include $1, $2, etc. for submatch references.
func (p *Provider) Replace(pattern, s, replacement string) (string, error) {
	re, err := p.compile(pattern)
	if err != nil {
		return "", err
	}
	return re.ReplaceAllString(s, replacement), nil
}

// ReplaceLiteral replaces all matches with the literal replacement string
// (no submatch expansion).
func (p *Provider) ReplaceLiteral(pattern, s, replacement string) (string, error) {
	re, err := p.compile(pattern)
	if err != nil {
		return "", err
	}
	return re.ReplaceAllLiteralString(s, replacement), nil
}

// Split splits the string around matches of the pattern.
// The count parameter limits the number of substrings; -1 means no limit.
func (p *Provider) Split(pattern, s string, count int) ([]string, error) {
	re, err := p.compile(pattern)
	if err != nil {
		return nil, err
	}
	return re.Split(s, count), nil
}
