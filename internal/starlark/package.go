// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// packageContextReceiver wraps PackageContext as a Starlark receiver.
// Exposed to phase scripts as the second argument.
//
// Starlark API:
//
//	package.name        # string: package name
//	package.version     # string: package version
//	package.features    # list[string]: enabled features
//	package.settings    # dict[string, string]: key-value settings
//	package.dry_run     # bool: true if this is a preview
//	package.source_root # string: package source directory
//	package.target_root # string: deployment target directory
//	package.has_feature(name)  # bool: check if feature is enabled
//	package.setting(key, default="")  # string: get setting value
type packageContextReceiver struct {
	ctx *PackageContext

	// Cached Starlark values (built once)
	features *starlark.List
	settings *starlark.Dict
}

func (r *packageContextReceiver) String() string        { return "package" }
func (r *packageContextReceiver) Type() string          { return "package" }
func (r *packageContextReceiver) Freeze()               {}
func (r *packageContextReceiver) Truth() starlark.Bool  { return true }
func (r *packageContextReceiver) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: package") }

func (r *packageContextReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "name":
		return starlark.String(r.ctx.Name), nil
	case "version":
		return starlark.String(r.ctx.Version), nil
	case "features":
		return r.features, nil
	case "settings":
		return r.settings, nil
	case "dry_run":
		return starlark.Bool(r.ctx.DryRun), nil
	case "source_root":
		return starlark.String(r.ctx.SourceRoot), nil
	case "target_root":
		return starlark.String(r.ctx.TargetRoot), nil
	case "has_feature":
		return starlark.NewBuiltin("package.has_feature", r.hasFeature), nil
	case "setting":
		return starlark.NewBuiltin("package.setting", r.setting), nil
	default:
		return nil, op.NoSuchAttrError("package", name)
	}
}

func (r *packageContextReceiver) AttrNames() []string {
	return []string{
		"dry_run", "features", "has_feature", "name", "setting",
		"settings", "source_root", "target_root", "version",
	}
}

func (r *packageContextReceiver) hasFeature(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("has_feature", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.Bool(r.ctx.HasFeature(name)), nil
}

func (r *packageContextReceiver) setting(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key string
	var defaultValue string
	if err := starlark.UnpackArgs("setting", args, kwargs, "key", &key, "default?", &defaultValue); err != nil {
		return nil, err
	}
	value := r.ctx.Setting(key)
	if value == "" {
		value = defaultValue
	}
	return starlark.String(value), nil
}

// ToStarlark converts the PackageContext to a Starlark receiver.
func (p *PackageContext) ToStarlark() starlark.Value {
	featureList := make([]starlark.Value, len(p.Features))
	for i, f := range p.Features {
		featureList[i] = starlark.String(f)
	}

	settingsDict := starlark.NewDict(len(p.Settings))
	for k, v := range p.Settings {
		_ = settingsDict.SetKey(starlark.String(k), starlark.String(v)) //nolint:errcheck // SetKey on fresh dict cannot fail
	}

	return &packageContextReceiver{
		ctx:      p,
		features: starlark.NewList(featureList),
		settings: settingsDict,
	}
}
