---
title: "Prose carries 110 British spellings: the US English ruling reaches this repository"
issue: https://github.com/NobleFactor/devlore-cli/issues/935
status: active
created: 2026-09-23
updated: 2026-09-23
---

# Plan: the US English ruling reaches this repository

## Issue 935

Phase 4 of noblefactor-ops#234, whose Phases 1-3 merged as noblefactor-ops#235. The full plan lives
there, in `docs/plans/fix/234-prose-is-not-checked-for.md`; this one carries what belongs to this
repository, and #234 stays `active` until these boxes close.

Ruled 2026-09-23: *"we're US … all words should be in US English."* No word is exempt.

## Current state

`misspell` enforces the rule inside Go source and nothing checks prose, so 110 British spellings
across 45 tracked files have drifted in. Counted by fetching codespell's own
`dictionary_en-GB_to_en-US.txt` — 535 entries — and matching its British side against every tracked
file, because codespell is not installed on this host and there is no Python to install it with.

26 distinct words. `licence` 29, `behaviour` 24, `cancelled` 9, `neighbours` 6, `analogue` 5,
`modelled` and `artefacts` 4 each, `acknowledgement` 3, then ones and twos.

## Requirements

### Requirement 1: Two stems that bite, and how

**`programme` is matched as a whole word, not a stem.** This tree holds `programming` (21),
`programmatic` (9), `programmer` (3) and `programmers` (2) — every one begins with `programme`, so a
stem replace produces `programr`. There is exactly one `Programme` to correct, and it is the only
pair here that needs boundaries.

**`catalogued` is corrected before the `catalogue` stem runs.** The stem alone turned it into
`catalogd` in noblefactor-ops. This tree holds only `catalogues`, so the ordering is insurance
rather than a fix, and it stays because the next sweep will not know that.

### Requirement 2: Text files only

A sweep of noblefactor-ops rewrote `docs/guides/examples/wasm-receiver/receivers/gitignore.wasm` and
shortened it by 98 bytes. codespell's `skip` already carried `*.wasm`; the `sed` loop had no
equivalent. Each file is tested with `grep -Iq .` first.

### Requirement 3: Every produced word is checked

Not every replaced word — every **produced** one. `catalogd` was a valid-looking substitution that
no British-spelling search would have found afterwards, because it is not British; it is not a word
at all.

**It caught a third stem here.** `cancell` → `cancel` is right for the two forms the dictionary asks
for, `cancelled` and `cancelling`, and wrong for everything else that begins that way: it turned 23
`cancellation` into `cancelation` and 2 `cancellable` into `cancelable`. **`cancellation` is correct
US English** — the dictionary says nothing about it precisely because there is nothing to say. Both
were restored, and the check that found them is a word-by-word comparison of the removed and added
lines rather than a re-run of the British-word search, which by construction cannot see a word that
is in neither language.

### Requirement 4: No prose gate is added here

It arrives with #932, which consolidates document checks into star. A bash gate now would be one more
thing for that lane to delete. This issue closes the drift; #932 closes the hole that let it in.

### Requirement 5: The legal lines are named

`CONTRIBUTING.md` and `TRADEMARK.md` hold three of the 32 `licence`, and they are the only lines in
either repository where the word carries legal weight. The other 29 are in
`docs/plans/licensing-readme-plan.md` and are the author's own prose — "assigns a licence to each
asset class" — which corrects cleanly. The pull request names the legal lines explicitly.

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, before the change

### Phase 2: The sweep

- [x] `programme` and `catalogued` handled as Requirement 1 describes
- [x] The pass over the 45 files that carry a hit, text-checked with `grep -Iq .` (Requirements 2, 3).
      Scoped to those files rather than all 1663: a pair-by-file loop over the whole tree spawned
      about 105,000 processes and had managed one file in two minutes
- [x] Every produced word inspected, as a count of each form on the removed and added lines. All 33
      balance. It caught `cancelation` and `cancelable`, which were restored
- [x] The dictionary re-run over all 1663 tracked files: zero hits outside this plan
- [x] This plan excluded from its own sweep, as noblefactor-ops#234's is from its gate

### Phase 3: Verify, then merge

- [ ] `make vet`, `make lint`, `make test`
- [ ] `gofmt -l` clean over any changed Go file
- [ ] The legal lines in `CONTRIBUTING.md` and `TRADEMARK.md` quoted in the pull request
- [ ] PR script written, analyzer-clean, shown, and handed over
- [ ] CI runs on the pull request and the merge gate blocks until every check reports pass

### Phase 4: Close noblefactor-ops#234

- [ ] #234's Phase 4 boxes ticked and its plan set `complete`, in the next noblefactor-ops pull
      request through — it could not close in #235, which carried its Phases 1-3 in another repository

## Related Documents

- [noblefactor-ops#234](https://github.com/NobleFactor/noblefactor-ops/issues/234) — the gap, the ruling, and Phases 1-3
- [noblefactor-ops documentation-standards.md](https://github.com/NobleFactor/noblefactor-ops/blob/develop/docs/documentation-standards.md) — where the rule is written
- [#932](https://github.com/NobleFactor/devlore-cli/issues/932) — where this repository's prose gate comes from
