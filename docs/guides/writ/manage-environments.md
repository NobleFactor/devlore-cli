---
title: "Manage Environments"
description: "Deploy, reconcile, upgrade, adopt, and decommission configuration projects"
tool: "writ"
category: "tutorial"
order: 2
---

# Manage Environments

This guide covers the lifecycle of an environment under writ: deploying
projects, handling occupied targets, reconciling the machine against the
record, upgrading copied files, adopting files you already have, and
decommissioning what writ deployed.

Every operation acts on **the record**: the receipts writ writes to its store
as it runs. A deploy replaces the record, an upgrade updates it, a
decommission removes what it names, and reconcile compares the machine
against it. The record is the desired state; your layer checkout is only
what a deploy reads.

## Deploy projects

A bare `writ deploy` deploys the implicit set: the reserved `common` project,
configuration that applies everywhere, plus one project per registered layer
repository, named for the repository. With the base layer registered from
`noblefactor-ops`, the team layer from `devlore-cli` and the personal layer from
`personal`, a bare deploy overlays `common`, `noblefactor-ops`, `devlore-cli`
and `personal` from every layer that carries them, each with its platform
variants. Naming a project adds it, and the record remembers it: a later bare
deploy keeps every project the current deployment put in place.

```bash
writ deploy
writ deploy noblefactor
writ deploy noblefactor thenobles
```

There is never anything to add for `common`, and a name no registered layer
carries is refused. The deploy says which projects were implicit, which the
record already held, and which you named.

Each `writ deploy` invocation is one deployment. Its receipts are the record
until the next deploy replaces it.

### Occupied targets

When a target already exists and is not what the record says writ wrote, the
`--conflict` policy decides. An occupant is writ's own when it matches the
record: a symlink whose literal endpoint is the recorded source, resolved or
dangling, or a file whose content is the recorded as-deployed content. A link
left dangling by a file that moved between layers is therefore still writ's
own, and a plain deploy re-points it.

```bash
# Refuse when a target is occupied, listing the occupants (the default)
writ deploy noblefactor

# Archive each occupant to the recovery site, then write; restorable
writ deploy --conflict=replace noblefactor

# Leave every occupied target untouched and continue
writ deploy --conflict=skip noblefactor
```

### Uncommitted changes

A deploy reads a layer at its committed state. When a layer's working tree
has uncommitted changes, deploy refuses so the record never names content git
does not have. To deploy anyway:

```bash
writ deploy --allow-dirty noblefactor
```

### Custom segments

