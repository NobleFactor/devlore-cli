// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package migrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/document"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"

	// Register the file provider, as the writ binary does via its inventory blank-imports.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
)

// TestExecutionTrace_SerializesAsMigrationReceipt illustrates the receipt path the migrate session uses (session.go).
//
// Build a graph through the public plan API, run it via a GraphExecutor, take the executor's
// op.Trace, and serialize it as the migration receipt via document.Write — confirming the Trace identifies its
// graph, reaches a terminal run state, and round-trips to a non-empty receipt file.
func TestExecutionTrace_SerializesAsMigrationReceipt(t *testing.T) {
	tmp := t.TempDir()

	environment, err := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithRoot(tmp).
		WithApplication(&application.Application{Name: "test"}))
	if err != nil {
		t.Fatalf("op.NewRuntimeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = environment.Close() })

	planProvider := plan.NewProvider(environment)

	invocation, err := planProvider.Plan(file.Mkdir, nil, map[string]any{
		"path": filepath.Join(tmp, "created"),
		"mode": os.FileMode(0o755),
		"user": "", "group": "",
	})
	if err != nil {
		t.Fatalf("Plan(file.mkdir): %v", err)
	}

	graph, err := planProvider.AssembleDefinition(
		[]*op.Invocation{invocation}, nil, nil, nil, nil, nil, planProvider.Origin("migrate"))
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	spec, err := planProvider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}

	executor := op.NewGraphExecutor(graph, spec)
	if _, runErr := executor.Run(context.Background(), nil); runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	// The migrate session serializes the executor's Trace as the receipt.
	trace := executor.Trace()
	if trace.GraphChecksum != graph.Checksum() {
		t.Errorf("trace.GraphChecksum = %q, want graph checksum %q", trace.GraphChecksum, graph.Checksum())
	}
	if trace.RunStatus.Phase != op.PhaseCompleted || trace.RunStatus.Condition != op.ConditionHealthy {
		t.Errorf("trace.RunStatus = %v, want completed × healthy", trace.RunStatus)
	}

	receiptPath := filepath.Join(tmp, ".writ-migrate-receipt.json")
	if err := document.Write(receiptPath, trace); err != nil {
		t.Fatalf("document.Write(receipt): %v", err)
	}

	info, err := os.Stat(receiptPath)
	if err != nil {
		t.Fatalf("receipt not written: %v", err)
	}
	if info.Size() == 0 {
		t.Error("receipt file is empty")
	}
}
