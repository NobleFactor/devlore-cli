// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devconfig_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/devconfig"
)

// goConstructor returns a Go-path constructor that builds a section reporting the given name. The concrete shape is
// immaterial to the registry, so a DataSection stands in for a Go-typed section here.
func goConstructor(name string) devconfig.SectionConstructor {
	return func() devconfig.Section { return devconfig.NewDataSection(name, nil) }
}

func TestAnnounceSection_RegistersAndConstructs(t *testing.T) {

	devconfig.AnnounceSection(reflect.TypeFor[devconfig.DataSection](), goConstructor("reg-alpha"))

	if !slices.Contains(devconfig.AnnouncedSectionNames(), "reg-alpha") {
		t.Errorf("AnnouncedSectionNames() = %v; want it to contain reg-alpha", devconfig.AnnouncedSectionNames())
	}

	construct, ok := devconfig.ConstructorFor("reg-alpha")
	if !ok {
		t.Fatal("ConstructorFor(reg-alpha) ok = false; want true")
	}
	if got := construct().Name(); got != "reg-alpha" {
		t.Errorf("constructor built %q, want reg-alpha", got)
	}
	if _, ok := devconfig.SpecFor("reg-alpha"); ok {
		t.Error("SpecFor(reg-alpha) ok = true; want false (the Go path has no spec)")
	}
}

func TestAnnounceSection_DuplicateIsFatal(t *testing.T) {

	devconfig.AnnounceSection(reflect.TypeFor[devconfig.DataSection](), goConstructor("reg-dup"))

	defer func() {
		if recover() == nil {
			t.Error("duplicate AnnounceSection did not panic")
		}
	}()
	devconfig.AnnounceSection(reflect.TypeFor[devconfig.DataSection](), goConstructor("reg-dup"))
}

func TestAnnounceSectionSpec_RegistersAndReturnsSpec(t *testing.T) {

	if err := devconfig.AnnounceSectionSpec(devconfig.SectionSpec{Name: "spec-alpha"}); err != nil {
		t.Fatalf("AnnounceSectionSpec returned %v", err)
	}

	got, ok := devconfig.SpecFor("spec-alpha")
	if !ok || got.Name != "spec-alpha" {
		t.Errorf("SpecFor(spec-alpha) = %v, %v; want spec-alpha, true", got, ok)
	}
	if _, ok := devconfig.ConstructorFor("spec-alpha"); ok {
		t.Error("ConstructorFor(spec-alpha) ok = true; want false (the data path has no constructor)")
	}
}

func TestAnnounceSectionSpec_DuplicateReturnsError(t *testing.T) {

	if err := devconfig.AnnounceSectionSpec(devconfig.SectionSpec{Name: "spec-dup"}); err != nil {
		t.Fatalf("first announce: %v", err)
	}
	if err := devconfig.AnnounceSectionSpec(devconfig.SectionSpec{Name: "spec-dup"}); err == nil {
		t.Error("duplicate AnnounceSectionSpec error = nil; want non-nil")
	}
}

func TestAnnounceSectionSpec_EmptyNameReturnsError(t *testing.T) {

	if err := devconfig.AnnounceSectionSpec(devconfig.SectionSpec{}); err == nil {
		t.Error("empty-name AnnounceSectionSpec error = nil; want non-nil")
	}
}

func TestAnnounce_GoNameCannotBeHijackedBySpec(t *testing.T) {

	devconfig.AnnounceSection(reflect.TypeFor[devconfig.DataSection](), goConstructor("reg-cross"))

	if err := devconfig.AnnounceSectionSpec(devconfig.SectionSpec{Name: "reg-cross"}); err == nil {
		t.Error("a spec claiming a Go-announced name: error = nil; want non-nil (G1)")
	}
}
