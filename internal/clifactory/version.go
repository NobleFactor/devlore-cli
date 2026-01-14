// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Noble Factor. All rights reserved.

package clifactory

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// VersionInfo contains version metadata set at build time.
type VersionInfo struct {
	Version   string // Semantic version (e.g., "0.1.0")
	Commit    string // Git commit hash
	BuildDate string // Build timestamp
}

// NewVersionCmd creates the version command.
func NewVersionCmd(info VersionInfo) *cobra.Command {
	var short bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			if short {
				fmt.Println(info.Version)
				return
			}
			fmt.Printf("Version:    %s\n", info.Version)
			fmt.Printf("Commit:     %s\n", info.Commit)
			fmt.Printf("Built:      %s\n", info.BuildDate)
			fmt.Printf("Go version: %s\n", runtime.Version())
			fmt.Printf("OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}

	cmd.Flags().BoolVarP(&short, "short", "s", false, "Print only the version number")

	return cmd
}
