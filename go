git add -A \
&& git commit -m "$(cat <<'EOF'
feat(writ): binding unification Phase 8 Part 26 — file.Provider for filesystem ops

Replace direct os.Rename/os.Symlink/os.MkdirAll calls with file.Provider
methods in four functions: adoptFile, linkToLayer, moveToLayer, and
migrate.Execute. Every filesystem mutation now routes through the provider,
gaining compensation receipts and honoring the single-implementation
guarantee.

Also fix all pre-existing lint issues in the three modified files:
- errorlint: %v → %w for error wrapping
- unparam: remove unused parameter and always-nil error return
- misspell: cancelled → canceled
- staticcheck: remove unused append result
- gocognit: decompose 7 high-complexity functions into focused helpers
  (classifyDrift, classifyCopiedEntry, classifySymlinkEntry,
  clearExistingLayer, buildUpgradeChain, detectUpgradeDrift, etc.)
- unlambda: os.Getenv closure → direct reference
- nilerr: distinguish IsNotExist from other Lstat errors
EOF
)"
