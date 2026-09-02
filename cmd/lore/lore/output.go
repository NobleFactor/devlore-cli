// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package lore

import (
	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
)

// outputOptions is the common set as the root binds it: `--output`, `--filter`, `--jq`, `--store`. One
// value for the process, because one cobra process runs one command tree.
var outputOptions cli.SinkOptions

// emitResult renders a command's result through the shared pipeline.
//
// Parameters:
//   - `cmd`: the running command, for its output writer.
//   - `value`: the result to render.
//
// Returns:
//   - `error`: non-nil when the pipeline cannot be built or the value cannot be rendered.
func emitResult(cmd *cobra.Command, value any) error {

	pipeline, err := cli.BuildPipeline(outputOptions, cmd.OutOrStdout())
	if err != nil {
		return err
	}

	return pipeline.Emit(value)
}
