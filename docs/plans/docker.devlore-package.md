---
title: "Docker devlore Package — a clean-slate rewrite that proves the lifecycle"
issue: TBD
status: draft
created: 2026-08-26
updated: 2026-08-26
---

# Plan: Docker devlore Package

## Summary

Replace the `docker` package in devlore-registry with a hand-written rewrite whose only source of
truth is the bash that deploys Docker today: `Home/noblefactor.Unix/.local/bin/Install-Docker` and
`Tools/Debian/Initialize-Debian` in the Personal repository. The existing package is discarded
entirely — it targets an API that no longer exists, and it describes a Darwin installation the user
does not perform. The rewrite exists to prove three lifecycle operations end to end (deploy,
upgrade, decommission) and to state precisely why the fourth (reconcile) cannot yet be proven by a
package.

## Goals

1. **Fidelity**: every step traces to a line of bash that runs today, or is deliberately marked as
   an addition with a reason.
2. **Current API only**: no call that the provider tree does not implement as of this branch.
3. **Receipts throughout**: every mutating step is compensable, because decommission is
   compensation in reverse receipt order rather than a hand-written teardown.
4. **Honest scope**: reconcile is described, not faked.

## Current State

### The package being replaced

`devlore-registry/packages/docker/` — 28 `.star` files across `Darwin`, `Linux.Debian`,
`Linux.Fedora`, and `Windows`, plus `README.md` (199 lines) and `lifecycle.yaml` (90 lines).

| Component | Status | Notes |
| --- | --- | --- |
| `plan.package.install("a", "b")` | Broken | Namespace is `pkg`; takes a list, not varargs |
| `plan.verify(name, check=…)` | Broken | No `Verify` method on any provider |
| `plan.file.write(path=…)` | Broken | It is `plan.file.write_text(destination_path=…, content=…, mode=…)` |
| `plan.user.add_to_group(…)` | Broken | Does not exist |
| `phase.env("USER")` | Broken | Does not exist |
| `plan.download(url, dest)` | Broken | `appnet.download(url)` returns bytes; there is no dest form |
| `plan.notify(…)` | Broken | Does not exist |
| All four `verify.star` | Dead | Bodies are entirely `plan.verify` calls |
| Darwin phases | Wrong | Describe a Docker Desktop DMG install that is not performed |
| `plan.service.enable/start` | Correct | Retained |
| `package.has_feature()` / `package.setting()` | Correct | Retained |

### The API that does exist

Verified against provider source on this branch.

| Surface | Members |
| --- | --- |
| `plan.pkg.*` | `install`, `installed`, `not_installed`, `observe`, `remove`, `update`, `upgrade`, `version_gte` |
| `plan.service.*` | `disable`, `enable`, `enabled`, `exists`, `restart`, `running`, `start`, `stop` |
| `plan.shell.*` | `exec` |
| `plan.file.*` | `copy`, `write_text`, and the rest of the mutating surface |
| `package.*` | `dry_run`, `features`, `has_feature`, `name`, `settings`, `setting`, `source_root`, `target_root`, `version` |
| Denied in phase scripts | `plan.assemble`, `plan.clear`, `plan.load`, `plan.run`, `plan.save` (`cmd/lore/lore/builder.go:31`) |

`service.Enable`, `service.Start`, `service.Stop`, and `service.Disable` each return
`(Resource, *Receipt, error)` — the receipt is what makes decommission mechanical.

The package-manager router wires **apt, brew, port, winget**. There is **no dnf**.

### The bash that is the source of truth

Two tiers, and the split matters:

- `Tools/Debian/Initialize-Debian` — root, machine scope, re-runnable. Writes
  `/etc/apt/keyrings/docker.asc` (`:231`) and `/etc/apt/sources.list.d/docker.sources` in deb822
  form with `Signed-By:` (`:256`). Upgrades the whole system with `apt-get full-upgrade --yes`
  (`:120`).
- `Home/noblefactor.Unix/.local/bin/Install-Docker` — user scope, 84 lines, **assumes the repo
  already exists**. Removes seven conflicts guarded by `dpkg-query`, installs six packages
  including `lshw`, then verifies against detected hardware.

Idempotence is by guard, never by recorded state: `[[ -f … ]] || curl …`,
`command -v … || install …`, `dpkg-query -W … && remove …`. Re-running the script *is* the update
mechanism.

## Rulings

