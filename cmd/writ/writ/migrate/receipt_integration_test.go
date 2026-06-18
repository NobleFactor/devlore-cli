// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/document"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"

	// Register the file provider, as the writ binary does via its inventory blank-imports.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
)

// TestExecutionTrace_SerializesAsMigrationReceipt illustrates the receipt path the migrate session uses
// (session.go): build a graph through the public plan API, run it via a GraphExecutor, take the executor's
// op.Trace, and serialize it as the migration receipt via document.Write — confirming the Trace identifies its
// graph, reaches a terminal run state, and round-trips to a non-empty receipt file.
func TestExecutionTrace_SerializesAsMigrationReceipt(t *testing.T) {
	tmp := t.TempDir()

	root, err := fsroot.OpenConfined(tmp)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	env := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithRoot(root).
		WithApplication(&application.Application{Name: "test"}))

	planProvider := plan.NewProvider(env)

	invocation, err := planProvider.Plan("file.mkdir", nil, map[string]any{
		"path":  filepath.Join(tmp, "created"),
		"chmod": os.FileMode(0o755),
		"chown": "",
	})
	if err != nil {
		t.Fatalf("Plan(file.mkdir): %v", err)
	}

	graph, err := planProvider.Assemble([]*op.Invocation{invocation}, nil, nil, nil, planProvider.Origin("migrate"))
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
	if trace.State != op.RunStateCompleted {
		t.Errorf("trace.State = %v, want RunStateCompleted", trace.State)
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
