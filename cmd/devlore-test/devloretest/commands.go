// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package devloretest

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
)

func newRunCmd(opts *cli.SinkOptions) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "run [flags] <script.star>",
		Short: "Run a Starlark test script that plans and executes a graph",
		Long: `Run a Starlark test script through the graph execution engine.

The script uses plan.* bindings to build a graph and t.* assertions to
verify expectations after execution.

The result -- pass/fail, unit and expectation counts, failures, and each
t.run's return value -- goes to stdout, rendered by --output. Narration
goes to stderr. The definition and its execution traces go to the
execution store, which --store relocates.`,
		Example: `  devlore-test run test.star
  devlore-test run --dry-run test.star
  devlore-test run --trace test.star
  devlore-test run -o yaml test.star
  devlore-test run -o none test.star
  devlore-test run --store ./run test.star`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTest(cmd, args[0], opts)
		},
	}

	cmd.Flags().Bool("dry-run", false, "Plan only, no side effects")
	cmd.Flags().Bool("trace", false, "Enable Starlark step trace")
	cmd.Flags().String("provider", "", "Restrict to a specific provider")

	return cmd
}

func runTest(cmd *cobra.Command, script string, opts *cli.SinkOptions) (err error) {
	dryRun, _ := cmd.Flags().GetBool("dry-run")      //nolint:errcheck // flag registered above
	trace, _ := cmd.Flags().GetBool("trace")         //nolint:errcheck // flag registered above
	provider, _ := cmd.Flags().GetString("provider") //nolint:errcheck // flag registered above

	// The script is verified before anything is written: a run that cannot start leaves no artifacts.
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("reading script: %w", err)
	}

	// Documents -- the definition and its traces -- go to the execution store. --store relocates the whole
	// store; unset, it is devlore's XDG state home.
	if opts.Store != "" {
		defer cli.SetStoreRoot(opts.Store)()
	}

	runOptions := []Option{}
	if dryRun {
		runOptions = append(runOptions, WithDryRun())
	}
	if trace {
		runOptions = append(runOptions, WithTrace())
	}
	if provider != "" {
		runOptions = append(runOptions, WithProvider(provider))
	}

	runner := NewRunner(script, runOptions...)
	result, err := runner.Start(cmd.Context())
	if err != nil {
		return err
	}

	if err := storeDocuments(runner); err != nil {
		return err
	}

	// The result goes to stdout, rendered by --output. Narration has been on stderr throughout.
	pipeline, err := cli.BuildPipeline(*opts, cmd.OutOrStdout())
	if err != nil {
		return err
	}
	if err := pipeline.Emit(result); err != nil {
		return fmt.Errorf("writing the result: %w", err)
	}

	if !result.Passed {
		return cli.Failure("%d expectation(s) failed", len(result.Failures))
	}

	return nil
}

// storeDocuments writes the run's definition and traces to the execution store.
//
// A definition persists once, keyed by its checksum; each trace persists beneath it, tied back through
// [op.Trace.GraphChecksum]. A script that assembled no graph stores nothing, which is not a failure.
//
// Parameters:
//   - `runner`: the finished runner holding the graph and its traces.
//
// Returns:
//   - `error`: non-nil when a document cannot be written.
func storeDocuments(runner *Runner) error {

	graph := runner.Graph()
	if graph == nil {
		return nil
	}

	path, err := cli.WriteGraph(graph)
	if err != nil {
		return fmt.Errorf("writing the definition to the store: %w", err)
	}
	cli.Note("definition %s", path)

	for _, trace := range runner.Traces() {
		tracePath, err := cli.WriteTrace(trace)
		if err != nil {
			return fmt.Errorf("writing a trace to the store: %w", err)
		}
		cli.Note("trace %s", tracePath)
	}

	return nil
}
