// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package reconcile_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/reconcile"
)

// TestReport_EveryFormatRenders pushes one Report through every value of --output.
//
// The pipeline is shared and already proven elsewhere; what this pins is the Report's SHAPE under it: that
// State marshals as its label rather than its glyph, that the four sections survive stage-1 normalization,
// and that no format is silently rejected. The command-level wiring -- that `writ reconcile -o yaml`
// reaches this pipeline at all -- is asserted by the scenario suite, which is where the defect lived.
func TestReport_EveryFormatRenders(t *testing.T) {

	report := &reconcile.Report{
		Layers: []reconcile.Layer{{Name: "base", Path: "/layers/base", State: "directory"}},
		Entries: []reconcile.Entry{
			{Target: "/t/a", Source: "/s/a", Project: "p", Action: "file.link", State: reconcile.StateLinked},
			{Target: "/t/b", Source: "/s/b", Project: "p", Action: "file.copy", State: reconcile.StateStale, Repair: "writ upgrade"},
		},
		Health: reconcile.Health{Runs: 1},
	}

	for _, format := range []string{"json", "yaml", "table", "list", "csv", "value", "none", "template={{len .entries}}"} {
		t.Run(format, func(t *testing.T) {

			var buf bytes.Buffer
			pipeline, err := cli.BuildPipeline(cli.SinkOptions{Format: format}, &buf)
			if err != nil {
				t.Fatalf("BuildPipeline(%q): %v", format, err)
			}
			if err := pipeline.Emit(report); err != nil {
				t.Fatalf("Emit(%q): %v", format, err)
			}
			out := buf.String()

			switch format {
			case "none":
				if out != "" {
					t.Fatalf("-o none wrote %q; its contract is silence", out)
				}
			case "json":
				var v map[string]any
				if err := json.Unmarshal([]byte(out), &v); err != nil {
					t.Fatalf("not JSON: %v\n%s", err, out)
				}
				entries, _ := v["entries"].([]any)
				if len(entries) != 2 {
					t.Fatalf("entries = %d, want 2:\n%s", len(entries), out)
				}
				if first, _ := entries[0].(map[string]any); first["state"] != "linked" {
					t.Fatalf("state marshals as %v, want \"linked\" -- the label, not the glyph", first["state"])
				}
			case "template={{len .entries}}":
				if strings.TrimSpace(out) != "2" {
					t.Fatalf("template rendered %q, want 2", out)
				}
			default:
				if strings.TrimSpace(out) == "" {
					t.Fatalf("-o %s wrote nothing", format)
				}
			}
		})
	}
}
