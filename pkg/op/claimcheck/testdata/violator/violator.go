// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package violator carries claims its own code contradicts, so the checker has something to catch.
//
// It lives under testdata and is never built or announced: `./...` excludes testdata, so nothing here reaches the
// registry, the conformance test, or a binary. It exists only for [claimcheck.Check] to be pointed at.
//
// Every function below is a shape the checker must report. A green conformance run proves no real claim is false;
// only these prove the checker can tell.
package violator

import (
	"os"
	"text/template"
)

// DirectCall claims determinism and reads the ambient environment in its own body.
//
// The simplest violation there is: a capability call, in the claiming function, one hop from the claim.
//
// +devlore:claim=deterministic
func DirectCall() string {
	return os.Getenv("HOME")
}

// FunctionValue claims determinism and never calls a capability — it stores one.
//
// This is the shape that motivated using types over syntax. `os.Getenv` here is an *ast.SelectorExpr in a
// composite literal, not an *ast.CallExpr, so a scan that inspects call expressions walks straight past it. It is
// also the exact shape of the one confirmed hermeticity violation in the catalog (#683): template's render-time
// function map maps `Env` to `os.Getenv`, and a template string decides whether it fires.
//
// +devlore:claim=deterministic
func FunctionValue() template.FuncMap {
	return template.FuncMap{"Env": os.Getenv}
}

// ThroughHelper claims determinism and reaches a capability only through a call to a function beside it.
//
// Its own body is clean. Nothing about it is suspicious until propagation follows the hop, which is why a
// body-only check would pass it.
//
// +devlore:claim=deterministic
func ThroughHelper() string {
	return helper()
}

// UnsandboxedRead claims its I/O is bounded and reads a path directly instead.
//
// os.ReadFile resolves against the process working directory, not against an fsroot.Dir, so the read has no
// boundary at all.
//
// +devlore:claim=sandboxed
func UnsandboxedRead(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// TypesAndConstantsOnly claims determinism and touches `os` only for a type and a constant.
//
// It must NOT be reported. os.FileMode is a type, os.ModePerm is a constant, and os.FileMode(0o644) is a
// conversion that parses as a call expression — three shapes a syntactic scan flags and a type-checked one does
// not. This is the false-positive guard: without it, ten real methods in the file provider would fail.
//
// +devlore:claim=deterministic
func TypesAndConstantsOnly() os.FileMode {
	return os.FileMode(0o644) & os.ModePerm
}

// helper is the hop [ThroughHelper] reaches its capability through.
func helper() string {
	return os.Getenv("PATH")
}
