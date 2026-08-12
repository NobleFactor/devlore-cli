# Contributing to DevLore

Thank you for your interest in DevLore.

## Licence and sign-off

DevLore is licensed under the [Apache License 2.0](LICENSE). Per §5 of the
licence, any contribution intentionally submitted for inclusion is under the
same terms, without additional terms or conditions.

Every commit must carry a
[Developer Certificate of Origin](https://developercertificate.org/) sign-off:

```bash
git commit -s
```

The `Signed-off-by:` line certifies that you wrote the contribution or have
the right to submit it under Apache-2.0.

## AI disclosure (required)

Contributions that include material generated with AI assistance are welcome,
**but the assistance must be disclosed**. AI is acknowledged, never credited:
no AI tool appears as an author or co-author. Add a trailer to each commit
where AI was used:

```text
Assisted-by: <tool name>
```

and say so in the pull request description. You remain responsible for having
the right to submit everything in the commit, disclosed or not.

**Undisclosed AI use is a bannable offence.** If a contribution is found to
contain AI-generated material that was not announced, the contributor is
permanently banned from the project. Disclosure costs one line; concealment
costs the maintainers the ability to trust every past and future submission
from that contributor — there is no proportionate lesser response.

## Workflow

1. Open or find an issue describing the change.
2. Branch from `develop`; open a pull request back to `develop`.
3. Make sure `make build` and `make test` pass, and run
   `star lint all` before pushing — the Go style rules are enforced by
   `star lint go-style` and documented in
   [docs/guides/go-style-guidelines.md](docs/guides/go-style-guidelines.md).
4. Squash merges only; keep commit messages in
   `type(scope): summary` form.

## Package contributions

Packages (deployment manifests and phase scripts) live in
[devlore-registry](https://github.com/NobleFactor/devlore-registry), which has
its own contribution terms — see its CONTRIBUTING.md.
