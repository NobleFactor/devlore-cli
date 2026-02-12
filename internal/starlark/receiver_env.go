// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"os"

	"go.starlark.net/starlark"
)

// EnvReceiver provides the env.* Starlark namespace.
//
// Backing implementation: os package (Getenv, Setenv, ExpandEnv).
// No backing struct — delegates directly to the standard library.
type EnvReceiver struct {
	Receiver
}

// NewEnvReceiver creates a new env receiver.
func NewEnvReceiver() *EnvReceiver {
	return &EnvReceiver{Receiver: NewReceiver("env")}
}

func (r *EnvReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "get":
		return MakeAttr("env.get", r.get), nil
	case "set":
		return MakeAttr("env.set", r.set), nil
	case "expand":
		return MakeAttr("env.expand", r.expand), nil
	default:
		return nil, NoSuchAttrError("env", name)
	}
}

func (r *EnvReceiver) AttrNames() []string {
	return []string{"expand", "get", "set"}
}

func (r *EnvReceiver) get(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, defaultValue string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name, "default?", &defaultValue); err != nil {
		return nil, err
	}

	value := os.Getenv(name)
	if value == "" {
		value = defaultValue
	}
	return starlark.String(value), nil
}

func (r *EnvReceiver) set(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, value string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name, "value", &value); err != nil {
		return nil, err
	}

	_ = os.Setenv(name, value)
	return starlark.None, nil
}

func (r *EnvReceiver) expand(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var template string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "template", &template); err != nil {
		return nil, err
	}

	return starlark.String(os.ExpandEnv(template)), nil
}
