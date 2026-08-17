// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package scenario holds end-to-end scenarios that belong to no single command.
//
// A scenario drives the real binaries in a pristine sandbox, as an operator would, rather than calling
// functions in process. The writ-deploy scenario lives with writ because it is writ's alone; the self-install
// scenario lives here because every tool installs itself the same way, and the point is that they agree.
//
// Scenarios run under `make test-scenario`, gated by `DEVLORE_SCENARIO_RUN`, so `make test` stays fast.
package scenario
