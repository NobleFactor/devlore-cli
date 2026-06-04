// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lore

import "github.com/NobleFactor/devlore-cli/pkg/op"

// Origin is lore's typed, read-only view over a graph's [op.Origin].
//
// It wraps the framework [op.Origin] (rather than embedding [op.OriginBase]) so the view survives a load round-trip:
// the stored concrete type is always [op.OriginBase], and wrapping the interface keeps the projection valid whether
// the origin was freshly built or decoded from a persisted graph. The projected accessors read lore's stamped
// annotation keys — `packages`, `platform`, `features`, `settings` — coercing the decoded `[]any` / `map[string]any`
// shapes back to their typed forms.
type Origin struct {
	op.Origin
}

// NewOrigin wraps a framework [op.Origin] in lore's typed view.
//
// Parameters:
//   - `origin`: the graph origin to project (typically `graph.Origin()`).
//
// Returns:
//   - `Origin`: the lore view.
func NewOrigin(origin op.Origin) Origin {
	return Origin{Origin: origin}
}

// region EXPORTED METHODS

// region State Management

// Features returns the lore feature flags stamped on the origin.
//
// Returns:
//   - `[]string`: the enabled features; nil when none were stamped.
func (o Origin) Features() []string {
	return annotationStringSlice(o.Annotations(), "features")
}

// Packages returns the package names the graph deploys.
//
// Returns:
//   - `[]string`: the package names; nil when none were stamped.
func (o Origin) Packages() []string {
	return annotationStringSlice(o.Annotations(), "packages")
}

// Platform returns the target platform token the graph was planned for.
//
// Returns:
//   - `string`: the platform token (e.g. "Linux.Debian"); "" when unset.
func (o Origin) Platform() string {
	value, _ := o.Annotations().Get("platform")
	token, _ := value.(string)
	return token
}

// Settings returns the lore configuration settings stamped on the origin.
//
// Returns:
//   - `map[string]string`: the settings; nil when none were stamped.
func (o Origin) Settings() map[string]string {
	return annotationStringMap(o.Annotations(), "settings")
}

// endregion

// endregion

// region HELPER FUNCTIONS

// annotationStringSlice reads `key` from `annotations` as a string slice, coercing the `[]any` shape a load produces.
//
// Parameters:
//   - `annotations`: the origin's annotation map.
//   - `key`: the annotation key to read.
//
// Returns:
//   - `[]string`: the coerced slice; nil when the key is absent or not slice-shaped.
func annotationStringSlice(annotations op.AnnotationMap, key string) []string {

	value, ok := annotations.Get(key)
	if !ok {
		return nil
	}

	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, element := range typed {
			if s, ok := element.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// annotationStringMap reads `key` from `annotations` as a string map, coercing the `map[string]any` shape a load
// produces.
//
// Parameters:
//   - `annotations`: the origin's annotation map.
//   - `key`: the annotation key to read.
//
// Returns:
//   - `map[string]string`: the coerced map; nil when the key is absent or not map-shaped.
func annotationStringMap(annotations op.AnnotationMap, key string) map[string]string {

	value, ok := annotations.Get(key)
	if !ok {
		return nil
	}

	switch typed := value.(type) {
	case map[string]string:
		return typed
	case map[string]any:
		out := make(map[string]string, len(typed))
		for k, element := range typed {
			if s, ok := element.(string); ok {
				out[k] = s
			}
		}
		return out
	default:
		return nil
	}
}

// endregion