---
title: "signing options — backends, envelope, and a no-cloud default"
status: draft
created: 2026-06-11
updated: 2026-06-11
---

# Signing options for `pkg/signing`

## Summary

Two requirements drove this note: **flexibility** (the signing mechanism must be pluggable, not hard-wired to one
cloud) and a **good default that needs no cloud account**. Both are now **settled**:

- **Publisher-identity model, not an envelope.** Signing establishes **integrity** (a signature over the canonical
  bytes) and **publisher authenticity** (the signer's key, resolved against a verifier-side trust list). The signature
  is a detached **`op.Signature` field inside the document** — *not* DSSE/JWS/COSE.
- **No hash options.** `op.Signature.Algorithm` names the whole ciphersuite (key type + hash + signature); the digest
  is intrinsic to it.
- **Default backend: SSH-key signing** — reuse the developer's `~/.ssh/id_ed25519` (+ ssh-agent), git's
  `allowed_signers` trust model. Zero new key, zero cloud. Generated local Ed25519 keyfile as the fallback.
- **Everything normalizes to OpenSSH conventions** — `algorithm` = the SSH key-type name, `public_key` = the SSH wire
  blob, trust = `allowed_signers` — uniformly, even for cloud backends.
- **Sigstore is the opt-in custody provider** for the cloud, self-hosted, and keyless tiers — bolted on behind our
  interface, isolated from the default build.

This supersedes the single-mechanism framing in [`graph-signing.md`](graph-signing.md) (AWS KMS "Option 1"): KMS is
**one backend among several**, and SSH is the default.

## The flexible shape: one interface, many backends

The Go-native abstraction is sigstore's `pkg/signature.SignerVerifier`:

```go
type Signer interface {
	PublicKeyProvider
	SignMessage(message io.Reader, opts ...SignOption) ([]byte, error)
}
type Verifier interface {
	PublicKeyProvider
	VerifySignature(signature, message io.Reader, opts ...VerifyOption) error
}
type SignerVerifier interface { Signer; Verifier }
```

Local algorithms (Ed25519, ECDSA P-256/384/521, RSA) and every cloud KMS implement this **same** interface, so the
graph/trace signing path is written once and the backend is a config choice. We adopt this *shape* for `pkg/signing`
whether or not we depend on the sigstore module itself (see "Sigstore due diligence" for the dependency call).

## The signature field — settled

`op.Signature` (`pkg/op/signature.go`) stays **three fields, unchanged**:

| Field | Holds | Notes |
|---|---|---|
| `algorithm` | the full ciphersuite, e.g. `ssh-ed25519` | implies the hash (no `hash` field) and the key format |
| `value` | the raw signature | over `namespace ‖ CanonicalContent` (`pkg/op/graph.go:490`) |
| `public_key` | the publisher key, OpenSSH wire-format | what the verifier's `allowed_signers` keys on |

Deliberately **not** in the field: no hash field (subsumed by `algorithm`); no envelope; no trust list (verifier-side);
no human-identity string (an unauthenticated claim — identity comes from the verifier's mapping); no namespace field
(a fixed protocol constant, signed but not stored). `value` is a **raw** signature, not an opaque SSHSIG blob — we own
verify and reuse only the OpenSSH `allowed_signers` *file format* for the trust list.

## Support matrix

`signing.backend` selects; the selected backend's keys live under `signing:` and reach `pkg/signing` via
`application.Application.Config`. Cloud credentials come from each provider's default chain (env/profile/IAM), never
from this config. Cloud-backend `key` values use sigstore's KMS URI scheme.

| Backend (custody) | `algorithm` | Cloud? | Powered by |
|---|---|---|---|
| **SSH keyfile** — *default* | `ssh-ed25519` (default), `ecdsa-sha2-nistp256/384`, `rsa-sha2-256/512` | no | stdlib + `x/crypto/ssh` |
| **ssh-agent** (incl. FIDO/PIV hardware) | above + `sk-ssh-ed25519@…` | no | `x/crypto/ssh/agent` |
| **Generated local key** — *fallback* | `ssh-ed25519` | no | stdlib `crypto/ed25519` |
| **AWS KMS** | `ecdsa-sha2-nistp256`, `rsa-sha2-*` | yes | sigstore `…/kms/aws` |
| **GCP KMS** | `ecdsa-…`, `rsa-…` | yes | sigstore `…/kms/gcp` |
| **Azure Key Vault** | `ecdsa-…`, `rsa-…` | yes | sigstore `…/kms/azure` |
| **HashiCorp Vault / OpenBao** | `ed25519`, `ecdsa-…`, `rsa-…` | self-host | sigstore `…/kms/hashivault` |
| **Sigstore keyless** | `ecdsa-sha2-nistp256` (ephemeral) | yes (OIDC) | sigstore-go |

(GPG and minisign were evaluated and **not** adopted: GPG is heavy and declining; minisign is redundant with the SSH
default. PKCS#11/TPM hardware is reached *through* ssh-agent as FIDO/PIV keys.)

### Sample `signing:` configuration

Every backend takes the same three common settings first — **`backend`**, **`key`**, **`allowed_signers`**. The
self-hosted and keyless backends add a few more *after* them.

```yaml
# SSH keyfile — the default
signing:
  backend: ssh
  key: ~/.ssh/id_ed25519
  allowed_signers: ~/.config/devlore/allowed_signers
```

```yaml
# ssh-agent — hardware / FIDO / PIV keys held by the agent
signing:
  backend: ssh
  key: "SHA256:abc123…"                 # which agent identity to use
  allowed_signers: ~/.config/devlore/allowed_signers
  use_agent: true
```

```yaml
# Generated local key — the no-SSH fallback
signing:
  backend: local
  key: ~/.config/devlore/signing/ed25519
  allowed_signers: ~/.config/devlore/allowed_signers
```

```yaml
# Cloud KMS — AWS / GCP / Azure (common settings only; creds from the provider's default chain)
signing:
  backend: kms
  key: awskms:///arn:aws:kms:us-east-1:123456789012:key/abcd-1234
  allowed_signers: ~/.config/devlore/allowed_signers
  # GCP:   key: gcpkms://projects/P/locations/L/keyRings/R/cryptoKeys/K/cryptoKeyVersions/1
  # Azure: key: azurekms://my-vault.vault.azure.net/keys/my-key
```

```yaml
# HashiCorp Vault / OpenBao — extras after the common three
signing:
  backend: kms
  key: hashivault://devlore-signing
  allowed_signers: ~/.config/devlore/allowed_signers
  address: https://vault.example.com:8200   # or $VAULT_ADDR
  mount: transit                            # transit secrets-engine mount; token from $VAULT_TOKEN
```

```yaml
# Sigstore keyless — no stable key or allowed_signers; OIDC + Fulcio + Rekor instead
signing:
  backend: keyless
  oidc_issuer: https://oauth2.sigstore.dev/auth
  identity: releases@noblefactor.com
  fulcio_url: https://fulcio.sigstore.dev
  rekor_url: https://rekor.sigstore.dev
```

(`key`/`allowed_signers` don't apply to keyless — its trust is the OIDC identity + Fulcio root + Rekor log, the one
backend that diverges from the common model.)

## The us-vs-sigstore split (the two refinements)

1. **Sigstore provides the *sign operation*; we own everything around it.** For a KMS backend, sigstore does the `Sign`
   call, but *our* code defines the `op.Signature` field, **re-encodes** the returned key/signature into OpenSSH wire
   conventions, and runs the `allowed_signers` trust check. The envelope, the normalization, and the trust model are
   uniformly ours across *all* backends; sigstore is just the custody/sign primitive for the external tier.
2. **Keyless is the seam.** Sigstore handles keyless signing, but it **won't normalize into the SSH/`allowed_signers`
   model** — its trust is an OIDC identity + Fulcio root + Rekor log, a separate verification path. Mechanically in
   sigstore's lane, but it doesn't ride the uniform trust model — the one backend that fights the design. Include it
   only if public-transparency-log provenance is a real goal.

## Envelope & hash — resolved

- **Envelope: none.** Earlier this note leaned DSSE. With the publisher-identity model settled, there is **no envelope**
  — the signature is a detached `op.Signature` field inside the artifact (graphs/traces already carry it). DSSE wraps a
  payload and has, by spec, no identity/trust model, so it answers neither goal. It would matter only if graphs/traces
  had to interoperate as **SLSA/in-toto attestations** — a different goal we are not pursuing.
- **Hash: no options.** The digest is intrinsic to `algorithm` (Ed25519 ⟹ SHA-512; ECDSA-P256 ⟹ SHA-256; the SSH
  key-type names state it). A negotiable hash buys no agility (you'd version the *scheme*) and is a downgrade-attack
  surface — the "no crypto knobs" discipline of Ed25519/age/minisign.
- **SSHSIG vs raw:** `value` is a **raw** signature (decomposed), not an opaque SSHSIG blob — so we own verify and keep
  the clean 3-field record, reusing only OpenSSH's `allowed_signers` *file format* for the verifier's trust list. The
  cost is no shell `ssh-keygen -Y verify` interop; verification is `devlore`'s own.

## Sigstore due diligence

The user asked: who authors it, what license, what adoption, can we copy the code, at what cost.

- **Who / governance.** Conceived and prototyped at **Red Hat** (Luke Hinds, founder; Dan Lorenc co-creator at Google;
  Bob Callaway at Google; Santiago Torres-Arias at Purdue). Now an **OpenSSF (Open Source Security Foundation)
  *graduated* project** under the **Linux Foundation** — founding members Red Hat, Google, Purdue; 70+ organizations
  involved (Chainguard, GitHub, Shopify, Autodesk, Trail of Bits, …). It is a **foundation-governed, multi-vendor**
  project, not a single-company library.
- **License.** **Apache-2.0** (the `sigstore/sigstore` Go library and the cosign tooling). Permissive; ships a
  COPYRIGHT/NOTICE.
- **Adoption.** De-facto supply-chain signing standard: **Kubernetes** (signs releases since 2022), **npm** provenance
  (GA 2025), **PyPI** attestations (GA 2024), **Maven Central** (2025), **Homebrew** (~7000 bottles via GitHub artifact
  attestations), **GitHub Actions** artifact attestations. Very high. GitHub footprint: org **~1.6k followers**;
  `cosign` **6.0k★**, `rekor` 1.2k★, `fulcio` 851★ (the `sigstore/sigstore` *library* repo is a modest ~524★ — it is
  shared plumbing, not the headline tool).
- **Maintenance.** Actively maintained — latest `sigstore/sigstore` is **v1.10.8 (2026-05-29)**, on a steady recent
  cadence (v1.10.5 Mar 19 → v1.10.6 May 4 → v1.10.7 May 28 → v1.10.8 May 29, 2026). Repo:
  <https://github.com/sigstore/sigstore>. **Security note:** advisory **GO-2026-4358** (arbitrary file write via cache
  path traversal) affects the module's **legacy TUF client** (v1.10.3+) — **not** `pkg/signature`. Another reason to
  import only the signing package and steer clear of the TUF client.
- **Can we copy / depend? At what cost.**
  - **License-wise: yes.** Apache-2.0 permits import, vendoring, and copying, provided we retain the license +
    copyright/NOTICE and note modifications. *Caveat:* this repo is **SSPL-1.0**; Apache-2.0 as an imported/vendored
    **dependency** is standard and compatible, but copied source files must keep their Apache-2.0 headers/NOTICE — run
    this past whoever owns licensing before vendoring source (not just importing).
  - **Cost is dependency weight, not license.** `sigstore/sigstore`'s `pkg/signature/kms/*` transitively drags the AWS
    SDK, Azure SDK, GCP SDK, k8s `client-go`, Docker internals, and OpenTelemetry; cosign's binary is **100MB+**.
    `sigstore-go` is lighter (~60MB) but **omits KMS** and is verification-focused. The **core `pkg/signature`** (local
    ecdsa/ed25519/rsa) is comparatively light; the bloat comes from the KMS subpackages.
  - **Mitigation (recommended).** Do **not** take a blanket sigstore dependency:
    1. For the **no-cloud default**, use **stdlib `crypto/ed25519`** + **`go-securesystemslib/dsse`** + an **sshsig**
       library (`42wim/sshsig` or `SierraSoftworks/sshsign-go`) — all light, no sigstore.
    2. Define **our own** small `Signer`/`Verifier` interface (mirroring sigstore's *shape* — a common interface
       pattern, not meaningfully "copying").
    3. Pull in `sigstore/sigstore/pkg/signature/kms/<provider>` **only** inside the optional KMS backend, isolated
       (separate package, ideally a build constraint or sub-module) so the cloud SDK weight is **opt-in** and never in
       the default build.

## Go libraries

| Need | Library | Weight |
|---|---|---|
| Local Ed25519 default | `crypto/ed25519` (stdlib) | none |
| DSSE envelope | `github.com/secure-systems-lab/go-securesystemslib/dsse` | light |
| SSH (SSHSIG) signing/verify | `github.com/42wim/sshsig` or `SierraSoftworks/sshsign-go` | light |
| Pluggable interface + KMS | `github.com/sigstore/sigstore/pkg/signature` (+ `…/kms/<p>`) | core light; KMS heavy |
| Keyless (CI) | `github.com/sigstore/sigstore-go` | medium |

## Config mapping

Signing **owns** its `SigningSection` (a `devconfig.Section`) in **`pkg/signing`** — not centralized in the config
package (see [`configuration.md`](configuration.md)). It is composed under **`Defaults`** (shared, since all tools sign)
and overridable per app, riding the typed `devconfig.Config` the configuration redesign moves onto `Application`,
resolved through the standard `cli > env > config#<app> > config#Defaults > builtin` overlay. `pkg/signing` reads it via
`signing.SectionFrom(cfg)` off the `RuntimeEnvironment`'s `Application`; `pkg/op` never sees the concrete type.

```yaml
# the Signing ConfigSection: backend discriminator + the common three + per-backend extras
signing:
  backend: ssh | local | kms | keyless
  key: <path | KMS URI>
  allowed_signers: <path>
  # kms / vault / keyless extras as in the Support matrix above
```

> **Naming caution:** writ already has a `SigningKey *age.X25519Identity` (`cmd/writ/writ/graph_types.go:41`) — that's
> an **age encryption identity**, unrelated to graph-provenance signing. The new config must use a distinct name
> (`signing.*`) to avoid collision.

## The `allowed_signers` trust file

The verifier's trust list is **OpenSSH's `allowed_signers`** format — the same file `ssh-keygen -Y verify` uses and that
git reads for SSH commit verification (`gpg.ssh.allowedSignersFile`). It is **not** a sigstore artifact; `devlore`
**parses it itself** (consistent with the raw-signature / own-verify decision — no shelling out to `ssh-keygen`). It
lives entirely **verifier-side**; signed documents never reference it, so a team may already maintain one.

One entry per line, space-separated:

```text
<principals>  [options]  <keytype> <base64-key>  [comment]
```

- **principals** — comma-separated identities (emails/usernames); wildcards allowed (`*@noblefactor.com`); no spaces.
- **options** (optional, comma-separated) — `namespaces="…"`, `valid-after="…"`, `valid-before="…"`, `cert-authority`.
- **key** — `keytype base64-blob`.
- **comment** — optional, ignored.

Sample (`~/.config/devlore/allowed_signers`):

```text
# Verifier-side trust list: which publisher keys devlore will accept.
#   <principals>  [options]  <keytype> <base64-key>  [comment]

# A plain publisher key — trusted for any namespace:
releases@noblefactor.com  ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIrP8eoWfI+wFScOfZ8iKs8VxqEofZfJZuEKgVtuwV8O  release-bot

# Restricted to devlore's signing namespaces only:
ci@noblefactor.com  namespaces="devlore.graph.v1,devlore.trace.v1" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILk2lPq9b8…  ci-signer

# With a validity window:
david.noble@noblefactor.com  valid-after="20260101",valid-before="20270101" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI8fQzR…  david laptop

# A CA that vouches for many publisher keys (the SSH-cert model — scales past listing every key):
*@noblefactor.com  cert-authority,namespaces="devlore.*" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItRuW…  noblefactor signing CA
```

Three properties this format buys us, each mapping to a decision already made:

- **`namespaces="devlore.graph.v1,…"`** is the signature's domain-separation namespace — a key can be scoped to devlore
  artifacts only, never reused from a git-commit key.
- **`cert-authority`** is the scaling path: trust a CA once and it issues SSH certs binding key→identity, instead of one
  line per publisher key.
- **wildcard principals + `valid-after`/`valid-before`** give rotation and revocation entirely verifier-side — no signed
  document changes.

For the KMS backends the published key lands here as an ordinary `ssh-ed25519` / `ecdsa-sha2-nistp256` line — the
"normalize to OpenSSH conventions" payoff: **one trust file regardless of where the private key lives**.

## Trust anchor, per backend

Verification pins a trust root **verifier-side**. Because every backend normalizes to OpenSSH conventions, that root is
the **`allowed_signers` file** (above) for all of them — the publisher's key is a line in it regardless of where the
private half lives:

- **SSH / local / KMS** → `allowed_signers` (the published key, optionally `namespaces`-scoped or CA-issued).
- **keyless** → the exception: Fulcio root + an OIDC identity policy (which identities are trusted) + Rekor inclusion.

## Open questions

1. SSHSIG vs DSSE envelope ((a) uniform DSSE vs (b) per-backend) — lean (a).
2. Whether to depend on `sigstore/sigstore` at all for KMS, or implement KMS backends directly against the cloud SDKs
   (skips the sigstore dep but loses the shared interface for KMS).
3. SSPL × Apache-2.0 vendoring sign-off (import is fine; copying source needs a licensing check).
4. Default trust-anchor bootstrap UX (first-run key generation / `allowed_signers` seeding).

## Recommendation

1. **SSH-key signing is the default**; generated local Ed25519 is the fallback — no cloud account.
2. `pkg/signing` is built on a **small in-house `Signer`/`Verifier`** interface, a **detached `op.Signature` field (no
   envelope)**, and **stdlib + light deps**; everything normalizes to OpenSSH conventions.
3. **KMS / keyless are opt-in backends** powered by sigstore, isolated so their cloud-SDK weight stays out of the
   default build.
4. Selection + per-backend config come from `signing.backend` in `Application.Config` (see the support matrix).

## References

- Sigstore — OpenSSF graduated project, Apache-2.0: <https://openssf.org/projects/sigstore/>,
  <https://blog.sigstore.dev/sigstore-openssf-graduation/>
- sigstore/sigstore `pkg/signature` (Go interface + algorithms): <https://pkg.go.dev/github.com/sigstore/sigstore/pkg/signature>
- sigstore-go (lighter, KMS-omitted): <https://github.com/sigstore/sigstore-go>
- cosign dependency-tree weight: <https://github.com/sigstore/cosign/issues/1462>
- SSH signing of arbitrary data (`ssh-keygen -Y`): <https://www.agwa.name/blog/post/ssh_signatures>
- sshsig (Go): <https://github.com/42wim/sshsig>
- minisign: <https://jedisct1.github.io/minisign/>
- DSSE + why-not-JWS: <https://github.com/secure-systems-lab/dsse>,
  <https://github.com/secure-systems-lab/dsse/blob/master/background.md>
- go-securesystemslib/dsse: <https://pkg.go.dev/github.com/secure-systems-lab/go-securesystemslib/dsse>
