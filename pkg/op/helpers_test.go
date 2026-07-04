// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "testing"

func TestTopologicallySorted_ProducerBeforeConsumer(t *testing.T) {

	a := makeNode("a", "test.a", nil, nil)
	b := makeNode("b", "test.b", nil, nil)
	c := makeNode("c", "test.c", nil, nil)

	// Input order is anti-topological; the edges demand a → b → c.
	sorted := topologicallySorted(
		[]ExecutableUnit{c, b, a},
		[]Edge{{From: "a", To: "b"}, {From: "b", To: "c"}},
	)

	if len(sorted) != 3 {
		t.Fatalf("sorted length = %d, want 3", len(sorted))
	}

	position := make(map[string]int, len(sorted))
	for i, unit := range sorted {
		position[unit.ID()] = i
	}

	if !(position["a"] < position["b"] && position["b"] < position["c"]) {
		t.Errorf("order %v does not place producers before consumers (want a before b before c)", position)
	}
}
