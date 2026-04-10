# Test Breaks — refactor/extract-starlark-from-op

All code compiles. These are runtime failures only. 16 failing packages remain.

## 1. Phase 9: String-to-resource coercion in starlark (8 TestStarlark + 2 bind)

**Root cause:** `executingReceiver.coerceResource` (`pkg/op/bind/executing_receiver.go:386`)
uses a nil package-level `registry` global instead of the context's registry. When starlark
passes a string where `*Resource` is expected, coercion silently fails and falls through to
`unmarshalStruct` which rejects the string.

**Fix:** Phase 9 work — fix `coerceResource` to get the registry from
`r.receiver.(op.Provider).ExecutionContext().Registry`, implement catalog-first lookup.

**Affected packages (8 TestStarlark):**
- `pkg/op/provider/appnet` — TestStarlark
- `pkg/op/provider/archive` — TestStarlark
- `pkg/op/provider/encryption` — TestStarlark
- `pkg/op/provider/file` — TestStarlark
- `pkg/op/provider/git` — TestStarlark
- `pkg/op/provider/pkg` — TestStarlark
- `pkg/op/provider/service` — TestStarlark (not listed separately — same pattern)
- `pkg/op/provider/platform` — TestStarlark (properties return methods instead of values)

**Affected (bind unmarshal):**
- `pkg/op/bind` — TestUnmarshal_WithConstructor, TestUnmarshal_Constructor_InvalidInput

## 2. Phase 9: FillSlot resource passthrough (2 tests in pkg/op/bind)

**Root cause:** `FillSlot` loses resource identity through marshal→unmarshal round-trip.
Needs `*Value` extraction before unmarshal.

- `pkg/op/bind` — TestFillSlotImplicitEdge_PlainResource
- `pkg/op/bind` — TestFillSlotImplicitEdge_ResourceWithOrigin

## 3. Appnet URI canonicalization (2 tests + 13 subtests in pkg/op/provider/appnet)

**Root cause:** Tests expect scheme-stripped URIs (`appnet:example.com/path`) but
`NewResource` stores the full URL (`appnet:https://example.com/path`). Design changed — URI
includes the scheme for identity.

**Fix:** Update test expectations to match the new URI format, or decide whether
transport-independence is still a goal and fix `NewResource` accordingly.

- `pkg/op/provider/appnet` — TestURITransportIndependent, TestURICanonicalization

## 4. Dependent type method parameter names (1 test in cmd/star/provider/starcode)

**Root cause:** `Sources.stats` method call fails with `unexpected keyword argument
"with_bytes"`. The `Sources` dependent type is marshaled via `bind.Marshal` → `NewValue`
→ `executingReceiver`. The receiver type for `Sources` is derived via reflection
(`resolveReceiverType`) which generates positional parameter names (`p0`, `p1`), not
the actual names (`with_bytes`, `with_loc`). The starlark script passes keyword args
by name, which don't match.

**Fix:** The dependent type codegen (`dependent_type.gen.go.template`) needs to register
method parameter names, or `resolveReceiverType` needs to derive names from the Go method
signatures.

- `cmd/star/provider/starcode` — TestIntegrationEndToEnd

## 5. Graph/executor state (8 tests, 2 packages)

**`cmd/lore/lore/builder_test.go`** — 5 failures:
- TestEngineRunsPackageInstallActions — `graph already executed (state: )` — needs
  `State: op.StatePending`
- TestEngineRunsNamespacedPackageActions (4 subtests) — same
- TestBuildPhased_LorePackageForwardOnly — subgraph has no children (builder issue)
- TestBuildPhased_LorePackageMultiPhase — install/provision subgraphs have no children
- TestBuildPhased_PlanIsGlobal — expected pkg.install node from plan

**`pkg/op/graph_test.go`** — 3 failures:
- TestNewGraph_InitializesCatalog — `NewGraph().Catalog is nil`
- TestGraph_CatalogNotSerialized — panic
- TestGraph_Summary — `ByAction[file.link].Total() = 5, want 4`

## 6. cmd/star/star starlark integration (9 tests)

**Root cause:** Starlark scripts can't resolve provider sub-attributes. Lint copyright
tests fail with `builtin_function_or_method has no .lint field`. Source file test fails
with starlark traceback.

- `cmd/star/star` — 8 TestLintCopyright_*, TestSourceFile_StarlarkIntegration

## 7. cmd/devlore-test (6 tests, 2 packages)

- `cmd/devlore-test` — TestCLI_SummaryOnly, TestCLI_ReceiptOnly*, TestCLI_RoutToFiles
- `cmd/devlore-test/devloretest` — TestCompensation, TestMkdirAndRemoveAll

## 8. File provider action dispatch (1 test)

- `pkg/op/provider/file` — TestActions_Join — returns `""`, want `"a/b/c.txt"`

## 9. Config schema (1 test)

- `cmd/star/provider/goast` — TestConfigSchemas_ProviderPicksUpConfig

## 10. Plan gen panic (1 test)

- `pkg/op/provider/plan/gen` — TestModule_Attr_Unknown — nil pointer in ResolveAttr

## Summary by count

| Root cause | Failures | Packages |
|------------|----------|----------|
| Phase 9 coercion | 10 | 9 |
| Phase 9 FillSlot | 2 | 1 |
| Appnet URI format | 2+13 sub | 1 |
| Dependent type params | 1 | 1 |
| Graph/executor state | 8 | 2 |
| cmd/star/star starlark | 9 | 1 |
| cmd/devlore-test | 6 | 2 |
| File TestActions_Join | 1 | 1 |
| Config schema | 1 | 1 |
| Plan gen panic | 1 | 1 |
| **Total** | **~54** | **16** |

## Disabled test files (need full rewrite)

- `cmd/lore/lore/runtime_test.go` — old starlark runtime API
- `cmd/lore/lore/integration_test.go` — old starlark runtime API
- `internal/execution/stateview_test.go` — old Graph structure, garbled content
- `pkg/op/executor_test.go` — pre-existing disable
