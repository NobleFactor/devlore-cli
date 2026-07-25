---
step: 3
title: "Reserved kwarg names refused at registration — options/args/kwargs can't be plain params; *args/**kwargs stay valid"
former_title: "Reserved-kwarg enforcement at method registration"
status: complete
proof_run: 2026-06-15
parent: ../../phase-8.md
---

# Step 3 — Reserved kwarg names refused at registration

**Status:** `complete` · **3 / 3 contract tests written and passing** · stated deliverable fully proven.

## What this step delivers

Provider authors are prevented from declaring a method parameter that collides with the dispatch contract's reserved
names. `newReceiverType` (`pkg/op/receiver_type.go:347`) returns an error at registration when a method declares
`options`, `args`, or `kwargs` as a plain name (including the `?`-optional and `*`-decorated forms), while the
legitimate variadic markers `*args` / `**kwargs` pass. A whole class of provider-definition mistakes is caught at boot,
as an error that names the provider, the method, and the offending parameter — rather than surfacing later as a
confusing dispatch failure.

The per-parameter message comes from `reservedParameterError` (`receiver_type.go:449`); the call site wraps it with
`provider <name> method <m>:` (`:357`), so the full error names all three.

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail. Files: all in `pkg/op/receiver_type_test.go`.

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| 1 | `TestNewReceiverType_RejectsReservedParameterNames` (`:215`) | `options` / `options?` / `*options` / `args` / `kwargs` are rejected | ☑ | ✅ |
| 2 | `TestNewReceiverType_AllowsVariadicMarkers` (`:242`) | `*args` / `**kwargs` pass | ☑ | ✅ |
| 3 | `TestNewReceiverType_RejectsReservedParameterNames_NamesProviderMethodParam` | the error **names the provider, method, and offending parameter** | ☑ | ✅ |

**Coverage: 3 / 3 — the stated deliverable is fully proven.**

## Proof run

```
$ go test ./pkg/op/ -run 'NewReceiverType_(RejectsReserved|AllowsVariadic)' -v -count=1
--- PASS: TestNewReceiverType_RejectsReservedParameterNames                       (options, options?, *options, args, kwargs)
--- PASS: TestNewReceiverType_RejectsReservedParameterNames_NamesProviderMethodParam
--- PASS: TestNewReceiverType_AllowsVariadicMarkers                               (*args, **kwargs)
ok  github.com/NobleFactor/devlore-cli/pkg/op
```

## Optional hardening (beyond the stated contract)

`TestNewReceiverType_RejectsReservedNameEdgeForms` — boundary forms not in the stated contract (`args?`, `kwargs?`,
`**args`, `*kwargs`). Not required for `complete`; a robustness add if the reserved-name rule is later tightened.
