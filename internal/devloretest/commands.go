// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devloretest

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/e2e/testrunner"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	filegen "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
)

// outputFlags collects repeated --output key=dest flags.
type outputFlags struct {
	entries map[string]string
}

func (o *outputFlags) String() string {
	var parts []string
	for k, v := range o.entries {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func (o *outputFlags) Set(val string) error {
	k, v, ok := strings.Cut(val, "=")
	if !ok {
		return fmt.Errorf("expected key=destination, got %q", val)
	}
	switch k {
	case "summary", "receipt", "graph":
		o.entries[k] = v
		return nil
	default:
		return fmt.Errorf("unknown output stream %q (valid: summary, receipt, graph)", k)
	}
}

func (o *outputFlags) Type() string {
	return "stream=dest"
}

func newRunCmd() *cobra.Command {
	outputs := &outputFlags{entries: map[string]string{
		"summary": "/dev/stdout",
		"receipt": "/dev/stdout",
		"graph":   "/dev/stdout",
	}}

	cmd := &cobra.Command{
		Use:   "run [flags] <script.star>",
		Short: "Run a Starlark test script that plans and executes a graph",
		Long: `Run a Starlark test script through the graph execution engine.

The script uses plan.* bindings to build a graph and t.* assertions to
verify expectations after execution.

Three output streams can be independently routed:
  graph    Output from the software under test
  summary  JSON test result (passed, node_count, failures)
  receipt  Full serialized execution graph

All streams default to /dev/stdout. Route to files or /dev/null:
  devlore-test run --output receipt=receipt.yaml test.star
  devlore-test run --output graph=/dev/null test.star`,
		Example: `  devlore-test run test.star
  devlore-test run --dry-run test.star
  devlore-test run --trace test.star
  devlore-test run --output receipt=receipt.yaml --receipt-format=json test.star
  devlore-test run --output graph=/dev/null --output receipt=/dev/null test.star`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTest(cmd, args[0], outputs)
		},
	}

	cmd.Flags().Bool("dry-run", false, "Plan only, no side effects")
	cmd.Flags().Bool("trace", false, "Enable Starlark step trace")
	cmd.Flags().String("provider", "", "Restrict to a specific provider")
	cmd.Flags().String("receipt-format", "yaml", "Receipt format: json or yaml")
	cmd.Flags().Var(outputs, "output", "Output routing: summary|receipt|graph=destination (repeatable)")

	return cmd
}

func runTest(cmd *cobra.Command, script string, outputs *outputFlags) (err error) {
	dryRun, _ := cmd.Flags().GetBool("dry-run")              //nolint:errcheck // flag registered above
	trace, _ := cmd.Flags().GetBool("trace")                 //nolint:errcheck // flag registered above
	provider, _ := cmd.Flags().GetString("provider")         //nolint:errcheck // flag registered above
	receiptFmt, _ := cmd.Flags().GetString("receipt-format") //nolint:errcheck // flag registered above

	if receiptFmt != "json" && receiptFmt != "yaml" {
		return fmt.Errorf("--receipt-format must be json or yaml, got %q", receiptFmt)
	}

	// Open graph output destination for streaming during execution.
	graphOut, err := openDest(outputs.entries["graph"])
	if err != nil {
		return fmt.Errorf("opening graph output: %w", err)
	}
	defer iox.Close(&err, graphOut)

	// Build and run.
	var opts []testrunner.Option
	opts = append(opts, testrunner.WithWriter(graphOut))
	opts = append(opts, testrunner.WithGraphBuilder(), testrunner.WithReceivers(filegen.Receiver))
	if dryRun {
		opts = append(opts, testrunner.WithDryRun())
	}
	if trace {
		opts = append(opts, testrunner.WithTrace())
	}
	if provider != "" {
		opts = append(opts, testrunner.WithProvider(provider))
	}

	runner := testrunner.New(script, opts...)
	result, err := runner.Start(cmd.Context())
	if err != nil {
		return err
	}

	// Write summary.
	if err := writeSummary(outputs.entries["summary"], result); err != nil {
		return fmt.Errorf("writing summary: %w", err)
	}

	// Write receipt.
	if err := writeReceipt(outputs.entries["receipt"], receiptFmt, runner.Graph()); err != nil {
		return fmt.Errorf("writing receipt: %w", err)
	}

	if !result.Passed {
		cli.Error("test failed")
		return cli.Failure("%d expectation(s) failed", len(result.Failures))
	}

	return nil
}

// openDest opens a destination path for writing. The caller must close the returned file.
func openDest(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // G304: path from CLI flag
}

func writeSummary(dest string, result *testrunner.Result) (err error) {
	f, err := openDest(dest)
	if err != nil {
		return err
	}
	defer iox.Close(&err, f)

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshaling: %w", err)
	}
	_, err = fmt.Fprintln(f, string(data))
	return err
}

func writeReceipt(dest string, format string, graph *op.Graph) (err error) {
	if graph == nil {
		return nil
	}

	f, err := openDest(dest)
	if err != nil {
		return err
	}
	defer iox.Close(&err, f)

	switch format {
	case "json":
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		return graph.Serialize(enc)
	default:
		enc := yaml.NewEncoder(f)
		enc.SetIndent(2)
		defer iox.Close(&err, enc)
		return graph.Serialize(enc)
	}
}
