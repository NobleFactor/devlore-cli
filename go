#!/usr/bin/env bash
set -euo pipefail

# --- noblefactor-ops: move changes to feature branch ---
cd /Users/david-noble/Workspace/NobleFactor/noblefactor-ops

# Create feature branch off develop (carries staged changes with it)
git checkout -b feature/codegen-doc-comment-funcs

# Stage only the source changes (star-bin and star/star are build artifacts)
git add internal/starlark/receiver_go_gen.go internal/starlark/receiver_go_gen_test.go

git commit -m "$(cat <<'EOF'
feat(codegen): docComment/docSummary template funcs, remove checksum from consumer model

- Add docComment: renders multi-line Go doc comments preserving Slots: sections
- Add docSummary: returns text before first blank line or structured section
- Remove ctx.TargetChecksum from consumer content model (checksums removed from Context)
- Update consumer return to pass result through instead of discarding
- Update tests for consumer and compensable consumer content models
EOF
)"

# --- devlore-cli ---
cd /Users/david-noble/Workspace/NobleFactor/devlore-cli.binding-unification
git add -A && git commit -m "$(cat <<'EOF'
feat(codegen): binding unification Phase 8 Part 28 — slot_docs in reference.yaml

- Add Slots: sections to all 35 provider methods across 9 providers
- Change predicates from bool to (bool, error): file.Exists, file.IsDir,
  pkg.Installed, pkg.NotInstalled, pkg.VersionGTE, service.Exists,
  service.Running, service.Enabled
- Change shell.Exec/PowerShell from error to (string, error)
- Update validate.star to detect MakeAttr calls (not just NewBuiltin)
- Regenerate all 18 generated files (9 plan_*_gen.go + 9 actions_gen.go)
- Add Receipt + Sidecar architecture analysis to execution-event.md
- Complete Reconciliation naming cleanup in execution-event.md
- Mark Part 28 done in phase-8.md
EOF
)"

# --- devlore-registry ---
cd /Users/david-noble/Workspace/NobleFactor/devlore-registry
git add -A && git commit -m "$(cat <<'EOF'
chore: regenerate reference.yaml with populated slot_docs

All plan.* provider entries now have slot_docs populated from
Slots: sections in provider doc comments. Only plan.choose and
plan.gather remain empty (flow actions, not generated providers).
EOF
)"
