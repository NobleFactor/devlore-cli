// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package provider_test

import (
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/appnet"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/git"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/pkg"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/service"
)

// TestConvert_NoSealedResourceIsReachedFromAStringOnTheTargetSide is the contract phase 8 of the sealed-resources
// plan (#649) leaves in place of the removed `ConvertFrom` / `CanConvertFrom` pairs.
//
// While the resources were structs, [op.Convert] step 7 probed `reflect.New(target)`, found a `*Resource` that
// implemented [op.TargetConverter], and let `ConvertFrom` mint a resource with no identity from a bare string. With
// every announced resource type an interface and the pairs gone, no string reaches a resource through the target
// side: env-less, the only path left is the registered constructor, which needs a runtime environment and interns
// through the catalog. Env-less isolates the case, because with an environment present step 6 wins first either way.
//
// If any row goes green by returning a resource, an identity-less mint has come back and the phase's premise is wrong.
func TestConvert_NoSealedResourceIsReachedFromAStringOnTheTargetSide(t *testing.T) {

	sealed := []struct {
		name   string
		target reflect.Type
	}{
		{"appnet.Resource", reflect.TypeFor[appnet.Resource]()},
		{"file.AnyKind", reflect.TypeFor[file.AnyKind]()},
		{"file.Directory", reflect.TypeFor[file.Directory]()},
		{"file.Regular", reflect.TypeFor[file.Regular]()},
		{"file.Resource", reflect.TypeFor[file.Resource]()},
		{"file.SymbolicLink", reflect.TypeFor[file.SymbolicLink]()},
		{"git.Resource", reflect.TypeFor[git.Resource]()},
		{"pkg.Resource", reflect.TypeFor[pkg.Resource]()},
		{"service.Resource", reflect.TypeFor[service.Resource]()},
	}

	for _, row := range sealed {
		t.Run(row.name, func(t *testing.T) {
			converted, err := op.Convert(nil, "some-name", row.target)
			if err == nil {
				t.Fatalf("op.Convert(env-less string → %s) = %#v, want an error: no target-side conversion may mint a resource",
					row.name, converted)
			}
		})
	}
}
