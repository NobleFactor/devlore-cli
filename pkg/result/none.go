// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

import "io"

// NoneFormatter renders nothing at all.
//
// This is not the same as redirecting to the null device. Redirection renders the value and then discards
// the bytes; this never renders it. The distinction matters where no shell exists to redirect -- a config
// file or an environment variable can select a rendering, and "no result" has to be one of them.
//
// `aws` ships the same value as `off`; `az` and `gcloud` spell it `none`.
type NoneFormatter struct{}

// Compile-time interface guard.
var _ Formatter = NoneFormatter{}

// Format writes nothing and reports no error.
//
// Parameters:
//   - `value`: ignored.
//   - `w`: never written to.
//
// Returns:
//   - `error`: always nil.
func (NoneFormatter) Format(_ any, _ io.Writer) error { return nil }
