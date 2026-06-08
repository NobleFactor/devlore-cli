// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// AddressingMode classifies a [Resource] by what its identity is grounded in.
//
// Two real modes: [AddressingLocation] for Resources whose identity is the place they live (file path, URL, repo path,
// service name) — bytes at the location are mutable, and the catalog uses shadow semantics to track changes.
// [AddressingContent] for Resources whose identity is their content digest — same URI implies same bytes by
// construction, so content "changes" mint new URIs rather than shadowing existing ones.
//
// [AddressingUnknown] is the zero-value sentinel. It is not a valid runtime classification — every concrete Resource
// type self-declares its mode by overriding [Resource.Addressing]. The boot-discipline test in
// pkg/op/addressing_test.go (added in 13.0(k) sub-step k.12) walks every announced Resource type and asserts none
// returns AddressingUnknown. The catalog's branch logic panics if it ever encounters AddressingUnknown at runtime.
type AddressingMode int

const (
	// AddressingUnknown is the zero-value sentinel. Concrete Resource types must override [Resource.Addressing]
	// to return one of the two real modes.
	AddressingUnknown AddressingMode = iota

	// AddressingLocation is for Resources whose identity is a location — file paths, URLs, repo paths, service names.
	// Used by file/git/appnet/pkg/service Resources. Catalog applies shadow semantics on content changes.
	AddressingLocation

	// AddressingContent is for Resources whose identity is their content digest. Used by mem/stream/function/json/yaml
	// Resources. URI takes the form tag:devlore.noblefactor.com,2026-01-01:<algo>:<hex>#<go-type-id>. Same URI implies
	// same content by construction; content changes mint new URIs.
	AddressingContent
)

// String returns the lowercase name of the addressing mode.
//
// Panics via [assert.Unreachable] when m holds an integer outside the declared constants — that state is a programming
// error (e.g., AddressingMode cast from an arbitrary int) and surfaces loudly rather than silently falling through.
func (m AddressingMode) String() string {

	switch m {
	case AddressingUnknown:
		return "unknown"
	case AddressingLocation:
		return "location"
	case AddressingContent:
		return "content"
	}

	assert.Unreachable(fmt.Sprintf("invalid value %d", m))
	return ""
}
