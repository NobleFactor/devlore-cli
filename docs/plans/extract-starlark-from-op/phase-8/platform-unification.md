---
title: "Platform Unification — one serializable op.Platform, pkg/platform as the implementation layer"
status: draft
created: 2026-06-04
updated: 2026-06-04
---

# Plan: Unify on `op.Platform`

## Context

Two parallel Platform stacks duplicate the package-manager / service-manager implementations
(apt/dnf/pacman/brew/…, plus the `Search` / `ParsePURL` parsing) verbatim:

- **`pkg/op/platform*.go`** — `op.Platform` (a serializable **struct**), the `op.PackageManager` /
  `op.ServiceManager` / `op.PlatformResult` / `op.SearchResult` contract, `op.PURL`, and a full per-OS
  manager set (`platform_linux.go`, `platform_darwin.go`, `platform_windows.go`).
- **`pkg/platform/*`** — a newer **interface**-based `platform.Platform`, a fluent `PlatformSpec` builder,
  a named-system catalog (`defaults.go`), constructors + host `Detect()`, plus its **own** duplicate copies
  of the same managers and contract types.

The driving requirement is **cross-platform graph production**. A graph authored on macOS that targets
Ubuntu must carry its target `Platform` (`os: linux, distro: ubuntu, arch: amd64`) **baked into the
serialized graph document**, then run on a real Ubuntu host where the live package/service managers are
attached. The app, the framework, and starlark code must share the **same** `Platform`, injected onto
`RuntimeEnvironment.Platform` exactly like the `ui.Provider` capability pattern: the app constructs it
(detect the host **or** build a named target from scratch), it flows onto the runtime environment, and the
stateless `pkg/op/provider/platform` provider exposes it to starlark.

`op.Platform` is the keeper — it is the type woven into `op`'s document model and shared by everyone.
`pkg/platform` is kept as the **implementation layer**, re-pointed at the `op` contract.

## Decisions (settled with the user)

1. **`op.Platform` is the single shared contract** in `pkg/op`; `pkg/platform` implements it.
   **Option A** — `pkg/platform` imports `pkg/op`; `pkg/op` stops importing `pkg/platform`.
2. **`op.Platform` is an interface, serialized via an unexported `platformData`** struct in `pkg/op` with
   custom `MarshalJSON` / `UnmarshalJSON` + `MarshalYAML` / `UnmarshalYAML` (a **document** format, not a
   wire/protocol format). Identity fields serialize; managers are runtime-only and never serialized.
   This **supersedes** [`devlore-op-platform-struct.md`](../../devlore-op-platform-struct.md) (2026-02-24
   draft, which chose a struct — it predates the `13.0(i)` interface design now live in `pkg/platform`).
3. **Construction = named specs → seal.** Each named function returns a **fresh, mutable `*PlatformSpec`**
   (a clone of the catalog default); `NewPlatform(spec)` seals it into an `op.Platform`. There is **no
   `.Build()`** terminal. The sealed one-shots `Linux(distro,arch)` / `Darwin(arch)` / `Windows(arch)` are
   **dropped**.

   ```go
   spec := platform.Debian()                  // a fresh *PlatformSpec — mutate freely
   spec.WithArch("arm64").WithVersion("12")
   target, err := platform.NewPlatform(spec)  // seal → op.Platform

   spec, err := platform.Detect()             // (*PlatformSpec, error) — host spec, mutable
   target, err := platform.NewPlatform(spec)
   ```

4. **The platform catalog is 10 named specs** (Arch and Manjaro deferred 2026-06-04 — re-addable later):
   `Darwin`, `Debian`, `Ubuntu`, `Mint`, `RHEL`, `Fedora`, `CentOSStream`, `AlmaLinux`, `Rocky`, `Windows`. The
   **manager** set per platform is restructured by decisions 5–8 below (the Composite model), which supersedes the
   earlier "no new managers" stance.
5. **`op.PackageManager` is a Composite router** — `pkg.Provider` veneer → composite router → leaf drivers, one
   `op.PackageManager` contract at every level. See *Package-manager architecture* below.
6. **Language managers are shared cross-platform singletons** (`npm`, `pip`, `gem`, `cargo`, `go`) declared once
   and consumed by every platform; the standalone `npm` provider is absorbed. Native managers stay platform-bound.
7. **Routing is by purl; there is no `manager` argument.** A bare purl is normalized with the platform's default
   native manager at `pkg.Resource` construction, so every `Resource` reaching the router carries a resolved
   manager and routing is total and deterministic.
