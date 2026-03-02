// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Command devlore-test is the graph test harness for Starlark plan + execute + verify.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/NobleFactor/devlore-cli/internal/e2e/testrunner"
)

func main() {
	script := flag.String("script", "", "path to test .star script (required)")
	dryRun := flag.Bool("dry-run", false, "plan only, no side effects")
	trace := flag.Bool("trace", false, "enable Starlark step trace")
	provider := flag.String("provider", "", "restrict to a specific provider")
	flag.Parse()

	if *script == "" {
		fmt.Fprintln(os.Stderr, "error: --script is required")
		flag.Usage()
		os.Exit(2)
	}

	var opts []testrunner.Option
	if *dryRun {
		opts = append(opts, testrunner.WithDryRun())
	}
	if *trace {
		opts = append(opts, testrunner.WithTrace())
	}
	if *provider != "" {
		opts = append(opts, testrunner.WithProvider(*provider))
	}

	runner := testrunner.New(*script, opts...)
	result, err := runner.Start(context.Background())
	if err != nil {
		errResult := map[string]any{
			"passed":  false,
			"error":   err.Error(),
			"node_count": 0,
			"expectation_count": 0,
			"failures": []any{},
		}
		data, _ := json.Marshal(errResult)
		fmt.Println(string(data))
		os.Exit(2)
	}

	data, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling result: %v\n", err)
		os.Exit(2)
	}
	fmt.Println(string(data))

	if !result.Passed {
		os.Exit(1)
	}
}