These were decided in session. Each is overturnable; none is a discovery.

1. **The package owns repository setup.** *Confirmed by the user, 2026-08-26: the `.sources` file and
   the keyring are ours, and they are prerequisites belonging to the first phase of deployment.*
   `docs/guides/lore/pipeline.md` assigns "Add package
   repository sources" and "Import GPG signing keys" to Phase 1 (Prepare). The bash splits it out
   only because `Initialize-Debian` batches every third-party repo at machine bootstrap. A package
   that assumed the repo would fail on any machine not already initialized by personal tooling,
   which is useless to a customer.
2. **Darwin ensures a runtime; it does not install Colima.** Colima is chosen for **licensing**:
   Docker Desktop requires a paid subscription above 250 employees or $10M revenue, and OrbStack has
   a paid commercial tier. The bash accepts any of the three and installs Colima only when none is
   present. Colima being scriptable makes it a far easier install to automate, but that is a
   consequence, not the reason.
3. **MacPorts before Homebrew.** `Install-Dependencies` tries `port` first and falls back to `brew`.
4. **`Linux.Fedora` is not in this package.** No dnf in the router. Driving it through
   `plan.shell.exec` would work but forfeits receipts, which would defeat the decommission proof.
   Chartered separately.
5. **`Windows` is not in this pass.** `winget` is wired, so it is viable later; it is simply not
   what the source bash covers.
6. **No `rootless`, no docker-group membership.** Neither appears in the bash. Everything is
   `sudo docker`.
7. **The package names no package manager.** `pkg/platform/defaults.go:72` gives Darwin
   `managers: []leaf{brew, port}` with `defaultManager: brew`, while `Install-Dependencies` tries
   `port` first. The development Mac has both. A registry package should not encode a manager
   preference — that is machine policy, and every leaf carries a purl type (`port`, `brew`) usable
   as an explicit routing prefix when someone genuinely needs one. Resources stay unprefixed and the
   router decides. If MacPorts should win on Darwin, the fix is `defaults.go:73`, which corrects it
   for every package rather than for docker alone. **Pending the user's ruling** — this diverges
   from the bash.

## Verified API shapes

Read off the generated bindings on this branch, not from documentation.

| Call | Starlark parameters |
| --- | --- |
| `plan.pkg.install` | `packages`, `**kwargs` |
| `plan.pkg.remove` | `packages`, `**kwargs` |
| `plan.pkg.upgrade` | `packages`, `**kwargs` |
| `plan.pkg.update` | none |
| `plan.pkg.installed` / `not_installed` | `name` |
| `plan.pkg.observe` | `resource` |
| `plan.pkg.version_gte` | `name`, `version` |
| `plan.file.write_text` | `destination_path`, `content`, `mode` |
| `plan.choose` | `*cases`, `default=` |
| `plan.case` | `when=`, `then=` |

`kwargs` are opaque native-installer flags forwarded to the routed leaf —
`pkg/op/provider/pkg/provider.go:44` names `cask` as the example.

## Resolved by probe, 2026-08-26

### `then=` and `default=` carry actions — Scenario 4's guard is expressible

**Answered: yes.** `cmd/devlore-test/devloretest/data/test_choose_then_action.star` gives both
branches live invocations instead of value lambdas, and passes: the then-body's `write_text` fires,
the default-body is never dispatched. The fixture is kept — it covers a shape the other nine
`test_choose_*` fixtures do not.

So the Darwin guard is:

```
plan.choose(
    plan.case(when=<runtime present?>, then=<no-op>),
    default=plan.pkg.install(packages=["colima", "docker"]),
)
```

### The receipt writer fails on any graph carrying a Starlark lambda

Found while probing, and material enough to gate authoring.

| Fixture | Lambdas | Expectations | Exit |
| --- | --- | --- | --- |
| `test_compensation.star` | none | pass | 0 |
| `test_choose_then_action.star` (lambda-free) | none | pass | 0 |
| `test_choose_lambdas.star` | many | pass | **1** |
| `test_choose_exists.star` | `default=lambda:` | pass | **1** |

The failure is `writing receipt: … function.Resource: mmap <session-root>/.devlore/function/resource/
sha256/…: no such file or directory`. A lambda is archived as a content-addressed
`function.Resource` at plan time; at receipt-write time the file is not on disk. Routing the receipt
to a real path rather than `/dev/null` makes no difference.