8. **Fan-out returns one `Receipt` per package** (the unified result); partial failure is normal. Convergence is
   the manager's own idempotency; outcome verification lives in the leaf.
9. **Verb surface = Install / Remove / Upgrade** — the familiar triad, kept for consumer expectations (not
   collapsed to `Install`-only). Applies to both `op.PackageManager` and `pkg.Provider`.
10. **No public `Update`.** Index refresh is an internal per-leaf, staleness-gated strategy (see *Index
    freshness*); auto-refreshing managers are no-ops; a `refresh=true` kwarg can force it.

## Package-manager architecture — Composite router over leaf drivers

`op.PackageManager` is a **Composite**: the same contract at every level. `pkg.Provider` is a thin veneer over a
single `op.PackageManager` — the platform's **router** — which never exposes its internals to the consumer.

```go
// op.PackageManager — identical contract for leaf and composite.
type PackageManager interface {
    Install(packages []*Resource, kwargs map[string]any) (receipts []*Receipt, err error)
    Remove(packages  []*Resource, kwargs map[string]any) (receipts []*Receipt, err error)
    Upgrade(packages []*Resource, kwargs map[string]any) (receipts []*Receipt, err error)
}
```

The verb surface is the familiar **Install / Remove / Upgrade** triad — kept for consumer expectations (apt and
brew expose all three even though `install` alone could converge everything), not collapsed to `Install`-only.
There is **no public `Update`**: refreshing a manager's package index is an internal per-leaf strategy
(*Index freshness*, below).

**Three pure layers:**

- **Leaf = mechanism.** `aptManager`, `npmManager`, … each do the operation on the list they are handed and return
  one `Receipt` per package, then stop. A leaf knows nothing of routing or other managers. Each leaf: pre-query
  state → run the (idempotent) manager command → re-query state → emit a `Receipt`, erroring that receipt if the
  post-state did not reach what the package's purl requested (outcome verification lives here).
- **Composite = routing + fan-out + unified receipts.** The platform's router groups the incoming list by purl,
  fans each slice out to its leaf concurrently, and concatenates the leaves' receipts into one unified result. It
  *is* an `op.PackageManager`, so the consumer sees one uniform surface.
- **`pkg.Provider` = thin veneer.** Adapts the starlark/graph call, hands the list down, hands the unified receipts
  up. No policy, no convergence, no manager selection.

**Convergence is the manager's own idempotency, not a layer.** `apt-get install -y git=2.39.0` already
installs-or-noops-or-changes-version. The only declaration is the verb: `Install` = converge to *present*,
`Remove` = converge to *absent* — chosen by the caller at authoring time, never decided at runtime. This is why
there is no `manager` argument and no reconciler "policy": the `Resource` (purl + version) plus the verb fully
describe the intent before it reaches the router.

### Flow

```
pkg.Provider.Install(resources, kwargs)              ← thin veneer: starlark/graph adapter
        │  (passes the list straight through)
        ▼
op.PackageManager [composite].Install(resources, kwargs)
        │  group by purl type
        ├── apt → aptManager.Install([git=2.39], kwargs)
        │           1. pre  = query installed? version?         ┐
        │           2. run  = apt-get install -y git=2.39       │ idempotent —
        │              (installs / no-ops / changes version)    │ the manager
        │           3. post = re-query installed? version?      │ converges
        │           4. Receipt{git, pre, post,                  │
        │                      err if post ≠ requested}         ┘ ← verification
        ├── npm → npmManager.Install([typescript], kwargs) → Receipt{…}
        └── pip → pipManager.Install([black], kwargs)      → Receipt{…}
        │  concatenate the leaves' receipts
        ▼
unified []*Receipt  ── returned straight back out through pkg.Provider
```

### Driver catalog — native per platform, language managers shared

A platform's routing table is `nativeDrivers ∪ sharedLanguageDrivers`:

- **Native drivers (platform-bound)** — declared per platform (locked 2026-06-04):

  | Platform | Drivers |
  |---|---|
  | Debian (→ Ubuntu, Mint) | `apt` ● (repo + local `.deb`), `snap`, `flatpak` |
  | Fedora (→ RHEL, CentOS, Alma, Rocky) | `dnf` ● (repo + local `.rpm`), `snap`, `flatpak` |
  | Darwin | `brew` ●, `port` |
  | Windows | `winget` ● |

