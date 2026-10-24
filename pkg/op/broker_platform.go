// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// region EXPORTED FUNCTIONS

// PlatformRunner returns the [platform.CommandRunner] that runs a platform manager's commands through the
// environment's broker: the caller owns the broker, the leaf receives the runner (6.2 § Open 2).
//
// Each command becomes an exec on the run `host`, its unit the account it asked for, with the manager's environment
// and the auto-confirm stdin, bounded by [platform.CommandTimeout]; the outcome folds back into a [platform.Result]
// as the platform's own runner shaped it, trailing newlines trimmed. A refusal is a Result with Code -1 naming it.
//
// Parameters:
//   - `runtimeEnvironment`: the environment whose broker and context the commands use.
//
// Returns:
//   - `platform.CommandRunner`: the runner.
func PlatformRunner(runtimeEnvironment *RuntimeEnvironment) platform.CommandRunner {

	return func(ctx context.Context, request platform.CommandRequest) platform.Result {

		if ctx == nil || ctx == context.Background() {
			ctx = runtimeEnvironment.Context
		}
		ctx, cancel := context.WithTimeout(ctx, platform.CommandTimeout)
		defer cancel()

		exec := ExecRequest{
			Run:     platformRun,
			Unit:    request.Account,
			Account: request.Account,
			Argv:    request.Argv,
			Env:     request.Env,
		}
		if request.AutoConfirm {
			exec.Stdin = &Stdin{AutoConfirm: true}
		}

		outcome, err := Run(ctx, runtimeEnvironment.Broker, exec)
		if err != nil {
			return platform.Result{Code: -1, Stderr: err.Error()}
		}

		stderr := strings.TrimSuffix(outcome.Stderr(), "\n")
		if outcome.Error != "" {
			stderr = strings.TrimSpace(stderr + "\n" + outcome.Error)
		}
		return platform.Result{
			OK:     outcome.ExitStatus == 0 && outcome.Error == "",
			Stdout: strings.TrimSuffix(string(outcome.Output), "\n"),
			Stderr: stderr,
			Code:   outcome.ExitStatus,
		}
	}
}

// endregion

// platformRun is the run id the platform's commands carry until the executor gives runs ids of their own.
const platformRun RunID = "host"