**Why CI does not see it**: `cmd/devlore-test/cli_test.go` exercises exactly one fixture,
`test_hello.star`, and contains zero references to choose. The remaining 103 fixtures are driven
in-process, where receipts are not written. Expectations pass either way, so the suite is green
while the receipt is unwritable — the same shape as a green knowledge-extract that emits a constant
artifact.

**Why it matters here**: receipts are the entire mechanism for Scenarios 3 and 5. A package whose
graph contains one lambda produces no receipt and therefore cannot be decommissioned by
compensation.

**Ruling 8 — package phase scripts contain no Starlark lambdas.** Build decision trees from
invocations only. This is an authoring constraint until the defect is fixed, and it costs nothing:
the lambda-free form is strictly more useful, since a branch that performs work beats one that
computes a string.

**Charter**: the `function.Resource` receipt defect needs its own issue and fix, and
`cli_test.go` needs to drive more than one fixture — the coverage gap is what let this sit.

### Superseded question — kept for the record

#### Can a `then=` body carry an action?

Scenario 4 needs "install Colima only when no runtime is present." The predicate half is settled:
`when=` accepts a live invocation, demonstrated by `plan.file.exists(path=…)` in
`test_choose_exists.star`, and `plan.file.is_dir` works the same way — enough to test for
`/Applications/OrbStack.app`.

The branch half is not. All nine `test_choose_*` fixtures pass **value-returning lambdas** to
`then=` and `default=`, and the fixture comments explain why: *"A lambda body desugars to a
function.call leaf: the lambda is archived as a content-addressed function.Resource at plan time and
invoked at dispatch."* That computes a value at dispatch; it is not a subgraph builder. Nothing
demonstrates `then=plan.pkg.install([…])`.

`cmd/devlore-test/devloretest/data/test_choose_then_action.star` was added to answer this. It gives
`then=` a live `plan.file.write_text` invocation and asserts the file appears. If it passes, the
fixture is worth keeping as coverage for a shape nothing else covers. If it fails, Scenario 4 cannot
express the guard and the fallbacks are: `plan.shell.exec` doing test-and-install as one opaque
command — which forfeits the install receipt and therefore Scenario 5's asymmetric decommission — or
new provider work.

Note this brushes against a stated intent. The demo milestone says *"NOT on this scenario's path:
`plan.choose` — adaptation is directory resolution, not a Starlark conditional."* That holds for
**platform** adaptation, which directories do handle. Runtime detection is not platform adaptation:
no directory layout can express "some container runtime is already present."

## Scenarios

Each scenario states preconditions, the graph the phase scripts contribute, the receipts produced,
verification, and postconditions. A scenario is proven when it runs end to end on a disposable
target and its postconditions hold.

### Scenario 1 — Deploy Docker on Linux.Debian

**Precondition**: a Debian or Ubuntu host with no Docker present, and no Docker apt repository
configured. Distribution packages (`docker.io`, `podman-docker`, …) may or may not be installed.

**Prepare** — `Linux.Debian/Deploy/prepare.star`

1. Remove the seven conflicting packages, each guarded so absence is not an error:
   `containerd`, `docker-compose`, `docker-compose-v2`, `docker-doc`, `docker.io`,
   `podman-docker`, `runc`. Contributed as `plan.pkg.remove([...])`.
2. Write `/etc/apt/keyrings/docker.asc` from `https://download.docker.com/linux/debian/gpg`.
3. Write `/etc/apt/sources.list.d/docker.sources` in deb822 form, `Suites:` bound to the detected
   distribution codename, `Signed-By:` pointing at the keyring.
4. `plan.pkg.update()`.

**Install** — `Linux.Debian/Deploy/install.star`

`plan.pkg.install(["containerd.io", "docker-buildx-plugin", "docker-ce", "docker-ce-cli",
"docker-compose-plugin", "lshw"])`.

`lshw` is not a Docker dependency. It is installed because verification is hardware-gated, and the
bash installs it for exactly that reason.

**Provision** — `Linux.Debian/Deploy/provision.star`

`plan.service.enable("docker")` then `plan.service.start("docker")`.

The bash does neither, because apt's `docker-ce` postinst already enables and starts the unit.
These are retained anyway: they are idempotent, they make the desired state explicit in the graph
rather than implicit in a maintainer script, and they are the only steps that emit a service
receipt — without them, decommission has nothing to compensate for the running daemon.

