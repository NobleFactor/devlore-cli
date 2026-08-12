// SPDX-License-Identifier: Apache-2.0
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
func assignSlots(slots, items []string) (matched []Match, unmatchedSlots, unmatchedItems []string) {
	n := len(slots)
	m := len(items)

	if n == 0 && m == 0 {
		return nil, nil, nil
	}

	scores := buildScoreMatrix(slots, items)

	slotTaken := make([]bool, n)
	itemTaken := make([]bool, m)

	// Greedy rounds: pick best pair above threshold.
	for round := 0; round < min2(n, m); round++ {
		bestI, bestJ, bestScore := bestFreePair(scores, slotTaken, itemTaken)

		if bestScore < 0.3 {
			break
		}

		matched = append(matched, Match{
			Slot:  slots[bestJ], //nolint:gosec // G602: reaching this append requires bestScore >= 0.3, which only a scored (i, j) pair sets
			Item:  items[bestI], //nolint:gosec // G602: reaching this append requires bestScore >= 0.3, which only a scored (i, j) pair sets
			Score: bestScore,
		})
		slotTaken[bestJ] = true
		itemTaken[bestI] = true
	}

	// Forced assignment: one unmatched slot + one unmatched item.
	if countFree(slotTaken) == 1 && countFree(itemTaken) == 1 {
		matched = append(matched, forcedMatch(slots, items, slotTaken, itemTaken))
	}

	return matched, freeStrings(slots, slotTaken), freeStrings(items, itemTaken)
}

// buildScoreMatrix scores every item against every slot via the normalized fuzzy comparison.
//
// Parameters:
//   - `slots`: the slot names.
//   - `items`: the item names.
//
// Returns:
//   - `[][]float64`: scores indexed [item][slot].
func buildScoreMatrix(slots, items []string) [][]float64 {

	scores := make([][]float64, len(items))
	for i, item := range items {
		scores[i] = make([]float64, len(slots))
		normItem := normalize(firstToken(item))
		for j, slot := range slots {
			scores[i][j] = fuzzyScore(normItem, normalize(slot))
		}
	}

	return scores
}

// bestFreePair finds the highest-scoring pair among the untaken items and slots.
//
// Parameters:
//   - `scores`: the score matrix, indexed [item][slot].
//   - `slotTaken`: the taken flags per slot.
//   - `itemTaken`: the taken flags per item.
//
// Returns:
//   - `bestI`: the best item index, or -1 when no free pair exists.
//   - `bestJ`: the best slot index, or -1 when no free pair exists.
//   - `bestScore`: the pair's score; 0.0 when no free pair beats it.
func bestFreePair(scores [][]float64, slotTaken, itemTaken []bool) (bestI, bestJ int, bestScore float64) {

	bestI, bestJ = -1, -1
	for i := range itemTaken {
		if itemTaken[i] {
			continue
		}
		for j := range slotTaken {
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

	return bestI, bestJ, bestScore
}

// forcedMatch pairs the single remaining free slot with the single remaining free item, marking both
// taken.
//
// Parameters:
//   - `slots`: the slot names.
//   - `items`: the item names.
//   - `slotTaken`: the taken flags per slot; exactly one must be free.
//   - `itemTaken`: the taken flags per item; exactly one must be free.
//
// Returns:
//   - `Match`: the forced pair, scored 0.1.
func forcedMatch(slots, items []string, slotTaken, itemTaken []bool) Match {

	match := Match{Score: 0.1, Forced: true}
	for j := range slotTaken {
		if !slotTaken[j] {
			match.Slot = slots[j]
			slotTaken[j] = true
			break
		}
	}
	for i := range itemTaken {
		if !itemTaken[i] {
			match.Item = items[i]
			itemTaken[i] = true
			break
		}
	}

	return match
}

// freeStrings collects the entries whose taken flag is unset, in order.
//
// Parameters:
//   - `list`: the entries.
//   - `taken`: the taken flags, parallel to `list`.
//
// Returns:
//   - `[]string`: the untaken entries.
func freeStrings(list []string, taken []bool) []string {

	var free []string
	for i := range list {
		if !taken[i] {
			free = append(free, list[i])
		}
	}

	return free
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
