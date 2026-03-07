// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// FatalError signals that the graph must halt immediately.
// The executor unwinds the recovery stack when it encounters this error.
//
// Defined in pkg/op/ so the executor can check for it via errors.As
// without importing the flow package.
type FatalError struct {
	Message string
}

func (e *FatalError) Error() string { return "fatal: " + e.Message }