**Verify** — declarative, in `lifecycle.yaml`

```yaml
verification:
  command: docker --version
  pattern: "Docker version \\d+\\.\\d+"
```

There is no `verify.star`. `plan.verify` does not exist, and the milestone already specifies that
receipt verify status comes from matching `verification.pattern`.

**Hardware gate** — the one step with a non-binary outcome. The bash reads
`sudo lshw -json | jq -r '.product'` and branches:

| Detected product | Outcome |
| --- | --- |
| `ODROID-C4*`, `ODROID-C5*` | Check `/media/boot/boot.ini` for `systemd.unified_cgroup_hierarchy=0`. If absent, print numbered remediation and **exit 0** — degraded, not failed |
| `Raspberry Pi 5*` | Known good, no action |
| anything else | Warn "Untested product", continue |

**Postconditions**: `docker --version` matches the pattern; the `docker` service is enabled and
running; `docker run hello-world` succeeds, or the run is reported degraded with remediation on
ODROID hardware lacking the boot argument.

**Receipts produced**: one per removed conflict, one per installed package, one for `enable`, one
for `start`.

### Scenario 2 — Upgrade Docker on Linux.Debian

**Precondition**: Scenario 1 has run; a trace exists supplying the from-state.

**Prepare**: `plan.pkg.update()` — refresh the index only. The repository and keyring are already
correct, and re-writing them is Scenario 1's job.

**Install**: `plan.pkg.upgrade([...])` over the same six packages.

**Verify**: `plan.pkg.version_gte("docker-ce", <from-state version>)` — the assertion that
distinguishes an upgrade from a no-op, using the prior receipt's recorded version.

**Postconditions**: installed version is greater than or equal to the recorded from-state version;
the service is still enabled and running.

**Note on scope**: the bash has no upgrade path for Docker specifically. Its upgrade is
`apt-get full-upgrade --yes` across the whole machine (`Initialize-Debian:120`). This scenario is
therefore an *addition* to what the bash does, and is the first place the package is more precise
than its source.

### Scenario 3 — Decommission Docker on Linux.Debian

**Precondition**: Scenario 1 has run and its receipts are durable.

This scenario is the reason receipts matter: it should require **no teardown script**. Compensation
in reverse receipt order gives `service.stop`, `service.disable`, then `pkg.remove` of exactly the
packages that were installed — and critically, it does **not** reinstall the conflicting packages
that prepare removed, unless those removals recorded receipts that say to.

**Open**: whether conflict removal in prepare should be compensable. Reinstating `docker.io` on
decommission is arguably correct restoration and arguably unwanted. Decide before writing prepare.

**`purge-data` feature**: when enabled, additionally remove `/var/lib/docker` and
`/var/lib/containerd`. Not compensable — this is the one deliberately irreversible step, and must
be declared as such.

**Postconditions**: `docker` is absent from `PATH`; the service unit no longer exists; with
`purge-data`, the data directories are gone; without it, they remain.

### Scenario 4 — Deploy Docker on Darwin

**Precondition**: macOS with MacPorts or Homebrew present.

**The development Mac, as measured 2026-08-26**:

| Fact | Value |
| --- | --- |
| `orbctl` | `/usr/local/bin/orbctl` → `/Applications/OrbStack.app/Contents/MacOS/bin/orbctl` |
| `docker` | `/usr/local/bin/docker` → `/Applications/OrbStack.app/Contents/MacOS/xbin/docker` |
| Live daemon | OrbStack — `docker info` reports server 29.4.0, name `orbstack` |
| `/Applications/Docker.app` | **Present**, Docker Desktop 4.37.0, dated 2024-10-30, dormant |
| Homebrew | `/opt/homebrew/bin/brew`; cask `orbstack` 2.2.3, `auto_updates`, installed 2026-08-11 |
| MacPorts | `/opt/local/bin/port` — both managers present, so the port-first rule applies here |

Two consequences the scenario has to account for.

**Binary presence is the wrong predicate.** `command -v docker` succeeds, but it resolves into
OrbStack.app; and `/Applications/Docker.app` is present while serving nothing. The bash's three-way
`(colima || orbctl || Docker.app)` test therefore reports "a runtime exists" on the strength of a
dormant 2024 application. The honest predicate is **does a docker endpoint answer** — `docker info`
succeeding — which `Install-Dependencies` already runs after starting Colima, just not as the gate.