- **Shared language drivers (cross-platform singletons, consumed by every platform)** — declared **once** and
  referenced by each platform: `npm`, `pip`, `gem`, `cargo`, `go`. `cargo install X` behaves identically on every
  host, so there is no per-platform copy. This absorbs the standalone `npm` provider.

`PackageManagerByName` / `DefaultPackageManager` / `AvailablePackageManagers` collapse into the router's
**internal routing table** — the consumer never selects a manager; the purl does.

**Deferred — re-addable as one leaf driver each (the model makes adds cheap, so deferring is free):** Arch
/`pacman`; Windows `choco`/`scoop`; direct-file `deb`/`rpm` as drivers distinct from `apt`/`dnf` (folded into them
instead — decision 1a); version-managers `asdf`/`mise`; declarative `nix`; JVM/other-language `maven` (rejected — a
build/dependency tool, not a binary installer), `sdkman`, `composer`, `dotnet tool`. None block the first cut.

### Normalization — the default manager is frozen at construction

A `pkg.Resource` may be built from a bare purl with no manager (`"git"`). At construction it is **normalized by
inserting the platform's default native manager** into both `Resource.URI` and the package name — `"git"` on
Debian becomes `apt`-qualified (`pkg:apt/git`); `"npm:typescript"` stays `npm`. So **every `Resource` reaching the
router already carries a resolved manager** in its purl; routing is total and deterministic, and the default
decision is frozen against the platform that built the `Resource` — the router never guesses.

### Index freshness — `Update` is internal, not a consumer verb

Refreshing a manager's package index (`apt-get update`, `pacman -Sy`) is **not** exposed as `pkg.Provider.Update`
or a public `op.PackageManager.Update`. Each **leaf** owns its freshness strategy: refresh transparently before an
`Install`/`Upgrade` when its index is stale, gated by a staleness guard so we do not hit the network on every call.
Managers that already auto-refresh (`brew`, `dnf`, `winget`; the language managers query their registry live) make
the strategy a no-op. A `refresh=true` kwarg can force it if ever needed. This keeps the consumer surface to the
Install/Remove/Upgrade triad.

### Failure handling — see the compensation-failure contract

A leaf's `Install` / `Remove` / `Upgrade` is best-effort: attempt every package, return one `Receipt` per package,
`error` if any failed. How a failure becomes `Degraded` (a consumer `flow.Degraded` error action), `Failed`
(unhandled → unwind), or `Stranded` (a failed unwind) is the framework's job, specified in
[`compensation-failure-contract.md`](compensation-failure-contract.md) — error actions MUST run; the four run
terminals are `Completed` / `Degraded` / `Failed` / `Stranded`. The pkg leaves conform and add no failure logic.

## End-state architecture

### Contract — `pkg/op`
- `op.Platform` **interface** — identity + capability surface: `OS`, `Arch`, `Distro`, `Version`, `Hostname`,
  `DefaultConcurrency`, `ServiceManager`, and a single **`PackageManager()`** returning the platform's Composite
  router. The old consumer-facing selection methods (`DefaultPackageManager`, `AvailablePackageManagers`,
  `PackageManagerByName`) collapse into the router's internal routing table; `InstalledBy` / `AllInstalledBy`
  move onto the router (they are manager queries, not platform identity). Defined in `pkg/op/platform.go`.
- `op.PackageManager` — reshaped into the **Composite router** (see *Package-manager architecture*): `Install` /
  `Remove` take `[]*Resource` + `kwargs` and return `[]*Receipt`, one per package. `op.ServiceManager` /
  `op.PlatformResult` / `op.SearchResult` — kept (already match the `pkg/platform` shapes). `op.PURL`
  (`pkg/op/purl.go`) — kept as canonical.
- `platformData` (new, unexported) — identity-only serializable type implementing `op.Platform` with nil
  managers; home of the custom JSON/YAML marshalers. The form a deserialized graph yields.

### Implementation — `pkg/platform` (re-pointed, `imports pkg/op`)
- Deletes its own `Platform` interface, its `manager.go` contract types, and `purl.go` — references `op.*`.
- Managers (`aptManager` … `windowsServiceManager`, `snapManager`, `flatpakManager`) implement
  `op.PackageManager` / `op.ServiceManager`.
