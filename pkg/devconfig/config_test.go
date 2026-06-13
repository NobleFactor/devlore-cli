// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devconfig_test

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/devconfig"
)

// signingSection is a Go-typed section used to exercise the [devconfig.Section] family in tests.
type signingSection struct {
	devconfig.SectionBase
	Backend string
}

func TestSectionBase_Name(t *testing.T) {

	base := devconfig.NewSectionBase("signing")
	if base.Name() != "signing" {
		t.Errorf("Name() = %q, want %q", base.Name(), "signing")
	}
}

func TestDataSection_LookupNamesGet(t *testing.T) {

	section := devconfig.NewDataSection("lint.copyright", map[string]any{
		"enabled": true,
		"license": "auto",
	})

	if got := section.Name(); got != "lint.copyright" {
		t.Errorf("Name() = %q, want %q", got, "lint.copyright")
	}

	value, ok := section.Lookup("license")
	if !ok || value != "auto" {
		t.Errorf("Lookup(license) = %v, %v; want auto, true", value, ok)
	}

	enabled, ok := devconfig.Get[bool](section, "enabled")
	if !ok || !enabled {
		t.Errorf("Get[bool](enabled) = %v, %v; want true, true", enabled, ok)
	}

	if _, ok := devconfig.Get[int](section, "enabled"); ok {
		t.Error("Get[int](enabled) ok = true; want false (wrong type)")
	}

	names := section.Names()
	if len(names) != 2 || names[0] != "enabled" || names[1] != "license" {
		t.Errorf("Names() = %v; want [enabled license]", names)
	}
}

func TestDataSection_StarlarkGet(t *testing.T) {

	section := devconfig.NewDataSection("test", map[string]any{"tool_path": "build/devlore-test"})

	value, found, err := section.Get(starlark.String("tool_path"))
	if err != nil || !found {
		t.Fatalf("Get(tool_path) = _, %v, %v; want found, nil", found, err)
	}
	if got, _ := starlark.AsString(value); got != "build/devlore-test" {
		t.Errorf("Get(tool_path) value = %q, want %q", got, "build/devlore-test")
	}

	// A missing key is found=false with no error, so indexing raises loudly while membership returns false.
	if _, found, err := section.Get(starlark.String("absent")); found || err != nil {
		t.Errorf("Get(absent) = _, %v, %v; want false, nil", found, err)
	}

	// A non-string key is an error.
	if _, _, err := section.Get(starlark.MakeInt(1)); err == nil {
		t.Error("Get(int key) error = nil; want non-nil")
	}
}

func TestDataSection_Projection(t *testing.T) {

	section := devconfig.NewDataSection("lint.copyright", map[string]any{
		"enabled":  true,
		"exclude":  []string{"vendor/", "testdata/"},
		"patterns": map[string]any{"go": "rule"},
	})

	enabled, _, _ := section.Get(starlark.String("enabled"))
	if enabled.Truth() != starlark.True {
		t.Errorf("enabled projected to %v, want True", enabled)
	}

	exclude, _, _ := section.Get(starlark.String("exclude"))
	if list, ok := exclude.(*starlark.List); !ok || list.Len() != 2 {
		t.Errorf("exclude projected to %T, want *starlark.List of 2", exclude)
	}

	patterns, _, _ := section.Get(starlark.String("patterns"))
	if _, ok := patterns.(*starlark.Dict); !ok {
		t.Errorf("patterns projected to %T, want *starlark.Dict", patterns)
	}
}

func TestDataSection_ItemsIterate(t *testing.T) {

	section := devconfig.NewDataSection("s", map[string]any{"b": 2, "a": 1})

	items := section.Items()
	if len(items) != 2 {
		t.Fatalf("Items() len = %d, want 2", len(items))
	}
	if key, _ := starlark.AsString(items[0][0]); key != "a" {
		t.Errorf("Items()[0] key = %q, want a (sorted)", key)
	}

	var got []string
	iterator := section.Iterate()
	defer iterator.Done()
	var name starlark.Value
	for iterator.Next(&name) {
		text, _ := starlark.AsString(name)
		got = append(got, text)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Iterate() = %v, want [a b]", got)
	}
}

func TestDataSection_ValueContract(t *testing.T) {

	empty := devconfig.NewDataSection("e", map[string]any{})
	if empty.Truth() != starlark.False {
		t.Error("empty section Truth() = True, want False")
	}

	section := devconfig.NewDataSection("s", map[string]any{"k": "v"})
	if section.Truth() != starlark.True {
		t.Error("non-empty section Truth() = False, want True")
	}
	if section.Type() != "config.section" {
		t.Errorf("Type() = %q, want config.section", section.Type())
	}
	if _, err := section.Hash(); err == nil {
		t.Error("Hash() error = nil; want non-nil (unhashable)")
	}
}

func TestConfig_SectionAndSectionOf(t *testing.T) {

	signing := &signingSection{SectionBase: devconfig.NewSectionBase("signing"), Backend: "ssh"}
	lint := devconfig.NewDataSection("lint.copyright", map[string]any{"enabled": true})

	cfg := devconfig.NewConfig(
		map[string]devconfig.Section{"signing": signing, "lint.copyright": lint},
		nil,
	)

	if got, ok := cfg.Section("signing"); !ok || got.Name() != "signing" {
		t.Errorf("Section(signing) = %v, %v; want signing, true", got, ok)
	}
	if _, ok := cfg.Section("absent"); ok {
		t.Error("Section(absent) ok = true; want false")
	}

	typed, ok := devconfig.SectionOf[*signingSection](cfg)
	if !ok || typed.Backend != "ssh" {
		t.Errorf("SectionOf[*signingSection] = %v, %v; want backend ssh, true", typed, ok)
	}
}

func TestConfig_Provenance(t *testing.T) {

	cfg := devconfig.NewConfig(
		nil,
		map[string]map[string]devconfig.SettingSourceKind{
			"signing": {"backend": devconfig.SourceCLI, "key": devconfig.SourceDefaults},
		},
	)

	if source, ok := cfg.Provenance("signing", "backend"); !ok || source != devconfig.SourceCLI {
		t.Errorf("Provenance(signing, backend) = %v, %v; want cli, true", source, ok)
	}
	if _, ok := cfg.Provenance("signing", "absent"); ok {
		t.Error("Provenance(signing, absent) ok = true; want false")
	}
	if _, ok := cfg.Provenance("absent", "x"); ok {
		t.Error("Provenance(absent, x) ok = true; want false")
	}

	all := cfg.Provenances("signing")
	if len(all) != 2 || all["backend"] != devconfig.SourceCLI {
		t.Errorf("Provenances(signing) = %v; want 2 entries with backend=cli", all)
	}

	// The returned map is a copy; mutating it must not leak into the sealed Config.
	all["backend"] = devconfig.SourceEnv
	if source, _ := cfg.Provenance("signing", "backend"); source != devconfig.SourceCLI {
		t.Error("Provenances() returned a live map; mutation leaked into the Config")
	}
}

func TestSettingSourceKind_String(t *testing.T) {

	cases := map[devconfig.SettingSourceKind]string{
		devconfig.SourceBuiltin:  "builtin",
		devconfig.SourceDefaults: "defaults",
		devconfig.SourceApp:      "app",
		devconfig.SourceProject:  "project",
		devconfig.SourceEnv:      "env",
		devconfig.SourceCLI:      "cli",
	}
	for kind, want := range cases {
		if got := kind.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}
