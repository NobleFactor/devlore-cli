// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package lore

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/lore/lore/onboard"
)

// TestOnboardResult_EmitsThroughThePipeline pins the shape `lore onboard` now emits. Until this phase the
// Result was narrated to stderr and stdout was silent; the file write is the side effect and the Result
// is what -o renders. The wiring -- runOnboard handing the result to emitResult -- is two lines read by
// eye; what this asserts is that the value renders as the discovery the user was told about, under the
// names the JSON carries, and that -o none leaves stdout empty.
func TestOnboardResult_EmitsThroughThePipeline(t *testing.T) {

	result := &onboard.Result{
		Product:  &onboard.ProductInfo{Name: "docker", Category: "container-runtime", Vendor: "Docker Inc."},
		Slots:    []onboard.ExtractedSlot{{}},
		Manifest: "packages:\n  - docker\n",
	}

	var buf bytes.Buffer
	pipeline, err := cli.BuildPipeline(cli.SinkOptions{Format: "json"}, &buf)
	if err != nil {
		t.Fatalf("BuildPipeline(json): %v", err)
	}
	if err := pipeline.Emit(result); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	var v map[string]any
	if err := json.Unmarshal(buf.Bytes(), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	product, _ := v["product"].(map[string]any)
	if product["name"] != "docker" || product["vendor"] != "Docker Inc." {
		t.Fatalf("product did not survive: %v", v["product"])
	}
	if slots, _ := v["slots"].([]any); len(slots) != 1 {
		t.Fatalf("slots = %v, want one", v["slots"])
	}
	if v["manifest"] != result.Manifest {
		t.Fatalf("manifest = %q, want %q", v["manifest"], result.Manifest)
	}

	buf.Reset()
	pipeline, err = cli.BuildPipeline(cli.SinkOptions{Format: "none"}, &buf)
	if err != nil {
		t.Fatalf("BuildPipeline(none): %v", err)
	}
	if err := pipeline.Emit(result); err != nil {
		t.Fatalf("Emit(none): %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("-o none wrote %q; the file is the only output then", buf.String())
	}
}