- `PlatformSpec` builder kept; `defaults.go` catalog kept. **New named spec functions** —
  `Debian()` / `Ubuntu()` / `Mint()` / `RHEL()` / `Fedora()` / `CentOSStream()` / `AlmaLinux()` /
  `Rocky()` / `Arch()` / `Manjaro()` / `Darwin()` / `Windows()` — each return `*PlatformSpec` (the existing
  `defaults.go` factories, promoted/exported). `Detect()` changes to `(*PlatformSpec, error)`.
  `NewPlatform(spec)` returns `op.Platform`. The sealed one-shots are removed from `constructors.go`.

### Deleted — redundant old manager set in `pkg/op`
- `pkg/op/platform_{linux,darwin,windows}.go` + their `*_panic.go`
- `pkg/op/platform_new.go` (`op.NewPlatform()`)
- `pkg/op/platform_helpers.go` (`runShellCommand` / `detectArch` — verified used only by the deleted
  managers; `pkg/platform/helpers.go` is the survivor)
- `pkg/op/platform_test.go` (tests the deleted `op.NewPlatform`)
- `pkg/platform/purl.go` (duplicate of `pkg/op/purl.go`)

## Design considerations — action semantics under override-capable options

Deep-research (2026-06-04, verified against primary docs for Puppet, Chef, Ansible, apt-get, dnf, brew,
winget; 23/25 claims survived 3-vote adversarial verification) confirms a **two-layer** industry pattern.
Adopt it:

**Layer 1 — normalized declarative state, capability-gated.** Model the action as a declarative *state*
(Puppet `ensure`, Ansible `state`), not as verb methods that assume their own meaning. Portable states
(`present` / `absent`) are universal; non-portable states (`latest`, `held`, `purged`, exact `version`)
are **capability-gated** — each manager declares which it implements, and the framework refuses a state a
manager cannot honor rather than silently mis-running it (Puppet: 11 features over ~40 providers; `ensure`→
method gating present→install / absent→uninstall / latest→update / version→install). [high]

**Layer 2 — opaque raw-flag passthrough, explicitly outside the semantic contract.** Vendor-specific flags
(Puppet `install_options`, Chef `options`) are forwarded **verbatim, per-manager, not normalized**. This is
the escape hatch — and it is exactly where semantics get subverted: Chef `:upgrade` ignores `version`;
`apt-get install foo-` (trailing hyphen) *removes* foo; `apt install --only-upgrade foo` installs nothing
when foo is absent (identical for `apt-get`); the `apt --allow-*` family is "dangerous"
(`--allow-unauthenticated` a "huge security risk"). [high]

