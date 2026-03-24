// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

// Match represents a slot-to-item assignment produced by assignSlots.
type Match struct {
	Slot   string
	Item   string
	Score  float64
	Forced bool // assigned by elimination, not by score
}

// assignSlots finds the best assignment of list items to named slots.
// Greedy: picks the highest-scoring pair, assigns it, removes both, repeats.
// Forced assignment when exactly one slot and one item remain.
//
// Returns matched pairs, unmatched slots, and unmatched items.
func assignSlots(slots []string, items []string) (matched []Match, unmatchedSlots []string, unmatchedItems []string) {
	n := len(slots)
	m := len(items)

	if n == 0 && m == 0 {
		return nil, nil, nil
	}

	// Build score matrix.
	scores := make([][]float64, m)
	for i, item := range items {
		scores[i] = make([]float64, n)
		normItem := normalize(firstToken(item))
		for j, slot := range slots {
			normSlot := normalize(slot)
			scores[i][j] = fuzzyScore(normItem, normSlot)
		}
	}

	slotTaken := make([]bool, n)
	itemTaken := make([]bool, m)

	// Greedy rounds: pick best pair above threshold.
	for round := 0; round < min2(n, m); round++ {
		bestScore := 0.0
		bestI, bestJ := -1, -1

		for i := 0; i < m; i++ {
			if itemTaken[i] {
				continue
			}
			for j := 0; j < n; j++ {
				if slotTaken[j] {
					continue
				}
				if scores[i][j] > bestScore {
					bestScore = scores[i][j]
					bestI = i
					bestJ = j
				}
			}
		}

		if bestScore < 0.3 {
			break
		}

		matched = append(matched, Match{
			Slot:  slots[bestJ],
			Item:  items[bestI],
			Score: bestScore,
		})
		slotTaken[bestJ] = true
		itemTaken[bestI] = true
	}

	// Forced assignment: one unmatched slot + one unmatched item.
	freeSlots := countFree(slotTaken)
	freeItems := countFree(itemTaken)
	if freeSlots == 1 && freeItems == 1 {
		for j := 0; j < n; j++ {
			if slotTaken[j] {
				continue
			}
			for i := 0; i < m; i++ {
				if itemTaken[i] {
					continue
				}
				matched = append(matched, Match{
					Slot:   slots[j],
					Item:   items[i],
					Score:  0.1,
					Forced: true,
				})
				slotTaken[j] = true
				itemTaken[i] = true
			}
		}
	}

	// Collect unmatched.
	for j := 0; j < n; j++ {
		if !slotTaken[j] {
			unmatchedSlots = append(unmatchedSlots, slots[j])
		}
	}
	for i := 0; i < m; i++ {
		if !itemTaken[i] {
			unmatchedItems = append(unmatchedItems, items[i])
		}
	}

	return
}

func countFree(taken []bool) int {
	n := 0
	for _, t := range taken {
		if !t {
			n++
		}
	}
	return n
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
