// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// ToStarlark converts the PackageContext to a Starlark struct.
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
func (p *PackageContext) ToStarlark() starlark.Value {
	// Convert features to Starlark list
	featureList := make([]starlark.Value, len(p.Features))
	for i, f := range p.Features {
		featureList[i] = starlark.String(f)
	}

	// Convert settings to Starlark dict
	settingsDict := starlark.NewDict(len(p.Settings))
	for k, v := range p.Settings {
		_ = settingsDict.SetKey(starlark.String(k), starlark.String(v))
	}

	return starlarkstruct.FromStringDict(starlark.String("package"), starlark.StringDict{
		"name":        starlark.String(p.Name),
		"version":     starlark.String(p.Version),
		"features":    starlark.NewList(featureList),
		"settings":    settingsDict,
		"dry_run":     starlark.Bool(p.DryRun),
		"source_root": starlark.String(p.SourceRoot),
		"target_root": starlark.String(p.TargetRoot),
		"has_feature": starlark.NewBuiltin("package.has_feature", p.hasFeatureBuiltin),
		"setting":     starlark.NewBuiltin("package.setting", p.settingBuiltin),
	})
}

func (p *PackageContext) hasFeatureBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("has_feature", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.Bool(p.HasFeature(name)), nil
}

func (p *PackageContext) settingBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key string
	var defaultValue string
	if err := starlark.UnpackArgs("setting", args, kwargs, "key", &key, "default?", &defaultValue); err != nil {
		return nil, err
	}
	value := p.Setting(key)
	if value == "" {
		value = defaultValue
	}
	return starlark.String(value), nil
}
