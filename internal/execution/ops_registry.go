// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import "fmt"

// ValidateOp checks a precondition and fails with a message if unmet.
// The check function is retrieved from ctx.Data["validators"][node's "check" slot].
// Hand-written: map[string]func() error is not in the generator type mappings.
type ValidateOp struct{}

func (o *ValidateOp) Name() string { return "validate" }

func (o *ValidateOp) Execute(ctx *Context, node *Node) error {
	checkName, _ := node.GetSlot("check").(string)
	if checkName == "" {
		return fmt.Errorf("validate: no check specified in node slots")
	}

	validators, ok := ctx.Data["validators"].(map[string]func() error)
	if !ok {
		return fmt.Errorf("validate: no validators configured in context")
	}

	validator, ok := validators[checkName]
	if !ok {
		return fmt.Errorf("validate: unknown check %q", checkName)
	}

	if err := validator(); err != nil {
		message, _ := node.GetSlot("message").(string)
		if message != "" {
			return fmt.Errorf("%s: %w", message, err)
		}
		return err
	}

	return nil
}

// AllOps returns all operations for registration.
// Both writ and lore use this to ensure the same operations are available.
func AllOps() []Operation {
	var ops []Operation
	ops = append(ops, FileOps(&FileService{})...)
	ops = append(ops, EncryptionOps(&EncryptionService{})...)
	ops = append(ops, PackageOps(&PackageService{})...)
	ops = append(ops, ShellOps(&ShellService{})...)
	ops = append(ops, ServiceManagerOps(&ServiceManagerService{})...)
	ops = append(ops, &ValidateOp{})
	return ops
}

