// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package devloretest

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
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

// stdoutSentinel names the process's standard output in --output routing. [openDest] intercepts
// it instead of opening it, so the name need not exist as a filesystem path — which it does not on
// Windows. The discard destination needs no sentinel: os.DevNull already names the platform's own
// device (/dev/null, or NUL on Windows) and opens correctly on each.
const stdoutSentinel = "/dev/stdout"

func newRunCmd() *cobra.Command {
	outputs := &outputFlags{entries: map[string]string{}}

	cmd := &cobra.Command{
		Use:   "run [flags] <script.star>",
		Short: "Run a Starlark test script that plans and executes a graph",
		Long: fmt.Sprintf(`Run a Starlark test script through the graph execution engine.

The script uses plan.* bindings to build a graph and t.* assertions to
verify expectations after execution.

Three output streams route independently; each defaults to an artifact file
named for the script, in the working directory:
  summary  JSON test result (passed, node_count, failures)   <script>.summary.json
  graph    The graph document from the software under test   <script>.graph.yaml
  receipt  Full serialized execution graph                    <script>.receipt.<format>

Reroute any stream to a path, %[1]s, or %[2]s:
  devlore-test run --output summary=%[1]s test.star
  devlore-test run --output graph=%[2]s test.star`, stdoutSentinel, os.DevNull),
		Example: fmt.Sprintf(`  devlore-test run test.star
  devlore-test run --dry-run test.star
  devlore-test run --trace test.star
  devlore-test run --output receipt=my-receipt.json --receipt-format=json test.star
  devlore-test run --output graph=%[1]s --output receipt=%[1]s test.star`, os.DevNull),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTest(cmd, args[0], outputs)
		},
	}

	cmd.Flags().Bool("dry-run", false, "Plan only, no side effects")
	cmd.Flags().Bool("trace", false, "Enable Starlark step trace")
	cmd.Flags().String("provider", "", "Restrict to a specific provider")
	cmd.Flags().String("receipt-format", "yaml", "Receipt format: json or yaml")
	cmd.Flags().Var(outputs, "output",
		"Stream routing: summary|receipt|graph=destination (repeatable; default: <script>.summary.json, "+
			"<script>.graph.yaml, <script>.receipt.<format> beside the working directory)")

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

	// The script is verified before any output destination opens: default routing names artifacts after the
	// script, and a run that cannot start must not litter the working directory with files named for a
	// script that does not exist.
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("reading script: %w", err)
	}

	// Default routing: three artifact files named for the script, in the working directory — results are
	// files, narration is stderr, and stdout stays clean (ruled 2026-08-20). An explicit --output — a path,
	// os.DevNull, or /dev/stdout — overrides per stream.
	base := strings.TrimSuffix(filepath.Base(script), filepath.Ext(script))
	for stream, dest := range map[string]string{
		"summary": base + ".summary.json",
		"graph":   base + ".graph.yaml",
		"receipt": base + ".receipt." + receiptFmt,
	} {
		if _, set := outputs.entries[stream]; !set {
			outputs.entries[stream] = dest
		}
	}
	cli.Note("summary=%s graph=%s receipt=%s",
		outputs.entries["summary"], outputs.entries["graph"], outputs.entries["receipt"])

	// Open graph output destination for streaming during execution.
	graphOut, err := openDest(outputs.entries["graph"])
	if err != nil {
		return fmt.Errorf("opening graph output: %w", err)
	}
	defer iox.Close(&err, graphOut)

	// Build and run.
	opts := []Option{WithWriter(graphOut)}
	if dryRun {
		opts = append(opts, WithDryRun())
	}
	if trace {
		opts = append(opts, WithTrace())
	}
	if provider != "" {
		opts = append(opts, WithProvider(provider))
	}

	runner := NewRunner(script, opts...)
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

// openDest opens a destination for writing.
//
// [stdoutSentinel] is intercepted rather than opened: it is not a filesystem path on Windows, and
// the process's standard output must survive the caller's Close. Every other destination is a real
// path — including the discard device, which os.DevNull names correctly on each platform.
//
// Parameters:
//   - `path`: the destination, either [stdoutSentinel] or a filesystem path.
//
// Returns:
//   - `io.WriteCloser`: the destination; the caller must close it, which is a no-op for standard
//     output.
//   - `error`: non-nil when a filesystem destination cannot be opened.
func openDest(path string) (io.WriteCloser, error) {

	if path == stdoutSentinel {
		return nopWriteCloser{os.Stdout}, nil
	}

	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // G304: path from CLI flag
}

// nopWriteCloser adapts a writer whose lifetime the caller does not own, so a deferred Close does
// not shut the process's standard output.
type nopWriteCloser struct{ io.Writer }

// Close discards the request, leaving the underlying writer open.
//
// Returns:
//   - `error`: always nil.
func (nopWriteCloser) Close() error { return nil }

func writeSummary(dest string, result *Result) (err error) {
	f, err := openDest(dest)
	if err != nil {
		return err
	}
	defer iox.Close(&err, f)

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshaling: %w", err)
	}
	//nolint:gosec // G705: JSON text output; no HTML sink.
	_, err = fmt.Fprintln(f, string(data))
	return err
}

func writeReceipt(dest, format string, graph *op.Graph) (err error) {

	// The destination is opened before the nil check so every run leaves all three artifacts — a script that
	// assembles no graph writes an empty receipt rather than no file (ruled 2026-08-20: three files, always).
	f, err := openDest(dest)
	if err != nil {
		return err
	}
	defer iox.Close(&err, f)

	if graph == nil {
		return nil
	}

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