Override platform detection with custom segment values (for example
`--segment ROLE=desktop`). See
[Platform Awareness](/guides/writ/platform-awareness/#custom-segments).

### Dry run

Preview what writ would do without making changes:

```bash
writ deploy --dry-run noblefactor
```

## Reconcile

Reconcile compares the machine against the record and reports every deployed
entry in one of six words. The report comes from the store, never from a
directory scan, so it lists what the current deployment put in place and
nothing else.

```bash
# Report everything the current deployment put in place
writ reconcile

# Report one project
writ reconcile noblefactor

# The report as JSON, and the drifted entries alone
writ reconcile -o json
writ reconcile -o json --jq '[.entries[] | select(.state != "linked" and .state != "copied")]'
```

The six words, and the command that repairs each:

| State | Meaning | Repair |
|-------|---------|--------|
| `linked` | The symlink is as recorded and its referent's content is as recorded | — |
| `copied` | The copied file is as recorded and its source's content is as recorded | — |
| `absent` | The record says a target is there and it is not | `writ deploy` |
| `changed` | The target is there but is not what the record says: not the recorded symlink, or a copy whose content moved | `writ deploy` for a link; `writ upgrade --force` for a copy |
| `dangling` | The source the record names does not resolve: a link whose referent is gone, or a copy whose source is gone | `writ deploy` |
| `stale` | The deployed file is as recorded, and its source's content has moved on | `writ upgrade` |

Reconcile reports and leaves the machine as it found it; you run the repair it
names.

### Exit status

The exit status is the answer, so a script can gate on it the way it gates on
`git diff --exit-code`:

| Exit | Answer |
|------|--------|
| `0` | Deployed and clean: every entry is `linked` or `copied` |
| `1` | Deployed and drifted: at least one entry is `absent`, `changed`, `dangling` or `stale`; the report says which |
| `66` | Never deployed: the store has no current deployment to compare against, because nothing has been deployed or it was decommissioned |

Reconcile is valid only after a deployment. Before one there is nothing to
compare against, and the answer is not-found rather than an empty report.

## Upgrade copied files

A symlink always points at its source and needs no upgrading. Copied files —
expanded templates and decrypted secrets — are what a deploy produced from a
source at the time, and when the source moves on, reconcile reports them
`stale`. Upgrade regenerates them:

```bash
# Regenerate every copied file
writ upgrade

# Regenerate one project's copied files
writ upgrade noblefactor

# Also regenerate copies you edited locally (reconcile reports them `changed`)
writ upgrade --force
```

Without `--force`, a copy that differs from what the record says is left
alone with a warning, since the difference may be your own edit.

## Adopt files you already have

Adopt moves a file you already have into a project directory and leaves a
symlink in its place, so it deploys like everything else. The project is
named by `--project`; the scope is inferred from the item's location — under
your home directory the item is adopted into `Home/`, elsewhere into
`System/`.

```bash
# Adopt a single file into the personal layer
writ adopt --project noblefactor ~/.zshrc

# Adopt several
writ adopt --project noblefactor ~/.zshrc ~/.bashrc ~/.config/nvim/init.lua

# Adopt a directory recursively
writ adopt --project noblefactor ~/.config/nvim

# Adopt into the team layer
writ adopt --layer team --project shared ~/.editorconfig

# Adopt a file that only Debian should get: it lands under
# noblefactor.Linux.Debian and deploys only there
writ adopt --project noblefactor --platform Linux.Debian ~/.config/apt.conf
```

`--platform` takes a suffix in the vocabulary the layer tree uses (`Darwin`,
`Unix`, `Windows`, `Linux`, `Linux.Debian`, `Darwin.arm64`, and so on); see
[Platform Awareness](/guides/writ/platform-awareness/). An adoption is
recorded like a deploy, so the adopted link is writ's own from then on and
reconcile reports it. Adopt therefore needs a current deployment to record
into; on a machine that has never deployed, deploy first.

### Adopt from a lore receipt

After installing software with lore, adopt the generated configuration:

```bash
writ adopt --from-receipt
writ adopt --from-receipt ~/.local/state/lore/receipts/2026-01-19T14:32:07.yaml
```

This reads the lore receipt and moves the packages manifest and any generated
configuration files into your environment repository.

## Decommission

Decommission removes what the record says a project put in place — nothing
more, because the inventory comes from the record, never from a directory
scan. Symlinks are unlinked; a target you replaced with a real file is refused,
not deleted. Copied files are archived to the recovery site before removal, so
the removal is restorable.

```bash
writ decommission noblefactor
writ decommission noblefactor thenobles

# Also remove parent directories the removal left empty
writ decommission --prune noblefactor
```

## Migrate an existing dotfiles repository

`writ migrate` analyzes an existing dotfiles repository — GNU Stow, chezmoi,
hand-written scripts — and produces a plan for bringing it into writ's layered
structure, using a language model for the classification:

```bash
# Produce the plan without changing anything
writ migrate --dry-run ~/dotfiles

# Review the plan, then run it: the layer directory becomes a symlink to
# ~/dotfiles (the default, --link)
writ migrate ~/dotfiles

# Or move the content into the layer directory and remove the source
writ migrate --move ~/dotfiles
```

### Choosing the model provider

By default, writ uses [Ollama](https://ollama.ai) for local inference. To use
a cloud provider, set the model flags or their environment variables:

```bash
# GitHub Models (free with a GitHub account)
DEVLORE_MODEL_PROVIDER=github DEVLORE_MODEL_API_KEY=$(gh auth token) \
  writ migrate ~/dotfiles

# Anthropic
writ --model-provider=anthropic --model-api-key=sk-... migrate ~/dotfiles
```

See [writ migrate](/cli/writ/migrate/) for every option.
