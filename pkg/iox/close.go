// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package iox provides standalone I/O utilities independent of the op framework.
package iox

import (
	"errors"
	"io"
)

// Close closes all provided closers, joining any errors into *err.
// Nil closers are safely skipped. Use with named returns:
//
//	defer iox.Close(&err, f, enc)
//
// Parameters:
//   - err: pointer to the named error return; close errors are joined into *err
//   - closers: values to close; nil entries are skipped
func Close(err *error, closers ...io.Closer) {

	for _, c := range closers {
		if c != nil {
			*err = errors.Join(*err, c.Close())
		}
	}
}
