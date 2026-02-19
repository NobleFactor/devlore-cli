# Phase 7: Update Architecture and User-Facing Documentation

**Status**: Planning

## Summary

Update all architecture documents, user-facing guides, and authoring references
to reflect the new programming model from Phase 6.

## Architecture Documents (devlore-cli)

| File | Changes |
|------|---------|
| `devlore-phase-execution.md` | Update examples: phase-named entry, `plan` as global |
| `devlore-orchestration-primitives.md` | Document Choose predicates replacing system probes |
| `devlore-operation-namespaces.md` | Update namespace references |
| `devlore-typed-slots.md` | Update examples |

## User-Facing Guides (devlore-cli)

| File | Changes |
|------|---------|
| `docs/guides/lore/create-manifests.md` | Rewrite with new entry point, output functions, Choose predicates |
| `docs/guides/lore/pipeline.md` | Remove system binding references |
| `docs/package-hierarchy.md` | Update references |

## Authoring References (devlore-registry)

| File | Changes |
|------|---------|
| `AUTHORING.md` | Rewrite all examples |
| `knowledge/package-authoring/prompts/*.txt` | Update LLM prompts |
| `knowledge/package-authoring/bindings/reference.md` | Regenerate |
| `knowledge/package-authoring/bindings/reference.yaml` | Regenerate |

## Plan Documents

| File | Changes |
|------|---------|
| `docs/plans/phase-binding.md` | Mark as fully superseded |