**Removing OrbStack is not sufficient to reach the install branch.** After
`brew uninstall --cask orbstack`, the `/usr/local/bin` symlinks dangle and Docker Desktop 4.37.0
remains in `/Applications`, so a directory-presence test still fires. Reaching the Colima branch on
this machine means clearing Docker.app as well — and Docker.app is **not** a Homebrew cask (only
`orbstack` is), so it came from Docker's own installer and needs its own removal.

**Prepare**: assert a package manager exists. The bash errors with install instructions for both
when neither is found; the package should fail with the same actionable message.

**Install** — the branch that makes this scenario different from every other:

```
if not (colima present or orbctl present or /Applications/Docker.app present):
    plan.pkg.install(["colima", "docker"])   # port first, brew fallback
```

The package **ensures a runtime exists**. It never displaces an installed Docker Desktop or
OrbStack, because the whole point is to avoid putting a licensed product on the machine, not to
prefer Colima on its merits.

**Provision**: `plan.shell.exec("colima start …")` — only on the branch where Colima was installed.
The flags are tribal knowledge worth encoding: `--vm-type vz --mount-type virtiofs` for file-sharing
performance and `--vz-rosetta` for x86 images on Apple Silicon. The bash currently passes only
`--cpu 2 --memory 4`; adding the others is a deliberate improvement, flagged here as such.

There is no `plan.service.*` on this path. Colima is a foreground-launched VM manager, not a launchd
service under the name `docker`.

**Postconditions**: `docker info` succeeds; `docker run --rm hello-world` succeeds.

### Scenario 5 — Upgrade and Decommission on Darwin

**Upgrade**: `plan.pkg.upgrade(["colima", "docker"])`, then `colima stop && colima start` to pick up
the new binary.

**Decommission**: asymmetric by construction. Compensation removes only what deploy installed —
so a machine where Docker Desktop was already present is left exactly as found, and a machine where
Colima was installed gets `colima stop`, `colima delete`, and `pkg.remove`. This asymmetry needs no
feature flag; it falls out of the receipt boundary.

### Corrections from `cmd/internal/lorepackage/lifecycle.go`, 2026-08-27

Reading the manifest struct before writing the manifest falsified three things above. They are left
visible rather than silently patched.

1. **Reconcile *is* package-authored.** `Reconcile Action = "Reconcile"` is a first-class action and
   `ReconcilePhaseOrder = []string{"scan", "repair", "verify"}` exists. A package can contribute
   `Reconcile/scan.star`, `Reconcile/repair.star`, and `Reconcile/verify.star`. Scenario 6 below is
   wrong about the package surface — what remains unimplemented is the engine-side convergence loop
   (`5.1`'s `ExecutionEvent` / `Reconcile` triangle), not the authoring surface.
2. **Upgrade phases are `prepare`, `upgrade`, `migrate`, `verify`** — not `install`. The discarded
   package's `Upgrade/install.star` names a phase that does not exist, and `migrate` (version-specific
   config or data migration) was entirely unknown to this plan. Scenario 2 needs rewriting against
   the real order.
3. **`verify` is a real phase** in the Deploy, Upgrade, and Reconcile orders. This plan earlier
   concluded "there is no `verify.star`" — wrong. What does not exist is the `plan.verify()`
   builtin. The phase is ordinary graph work, and `lifecycle.yaml`'s `verification:` block is a
   separate, additional mechanism.

All other manifest fields the discarded package used — `hardware_provisions`, `features`,
`settings`, `notes`, `tags`, `aliases`, `signatures` — are parsed. That part was not fiction.

### Scenario 6 — Reconcile: why this one is not proven here

**Superseded by correction 1 above — the premise of this section is wrong.** Reconcile is
package-authored after all.

The predicates exist and are sufficient to *detect* drift: `plan.pkg.installed`,
`plan.pkg.version_gte`, `plan.pkg.observe`, `plan.service.enabled`, `plan.service.running`.
`docs/architecture/5.1-reconciliation.md` records that `writ status` with Etag/Digest drift
attribution landed in steps 47–48, but that the `ExecutionEvent` / `Reconcile` triangle is
"Not implemented — preserved as design."

So drift detection exists; the convergence loop does not. Proving reconcile is engine work, not a
package. What this package contributes is the observable surface a convergence loop would read.

