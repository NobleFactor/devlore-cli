// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"reflect"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// region Receiver params registry

// receiverEntry pairs a ReceiverFactory with its MethodParams.
type receiverEntry struct {
	factory op.ReceiverFactory
	params  MethodParams
}

// receiverParamsRegistry maps reflect.Type (struct, not pointer) to
// receiverEntry. When marshalReflect encounters a pointer to a registered
// type, it calls WrapProviderInExecutingReceiver instead of flattening to fields.
var receiverParamsRegistry sync.Map

// RegisterReceiverParams stores receiver params for a provider type.
// Called by RegisterActions as a side effect for providers with
// actions, and directly by immediate-only providers in their Register()
// callback.
func RegisterReceiverParams(factory op.ReceiverFactory, params MethodParams) {
	registerReceiverParamsReflect(factory, params)
}

func registerReceiverParamsReflect(factory op.ReceiverFactory, params MethodParams) {
	providerType := factory.ProviderType()
	if providerType.Kind() == reflect.Ptr {
		providerType = providerType.Elem()
	}
	receiverParamsRegistry.Store(providerType, receiverEntry{factory: factory, params: params})
}

func lookupReceiverParams(t reflect.Type) (receiverEntry, bool) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	v, ok := receiverParamsRegistry.Load(t)
	if !ok {
		return receiverEntry{}, false
	}
	return v.(receiverEntry), true
}

// endregion

// region Type params registry

// typeParamsRegistry maps reflect.Type (struct, not pointer) to MethodParams.
// Types like yaml.Resource register their parameterized methods here so
// discoverMethods can expose them as Starlark callables instead of filtering
// them out.
var typeParamsRegistry sync.Map

// RegisterTypeParams stores method parameter metadata for a struct type.
func RegisterTypeParams(t reflect.Type, params MethodParams) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	typeParamsRegistry.Store(t, params)
}

// endregion

// region Type cache

// typeCache stores struct introspection results, keyed by reflect.Type.
// Computed once per type, concurrent-safe, amortized O(1) lookups.
var typeCache sync.Map

// endregion
