// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestNewRootCmd_NoUsageTextOnError pins the 2026-09-02 ruling for every program on the shared root: no
// usage text after an error -- not after a command that ran and failed, and not after a bad flag or an
// unknown subcommand either. Cobra's default prints the usage block in all three cases.
func TestNewRootCmd_NoUsageTextOnError(t *testing.T) {

	for _, args := range [][]string{
		{"fail"},
		{"fail", "--no-such-flag"},
		{"no-such-command"},
	} {
		root := NewRootCmd(RootConfig{Name: "probe", Short: "a probe"})
		root.AddCommand(&cobra.Command{
			Use:  "fail",
			RunE: func(*cobra.Command, []string) error { return errors.New("the answer is no") },
		})

		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Errorf("%v: expected an error", args)
		}
		if strings.Contains(out.String(), "Usage:") {
			t.Errorf("%v printed the usage block after its error:\n%s", args, out.String())
		}
	}
}