## Implementation Phases

### Phase 1: Remove the old package

- [ ] User deletes `devlore-registry/packages/docker/` in the registry worktree
- [ ] Confirm nothing else references it (`packages/cross-reference.yaml`, `INDEX.yaml`)

### Phase 2: lifecycle.yaml

- [ ] Rewrite from the bash: signatures, conflicts, features (`purge-data` only), settings,
      verification block
- [ ] Drop `rootless`; drop platforms not in this pass

### Phase 3: Darwin

Darwin goes first. It is the machine under the author's hands, so every scenario is runnable the
moment it is written, and the runtime-detection branch is exercised immediately (see Scenario 4).

- [ ] `Deploy/prepare.star`, `Deploy/install.star`, `Deploy/provision.star`
- [ ] `Upgrade/*`

### Phase 4: Prove Darwin

- [ ] Scenario 4 on the development Mac — detection branch, which is the only branch that machine
      can take while OrbStack is installed
- [ ] Scenario 4 install branch on a Mac with no runtime, or with detection forced
- [ ] Scenario 5
- [ ] Record what each scenario actually did, and correct this document

### Phase 5: Linux.Debian

- [ ] `Deploy/prepare.star`, `Deploy/install.star`, `Deploy/provision.star`
- [ ] `Upgrade/prepare.star`, `Upgrade/install.star`
- [ ] Decommission: prove it needs no scripts, or write the minimum that compensation cannot express

### Phase 6: Prove Linux.Debian

- [ ] Scenarios 1–3 against a disposable Debian target
- [ ] Record what each scenario actually did, and correct this document

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `devlore-registry/packages/docker/**` | Delete | Clean slate; user performs the deletion |
| `devlore-registry/packages/docker/lifecycle.yaml` | Create | Manifest rewritten from the bash |
| `devlore-registry/packages/docker/README.md` | Create | Tribal knowledge, sourced and dated |
| `devlore-registry/packages/docker/Linux.Debian/**` | Create | Deploy and Upgrade phases |
| `devlore-registry/packages/docker/Darwin/**` | Create | Deploy and Upgrade phases |

## Related Documents

- [Demo milestone](./extract-starlark-from-op/demo-milestone.md) — Scenario 1 there predates this
  work and describes the discarded DMG approach; it needs correcting
- [Reconciliation](../architecture/5.1-reconciliation.md) — why Scenario 6 is blocked
- [The pipeline](../guides/lore/pipeline.md) — the four-phase model; its Docker examples use the
  broken `plan.package.*` API and need correcting
- `devlore-registry/AUTHORING.md` — the ABNF for package layout, including the unused `Common` and
  `Unix` platform tokens

## Open Questions

- [ ] **How is any of this actually run?** Every "prove it" phase assumes `lore deploy docker`
      resolves a package out of devlore-registry and executes it. `lore` is characterized as mostly
      untested, and the milestone's criterion 5 (sealed graph plus lore consumer migration) was ⬜
      as of 2026-06-02. If `lore deploy` does not yet run end to end, the scripts can be written and
      reviewed but no scenario can be *proven*, and that is a prerequisite this plan does not own.
- [ ] Does `lifecycle.yaml`'s `verification:` block actually drive receipt verify status? The
      struct exists at `cmd/internal/lorepackage/lifecycle.go:61` and the milestone describes the
      behavior, but the consuming code path is unread. The plan currently assumes it.
- [ ] Should MacPorts precede Homebrew on Darwin (`pkg/platform/defaults.go:73`)? Ruling 7 leaves
      the router's brew default in place, which diverges from `Install-Dependencies`.
- [ ] How is Scenario 4's install branch proven, given OrbStack occupies the development Mac? A
      second Mac, a VM, or a supported override that makes runtime detection miss? Note that
      removing OrbStack alone is insufficient — Docker Desktop 4.37.0 remains in `/Applications`.
- [ ] Should conflict removal in prepare be compensable, reinstating `docker.io` on decommission?
- [ ] Does `Common/` or `Unix/` reduce duplication once Fedora and Windows return, or does the
      per-platform split stay?
- [ ] Where does the ODROID degraded outcome attach — which flow-control construct does
      `test_flow_degraded.star` demonstrate, and is it reachable from a phase script?
- [ ] Does `plan.pkg.remove` on an absent package fail, or is the `dpkg-query` guard's job already
      done by the router?
