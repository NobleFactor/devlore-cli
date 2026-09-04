// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package bad is a fixture: a command package that builds its own rendering. NoPrivatePipeline must report
// the import.
package bad

import (
	"os"

	"github.com/NobleFactor/devlore-cli/pkg/result"
)

func render(value any) error {
	formatter, err := result.FormatterByName("json")
	if err != nil {
		return err
	}
	return formatter.Format(os.Stderr, value)
}
