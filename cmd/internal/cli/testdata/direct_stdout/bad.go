// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package bad is a fixture: every way a command package can reach stdout without the sink, and the two
// reads of os.Stdout that are not writes. NoDirectStdout must report the six and not the two.
package bad

import (
	"fmt"
	"os"
	"os/exec"
)

func writes() {
	fmt.Println("a result")                  // 1
	fmt.Printf("%d\n", 1)                    // 2
	fmt.Fprintln(os.Stdout, "a result")      // 3
	_, _ = os.Stdout.WriteString("a result") // 4
	println("a result")                      // 5

	child := exec.Command("vi")
	child.Stdout = os.Stdout // 6: the handoff, outside RunInteractive
	_ = child

	_ = os.Stdout.Fd()                   // a read: not reported
	_, _ = os.Stdout.Stat()              // a read: not reported
	fmt.Fprintln(os.Stderr, "narration") // stderr: not reported
}