**The safety net (our requirement, confirmed by the research's open questions).** Because Layer 2 can change
the effective action, success and resulting state MUST be derived from the **actual post-condition**
(re-query installed state / version), never inferred from which method/state was requested. This is the
idempotency hazard the research flags as unresolved in those tools — devlore closes it with outcome
verification in the managers (`pkg/platform`) and the pkg-install reconciler
([`pkg-install-reconciler.md`](pkg-install-reconciler.md)).

Sources: Puppet package type & `package.rb`; Chef package / yum_package; Ansible package module; apt-get(8);
dnf command ref; brew manpage.

### Cross-platform common options — DEFERRED (out of unification scope)
**Resolved (item 5): this table is OUT of `platform-unification`.** It is an input to a future capability-gating
plan, not part of the verb-shell + opaque-passthrough model this plan ships. The candidate table below is
**UNVERIFIED** and recorded only as a starting point for that future plan — do not implement it here.

The research did **not** produce a verified cross-tool option-name table (it is an explicit open question).
The authoritative cross-tool mappings to mine are **pacaptr** (`github.com/rami3l/pacaptr`) and the
**upkg rosetta stone** (`github.com/Inducido/upkg-package-manager-rosetta-stone`). Candidate normalized
options to capture — **names UNVERIFIED, confirm against those sources before coding**:

| Normalized | apt-get | dnf | pacman | brew | winget |
|---|---|---|---|---|---|
| assume-yes / non-interactive | `-y` | `-y` | `--noconfirm` | (default) | `--silent` + `--accept-*-agreements` |
| dry-run / simulate | `-s` | (test) | `-p` | `--dry-run` | (none) |
| download-only | `-d` | `--downloadonly` | `-w` | (n/a) | `--download` |
| reinstall | `--reinstall` | `reinstall` | `-S` (default) | `reinstall` | (none) |
| only-upgrade | `--only-upgrade` | `upgrade` verb | (none) | (n/a) | (none) |
| no-recommends | `--no-install-recommends` | `--setopt=install_weak_deps=0` | (n/a) | (n/a) | (n/a) |
| skip-integrity (DANGEROUS) | `--allow-unauthenticated` | `--nogpgcheck` | `--force` | (n/a) | (n/a) |
| accept-license | (n/a) | (n/a) | (n/a) | (n/a) | `--accept-package-agreements` |

**DECISIONS:**
1. **Action model — RESOLVED (Composite model above).** Keep verb-based managers (`Install` / `Remove`): leaves
   are thin verb shells, convergence is the manager's idempotency, outcome verification is in the leaf, and
   `kwargs` carry opaque native flags. Declarative state + capability-gating, if ever wanted, is a separate
   follow-on — not this plan.
2. **Dangerous overrides — RESOLVED: escape-hatch passthrough only.** `kwargs` are opaque native flags; consumers
   know their manager's flags. No typed-and-gated normalized options in this pass — the backstop is the **leaf's
   outcome verification** (re-query post-state), which catches a flag that subverted the result (`apt-get install
   foo-` removing `foo`, `--only-upgrade` no-op-ing when absent) and errors the receipt regardless of which flags
   ran. Typed/gated cross-tool options are a future capability-gating plan.

## Work breakdown

**Phasing (item 3 — resolved).** Phases 1, 2, and 4 are **structurally coupled** and land as **one coordinated
green commit**: the `op.Platform` name cannot be both a struct and an interface, and the `pkg/op ↔ pkg/platform`
import direction cannot be half-flipped (circular-import hazard), so the contract flip, the `pkg/platform`
re-point, and the consumer migration cannot be split into independently-green micro-phases (the 13.0(n) monster-PR
precedent applies). **Phase 3 (delete + verify) is the separable, independently-green LAST step** — deleting the
now-dead `pkg/op` manager files and grepping the import direction is green on its own. Effective order:
**[1 + 2 + 4 together] → [3 last]**; `make check` green at each commit boundary.

### Phase 1 — `op.Platform` contract + serialization (`pkg/op`)
Rewrite `pkg/op/platform.go`: `op.Platform` becomes the 13-method interface; keep the manager / result
types. Add unexported `platformData` implementing `op.Platform` (identity-only) with custom
`MarshalJSON` / `UnmarshalJSON` / `MarshalYAML` / `UnmarshalYAML`.

### Phase 2 — `pkg/platform` re-points to the `op` contract
`import pkg/op`; delete `pkg/platform`'s `Platform` interface + `manager.go` type duplicates + `purl.go`;
managers implement `op.*`. Promote the `defaults.go` factories to exported named spec functions returning
`*PlatformSpec`; change `Detect()` to `(*PlatformSpec, error)`; `NewPlatform(spec)` returns `op.Platform`;
remove the sealed one-shots.

### Phase 3 — delete redundant `pkg/op` managers + verify import flip
Delete the seven `pkg/op/platform_*.go` files + `pkg/platform/purl.go`. Verify **no `pkg/op` package file
(non-subpackage) imports `pkg/platform`**.

### Phase 4 — consumer migration (type flips `platform.* → op.*`; methods unchanged)
- `pkg/op/runtime_environment.go`: `Platform platform.Platform` → `op.Platform`; `WithPlatform(...)`
  signature; drop the `pkg/platform` import.
- `pkg/op/provider/{pkg,service,platform}/**` (incl. `*_test.go` mocks): `platform.Platform` /
  `platform.PackageManager` / `platform.ServiceManager` / `platform.PlatformResult` /
  `platform.SearchResult` / `platform.PURL` → the `op.*` equivalents. Method calls unchanged.
- `internal/lorepackage/{package.go:264, search.go:131, search.go:228}`: `op.NewPlatform().PackageManager`
  → `platform.Detect()` + `platform.NewPlatform` + `.DefaultPackageManager()`, with `(*PlatformSpec, error)`
  / `(Platform, error)` + nil-manager handling at each site.
- `internal/execution/provider_test.go` (`//go:build ignore`): its mocks target `op.ServiceManager` /
  `op.PlatformResult`, which **survive** — re-verify it compiles, minor / no change.

### Phase 5 — graph embedding + preflight — MOVED OUT (separate follow-on)
**Resolved (item 2): Phase 5 is not in this plan.** Baking the target `op.Platform` into the serialized graph
(`Origin` / `OriginBase` `TargetPlatform`) and the **preflight platform-mismatch check** (`platform.Detect()` vs
the graph's target) is the **Scenario-1 capstone** — its own follow-on, tracked as
[#282](https://github.com/NobleFactor/devlore-cli/issues/282), slotted with the lore deploy milestone (after the
contract flip lands). This plan stops at making `op.Platform` *serializable* (Phases 1–4); Phase 5 is what *uses*
the serialized form end-to-end.

## Critical files
- `pkg/op/platform.go` (rewrite), `pkg/op/purl.go` (keep)
- `pkg/platform/{platform.go,manager.go,constructors.go,defaults.go,*_managers_*.go}` (re-point),
  `pkg/platform/purl.go` (delete)
- `pkg/op/runtime_environment.go`; `pkg/op/provider/{pkg,service,platform}/**`
- `internal/lorepackage/{package.go,search.go}`; `internal/execution/provider_test.go`
- Delete: `pkg/op/platform_{linux,darwin,windows}.go` + `*_panic.go`, `platform_new.go`,
  `platform_helpers.go`, `platform_test.go`

## Verification
1. `make check` — vet + lint + all tests green (`pkg/op/...`, `pkg/platform/...`,
   `pkg/op/provider/{pkg,service,platform}/...`, `internal/lorepackage/...`).
2. Import direction: `pkg/platform` imports `pkg/op`; no `pkg/op` package file imports `pkg/platform`
   (`go list -deps` / grep).
3. Round-trip: marshal an `op.Platform` built from `platform.Ubuntu()` to JSON **and** YAML, unmarshal,
   assert identity fields survive and managers are nil (the document form).
4. Cross-host fixture: on macOS, `platform.Ubuntu()` + `NewPlatform` constructs without touching the host;
   `platform.Detect()` returns the real host spec.
5. Regenerate provider gen code (`pkg/op/provider/{pkg,service,platform}`) and re-run `make check`.

## Process notes
- The pending `../go-pissant` style commit for `pkg/op/platform_linux.go` is **abandoned** — that file is
  deleted by this plan.
- Work stays on `refactor/extract-starlark-from-op.phase-8` (no branch switch; the user runs all git).
- A GitHub tracking issue is opened after this plan is approved, before code.

---

# Resolved direction & contract — step 21.4 (2026-06-04)

**This section supersedes Decisions 1–2 and the Phase 1–4 framing above.** Settled with the user on 2026-06-04
after establishing that `pkg/platform` is already **op-free** (it imports only the stdlib — strictly more
independent than `pkg/result` / `pkg/status`, which import `pkg/sink`), and that `pkg/op` already imports it
(`runtime_environment.go`).

## Direction — keep `pkg/platform` op-free; delete the `op` duplicate (reversal of the flip)

The unification keeps `pkg/platform` as a **standalone, op-free capability** that `pkg/op` *imports* — the same
shape as `pkg/result` and `pkg/status`, **not** the original "move the contract into `op`" flip. The duplicate
**`op.Platform` struct is deleted**; everyone consolidates on **`platform.Platform`**.

1. `pkg/platform` stays op-free (imports no devlore package). `pkg/platform/purl.go` is **kept** (the earlier
   "delete duplicate purl.go" was predicated on the flip).
2. `platform.PackageManager` is reshaped into the **Composite router** (below), returning a **platform-local
   `Receipt`** — never `op.Receipt`. Returning `op.Receipt` would force `pkg/platform` to import `pkg/op` and
   destroy the op-free property that justifies its standalone existence.
3. The `op.Receipt` compensation receipt is minted by the **manager-aware veneer** (`pkg.Provider`, which already
   imports `op`) by adapting `[]platform.Receipt`. `pkg.Provider` / `pkg.Resource` are the **only**
   package-manager-aware op-side types; `op` itself never names a package-manager concept.
4. **Deleted from `pkg/op`:** `platform.go`, `purl.go` (`op.PURL`/`op.ParsePURL` are orphaned once the `op`
   managers go — the sole live `ParsePURL` call routes through the *platform* manager), `platform_{linux,darwin,
   windows}.go` + `*_panic.go`, `platform_new.go`, `platform_helpers.go`, `platform_test.go`.

**Import direction (one-way):** `pkg/platform` (op-free) ← imported by `pkg/op`, `pkg/op/provider/pkg`,
`pkg/op/provider/service`, `internal/lorepackage`. Nothing `platform` imports points back into devlore. (The
plan's old verification "no `pkg/op` file imports `pkg/platform`" is **inverted** — `op` importing `platform` is
correct and intended.)

**Out of scope (deferred to #282 / former Phase 5):** baking the target platform into the serialized graph
(`Origin.TargetPlatform`) and the preflight host-mismatch check. Identity marshalers, when needed, live on
`platform` (op-free), not `op`. This step ships the contract reshape only.

## Contract — `pkg/platform` (reshaped)

```go
// PURL — KEPT as-is (pkg/platform/purl.go). The routing key; Type selects the leaf.

// Receipt — NEW, op-free. One per package; partial failure is normal. State is observed by re-query
// (pre/post), never by scraping command output.
type Receipt struct {
    Purl         PURL   // the package acted on (Purl.Type = the leaf that handled it)
    PriorVersion string // installed version observed BEFORE the op ("" if absent)
    Version      string // installed version observed AFTER  ("" if absent / removed)
    Err          error  // non-nil if the post-state did not reach what the purl requested
}

// PackageManager — Composite router; the SAME contract at leaf and composite.
type PackageManager interface {
    // Verbs — converge per package; one Receipt each; route by Purl.Type.
    Install(packages []PURL, kwargs map[string]any) ([]Receipt, error)
    Remove(packages  []PURL, kwargs map[string]any) ([]Receipt, error)
    Upgrade(packages []PURL, kwargs map[string]any) ([]Receipt, error)
    // Queries — folded into the one contract (per the 2026-06-04 decision).
    Installed(p PURL) bool
    Version(p PURL) string
    Available(p PURL) bool
    Search(query string, limit int) []SearchResult
}

// SearchResult — gains Manager so the composite can fan out and each hit self-identifies its leaf.
type SearchResult struct { Name, Version, Description, Manager string }
```

- **Leaf = mechanism.** Each `aptManager` … operates on the `[]PURL` slice it is handed, brackets every op with a
  pre-/post-state query, and returns one `Receipt` per package (erroring that receipt when the post-state misses
  the requested purl). Leaf shell-out bodies are kept; only the method *shapes* change
  (`Install(...string) PlatformResult` → `Install([]PURL, kwargs) ([]Receipt, error)`, etc.). `ParsePURL`,
  `AddRepo`, `Update`, `NeedsSudo`, `Name` leave the contract — `Update` becomes the internal staleness-gated
  refresh; `ParsePURL` is replaced by purl construction in the veneer; per-leaf identity stays on the concrete
  type for the router's table, off the interface.
- **Composite = routing + fan-out + unified receipts.** The router groups input `[]PURL` by `Purl.Type`,
  dispatches each slice to its leaf concurrently, concatenates the `Receipt`s. An unknown purl type sets *that
  package's* `Receipt.Err` — the call still returns the rest.
- **Platform exposes one router.** `platform.Platform` drops `DefaultPackageManager` /
  `AvailablePackageManagers` / `PackageManagerByName` / `InstalledBy` / `AllInstalledBy`; it gains
  `PackageManager() PackageManager` (the router) and `DefaultManagerName() string` (the default native type,
  consumed by the veneer to normalize bare purls). `ServiceManager()` is unchanged.

## Construction surface — clones of well-known defaults, or detect

Construction is **clone a named default spec, mutate, seal** — or **detect the host**. There is **no** blank-spec
constructor (`NewSpec`): the catalog *is* the supported set, so every spec originates from a known system (a clone
of its catalog default) or from the running host.

```go
// Named factories — each returns a FRESH *Spec (a clone of its catalog default), safe to mutate:
platform.Debian()  platform.Ubuntu()  platform.Mint()
platform.RHEL()    platform.Fedora()  platform.CentOSStream()  platform.AlmaLinux()  platform.Rocky()
platform.Darwin()  platform.Windows()
platform.Detect() (*Spec, error)   // the host spec

target, err := platform.New(platform.Ubuntu().WithArch("amd64").WithVersion("22.04"))  // seal → Platform
host,   err := platform.Detect()
self,   err := platform.New(host)
```

Renames folded in: `PlatformSpec` → **`Spec`**, `NewPlatform` → **`New`**. The sealed one-shots
`Linux(distro,arch)` / `Darwin(arch)` / `Windows(arch)` are **dropped** in favor of the per-system `*Spec`
factories above (`Spec` keeps its pointer-receiver `With*` chain). Arch / Manjaro stay deferred (re-addable as one
factory each). `New` takes **`*Spec`**; `Spec` is **exported** — it crosses the factory and `New` boundaries, so
an unexported `spec` would trip `revive`'s `unexported-return` and block caller-side helpers and struct fields.

## Veneer — `pkg.Provider` / `pkg.Resource` (the manager-aware layer)

- **`buildCandidate` (`pkg.Resource` construction):** a bare name (`"git"`) is normalized by qualifying it with
  `Platform.DefaultManagerName()` → `PURL{Type: "apt", Name: "git"}`; a prefixed name (`"npm:typescript"`) →
  `PURL{Type: "npm", Name: "typescript"}`; a versioned form (`"foo@1.2"`) parses the `@`. Uses `platform`'s purl
  construction (kept), **not** a manager `ParsePURL` method.
- **Queries** (`pkg.Resource.Etag`, `pkg.Provider.Observe` / `Installed` / `NotInstalled` / `VersionGTE`): build
  a `PURL` from the resource's `Type`+`Name` and call `Platform.PackageManager().Version(purl)` /
  `.Installed(purl)`. The `resolvePlatformManager*` helpers (`pkg/op/provider/pkg/helpers.go`) are **deleted**.
- **Verbs** (`pkg.Provider.Install` / `Remove` / `Upgrade`): signature becomes
  `(packages []*Resource, kwargs map[string]any)` — drops `manager` (routing is by purl) and `cask` (`cask`
  becomes `kwargs["cask"]`, honored by the brew leaf). Converts `[]*Resource` → `[]PURL`, calls the router, adapts
  `[]platform.Receipt` → `op.Receipt` (`pkg.Receipt`), correlating by purl and reconstructing the compensation
  tombstone from `PriorVersion` (`""` ⇒ was absent ⇒ unwind removes it). **Provider gen code is regenerated**
  (`provider.gen.go` `ParameterNames` change from `[packages, manager, cask]`).
- **`pkg.Provider.Update`** (the standalone index-refresh action) is **removed** (no public Update).
- **`service.Provider`** is unaffected (`ServiceManager`, unchanged).

## Consumer migration map

| Site | Now | After |
|---|---|---|
| `pkg/op/runtime_environment.go` | `Platform platform.Platform` | unchanged (already correct) |
| `pkg/op/provider/platform/provider.go` | calls `re.Platform.Arch()` …; doc says `op.Platform` | **doc-comment-only** change to `platform.Platform` |
| `pkg/op/provider/pkg/*` | `platform.PackageManager` query verbs + `manager`/`cask` params | router verbs + PURL queries; veneer adapts receipts |
| `internal/lorepackage/{search,package}.go` | `op.NewPlatform()` + struct `.PackageManager` field | `platform.Detect()`; `Search`/`Installed`/`Available` via the router (PURL built from `SearchResult.Manager`) |

## Verification (revised)

1. `make check` green at the commit boundary (`pkg/op/...`, `pkg/platform/...`,
   `pkg/op/provider/{pkg,service,platform}/...`, `internal/lorepackage/...`).
2. Import direction: `pkg/platform` imports nothing in devlore; `pkg/op` imports `pkg/platform` (allowed).
3. Round-trip a `[]platform.Receipt` → `op.Receipt` adaptation in `pkg.Provider` (compensation tombstone built
   from `PriorVersion`).
4. Cross-host: on macOS, `platform.New(platform.Ubuntu())` constructs without touching the host;
   `platform.Detect()` returns the real host spec.
5. Regenerate `pkg/op/provider/pkg` gen code; re-run `make check`.

## Sub-decisions — resolved 2026-06-04

1. **Verb input type** — **`[]PURL`** (`PURL` carries Type/Name/Version; a `[]Package` wrapper buys nothing yet).
2. **Receipt correlation key** — **full `Receipt.Purl`** (the unambiguous key against the input list).
3. **Named-spec catalog — IN this step.** Spec-first construction is adopted: promote the `defaults.go` factories
   to exported per-system `*Spec` functions (`Debian`/`Ubuntu`/`Mint`/`RHEL`/`Fedora`/`CentOSStream`/`AlmaLinux`/
   `Rocky`/`Darwin`/`Windows`), change `Detect()` → `(*Spec, error)`, drop the sealed `Linux`/`Darwin`/`Windows`
   one-shots. See *Construction surface* above.
4. **`cask`** — **opaque `kwargs["cask"]`** honored by the brew leaf (matches the opaque-native-flags decision).
5. **Type/constructor naming** — type stays **`platform.Platform`** (flagship-type idiom, `time.Time` company);
   `PlatformSpec` → **`platform.Spec`**; `NewPlatform` → **`platform.New(spec *Spec)`**. `Definition` rejected
   (overlaps `Spec`; under-describes the runtime capability).