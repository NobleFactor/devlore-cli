# Phase 7: Codegen

**Status**: Done
**PR**: pending

## Summary

Remove the `+devlore:callable` annotation system from the code generator.
With full-signature callable matching (Phase 5) and the annotation removed
from `Reducer` (Phase 6), the codegen no longer needs special callable
handling. Func-typed params flow through as regular params. The runtime
reflection layer handles callable adaptation via `buildCallableFunc`.

## Changes

### generate.star — removed dead callable code

Removed 5 functions and their call sites:
- `parse_callable_directive(doc)` — parsed `+devlore:callable swallow=stack`
- `CALLABLE_TYPE_MAPPINGS` — type list used by callable classification
- `classify_callable_params(callable_info, directive)` — role classification
  (swallowed, pass_through, projected, handle)
- `resolve_callable_params(path, descriptors, type_prefix)` — scanned for
  `+devlore:callable` annotations and annotated params
- `filter_callable_methods(methods, template_name)` — excluded methods with
  callable params from planned/action templates

Updated:
- `compute_param_names_list()` — no longer skips callable params
- `prepare_render_data()` — no longer filters/flags callable methods;
  return signature simplified (no more `flagged` list)
- `gen_file()` — removed flagged/unprojectable warning loop

### docs/guides/provider-development.md

Removed `+devlore:callable` from the directives table.

## Verification

After removing the annotation system, `make generate` produces correct
output for all providers. Func-typed params like `fn` (Reducer) appear
as regular params in `params.gen.go`. All generated tests pass.
