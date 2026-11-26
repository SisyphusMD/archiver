# #393 chore(deps): update ghcr.io/sigstore/cosign/cosign docker tag to v3.1.6

- Kind: pull, state: closed, by renovate, created 2026-11-25T14:02:08-08:00
- Labels: none; milestone: none
- refs/pull/393/head -> main, merged: True

This PR contains the following updates:

| Package | Update | Change |
|---|---|---|
| [ghcr.io/sigstore/cosign/cosign](https://github.com/sigstore/cosign) | patch | `v3.1.5` → `v3.1.6` |

---

### Release Notes

<details>
<summary>sigstore/cosign (ghcr.io/sigstore/cosign/cosign)</summary>

### [`v3.1.6`](https://github.com/sigstore/cosign/releases/tag/v3.1.6)

[Compare Source](https://github.com/sigstore/cosign/compare/v3.1.5...v3.1.6)

#### Changelog

- [`252f10a`](https://github.com/sigstore/cosign/commit/252f10a80ba871152a12e5ee324ad1e02c85d079) Have cosign verify require <https://sigstore.dev/cosign/sign/v1> predic… ([#&#8203;5253](https://github.com/sigstore/cosign/issues/5253))
- [`44238b7`](https://github.com/sigstore/cosign/commit/44238b7d2f0aa2c98b28498b430e05f2ee36dd78) Ensure that when we read local manifests, they match their claimed digest ([#&#8203;5249](https://github.com/sigstore/cosign/issues/5249))
- [`80408c2`](https://github.com/sigstore/cosign/commit/80408c26b036313146f241e8b2df515423a86bfb) Get timestamp bytes from payload for attestations ([#&#8203;5250](https://github.com/sigstore/cosign/issues/5250))
- [`73b95f6`](https://github.com/sigstore/cosign/commit/73b95f67cd77ffd12329d0273949289f48fa5285) Move old verifier to encoding/json/v2 for case sensitivity ([#&#8203;5252](https://github.com/sigstore/cosign/issues/5252))
- [`75e6e4c`](https://github.com/sigstore/cosign/commit/75e6e4cbaa4834d64a3e573100508bcc009055e8) Update sigstore-go to v1.4.1 ([#&#8203;5246](https://github.com/sigstore/cosign/issues/5246))

##### Thanks to all contributors!

</details>

---

### Configuration

📅 **Schedule**: (UTC)

- Branch creation
  - At any time (no schedule defined)
- Automerge
  - At any time (no schedule defined)

🚦 **Automerge**: Enabled.

♻ **Rebasing**: Whenever PR is behind base branch, or you tick the rebase/retry checkbox.

🔕 **Ignore**: Close this PR and you won't be reminded about this update again.

---

 - [ ] <!-- rebase-check -->If you want to rebase/retry this PR, check this box

---

Refs #349: standing issue for dependency updates.
<!--renovate-debug:eyJjcmVhdGVkSW5WZXIiOiI0NC4yNjAuMyIsInVwZGF0ZWRJblZlciI6IjQ0LjI2MC4zIiwidGFyZ2V0QnJhbmNoIjoibWFpbiIsImxhYmVscyI6WyJkb2NrZXIiLCJwYXRjaC11cGRhdGUiXX0=-->


## forgejo-actions, 2026-11-25T14:02:45-08:00

<!-- ai-review -->
**AI review** of `b85d267e51`: Single-line Renovate patch bump of the cosign Docker image (v3.1.5 → v3.1.6) in the publish workflow, pinned by digest, referencing the standing dependency-update issue #349. Clean and minimal.

No findings.

Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W10= -->
