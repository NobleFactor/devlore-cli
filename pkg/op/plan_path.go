// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"sync"
)

// PlanPathNormalizer renders an authored plan-space path into its canonical rel form, or refuses it.
//
// The plan-space grammar belongs to the scheme that owns the resource type (the file scheme's git model:
// docs/architecture/4-resource-management.md §5.2), so the framework carries only this seam: a resource type
// registers its normalizer, and [ActionPlanner] applies it to string values bound to that type's parameters
// at plan time. Immediate-mode construction and programmatic Go callers are untouched — the little language
// governs what a PLAN may say, not what a session may do.
type PlanPathNormalizer func(path string) (string, error)

var (
	// planPathNormalizersMu guards planPathNormalizers. Registration happens in provider init functions;
	// lookups run on every plan-time slot bind.
	planPathNormalizersMu sync.RWMutex

	// planPathNormalizers maps a resource's reflect.Type to its plan-space normalizer.
	planPathNormalizers = make(map[reflect.Type]PlanPathNormalizer)
)

// RegisterPlanPathNormalizer registers `normalize` as the plan-space grammar for resource type `t`.
//
// Called from provider init alongside the type's announcement. Registering the value type covers the
// pointer spelling and vice versa — lookup tries both forms.
//
// Parameters:
//   - `t`: the resource's reflect.Type (value or pointer form).
//   - `normalize`: the scheme's plan-space normalizer.
func RegisterPlanPathNormalizer(t reflect.Type, normalize PlanPathNormalizer) {

	planPathNormalizersMu.Lock()
	defer planPathNormalizersMu.Unlock()

	planPathNormalizers[t] = normalize
}

// planPathNormalizerFor returns the registered normalizer for `t`, trying the exact type, its element form,
// and its pointer form — the same fallback ladder the registry's type lookups use.
//
// Parameters:
//   - `t`: the parameter's reflect.Type.
//
// Returns:
//   - `PlanPathNormalizer`: the registered normalizer, or nil.
//   - `bool`: true when a normalizer is registered for `t` in any spelling.
func planPathNormalizerFor(t reflect.Type) (PlanPathNormalizer, bool) {

	planPathNormalizersMu.RLock()
	defer planPathNormalizersMu.RUnlock()

	if normalize, ok := planPathNormalizers[t]; ok {
		return normalize, true
	}

	if t.Kind() == reflect.Pointer {
		if normalize, ok := planPathNormalizers[t.Elem()]; ok {
			return normalize, true
		}
	} else {
		if normalize, ok := planPathNormalizers[reflect.PointerTo(t)]; ok {
			return normalize, true
		}
	}

	return nil, false
}
