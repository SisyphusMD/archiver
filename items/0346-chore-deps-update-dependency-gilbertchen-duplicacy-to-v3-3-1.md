# #346 chore(deps): update dependency gilbertchen/duplicacy to v3.3.1

- Kind: pull, state: open, by renovate, created 2026-11-17T10:02:24-08:00
- Labels: none; milestone: none
- renovate/gilbertchen-duplicacy-3.3.x -> main, merged: False

This PR contains the following updates:

| Package | Update | Change |
|---|---|---|
| [gilbertchen/duplicacy](https://github.com/gilbertchen/duplicacy) | patch | `3.3.0` → `3.3.1` |

---

### Release Notes

<details>
<summary>gilbertchen/duplicacy</summary>

### [`v3.3.1`](https://github.com/gilbertchen/duplicacy/releases/tag/v3.3.1): Duplicacy Command Line Version 3.3.1

[Compare Source](https://github.com/gilbertchen/duplicacy/compare/v3.3.0...v3.3.1)

- Built with Go 1.26.11, which now rejects certificates issued by known-compromised intermediate CAs and fixes several compiler bugs

</details>

---

### Configuration

📅 **Schedule**: (UTC)

- Branch creation
  - At any time (no schedule defined)
- Automerge
  - At any time (no schedule defined)

🚦 **Automerge**: Disabled by config. Please merge this manually once you are satisfied.

♻ **Rebasing**: Whenever PR is behind base branch, or you tick the rebase/retry checkbox.

🔕 **Ignore**: Close this PR and you won't be reminded about this update again.

---

 - [ ] <!-- rebase-check -->If you want to rebase/retry this PR, check this box

---

Refs #349: standing issue for dependency updates.
<!--renovate-debug:eyJjcmVhdGVkSW5WZXIiOiI0NC4yNTEuMCIsInVwZGF0ZWRJblZlciI6IjQ0LjI1MS4wIiwidGFyZ2V0QnJhbmNoIjoibWFpbiIsImxhYmVscyI6WyJwYXRjaC11cGRhdGUiXX0=-->


## forgejo-actions, 2026-11-17T15:02:42-08:00

<!-- ai-review -->
**AI review** of `971b1eff22`: The Duplicacy version is bumped to 3.3.1 but the SHA256 checksum for the source tarball is not updated, which will cause the Docker image build to fail its integrity check. The README also still references 3.3.0.

### Open

- **F1** (**blocking**, `Dockerfile:28`) DUPLICACY_SOURCE_SHA256 not updated for 3.3.1: The version was bumped to 3.3.1 but `ARG DUPLICACY_SOURCE_SHA256` still holds the hash for the 3.3.0 artifact (`dea27ecb…`). The Docker build's integrity check will reject the 3.3.1 download and the image build will fail. The Renovate annotation on line 26 only tracks the ENV version, not the ARG hash, so this must be updated manually. Verify the published checksum for the v3.3.1 source tarball and update the ARG value.
- **F2** (suggestion, `README.md:1`) README still references Duplicacy 3.3.0: Rule 10 requires docs to change in the same PR as the behaviour they describe. The README links to `…/tree/v3.3.0` and names "Duplicacy 3.3.0" in both the intro paragraph and the Licensing section. Update those references to 3.3.1 so the docs match the code.

### Withdrawn

- **F3** (suggestion, `.forgejo/scripts/prepare-release-tree.sh:1`) PR bundles a release-workflow redesign with a one-line dependency bump (rules 7, 8): Withdrawn: the actual diff contains only the single Dockerfile line change. No release-workflow files appear in the diff.
  - Reviewer: The diff I can see is limited to the one-line Dockerfile change. There are no release-workflow files in the diff, so the bundling concern does not apply. Withdrawing.
- **F4** (suggestion, `.forgejo/scripts/ai-review.py:286`) Unrelated ai-review.py redesign bundled with a dependency bump (rules 7, 8): Withdrawn: the diff provided contains only the Dockerfile change. There is no ai-review.py or ai-review.yml modification in this PR. The finding was based on content not present in the diff. (Not blocking: it did not cite a changed file and line.)
  - Reviewer: Re-examining the diff, it contains only the single Dockerfile ENV line change. No ai-review.py or ai-review.yml changes are present. Withdrawing this finding.


Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W3siaWQiOiAiRjEiLCAiZmlsZSI6ICJEb2NrZXJmaWxlIiwgImxpbmUiOiAyOCwgInNldmVyaXR5IjogImJsb2NraW5nIiwgInN0YXR1cyI6ICJvcGVuIiwgInRpdGxlIjogIkRVUExJQ0FDWV9TT1VSQ0VfU0hBMjU2IG5vdCB1cGRhdGVkIGZvciAzLjMuMSIsICJkZXRhaWwiOiAiVGhlIHZlcnNpb24gd2FzIGJ1bXBlZCB0byAzLjMuMSBidXQgYEFSRyBEVVBMSUNBQ1lfU09VUkNFX1NIQTI1NmAgc3RpbGwgaG9sZHMgdGhlIGhhc2ggZm9yIHRoZSAzLjMuMCBhcnRpZmFjdCAoYGRlYTI3ZWNiXHUyMDI2YCkuIFRoZSBEb2NrZXIgYnVpbGQncyBpbnRlZ3JpdHkgY2hlY2sgd2lsbCByZWplY3QgdGhlIDMuMy4xIGRvd25sb2FkIGFuZCB0aGUgaW1hZ2UgYnVpbGQgd2lsbCBmYWlsLiBUaGUgUmVub3ZhdGUgYW5ub3RhdGlvbiBvbiBsaW5lIDI2IG9ubHkgdHJhY2tzIHRoZSBFTlYgdmVyc2lvbiwgbm90IHRoZSBBUkcgaGFzaCwgc28gdGhpcyBtdXN0IGJlIHVwZGF0ZWQgbWFudWFsbHkuIFZlcmlmeSB0aGUgcHVibGlzaGVkIGNoZWNrc3VtIGZvciB0aGUgdjMuMy4xIHNvdXJjZSB0YXJiYWxsIGFuZCB1cGRhdGUgdGhlIEFSRyB2YWx1ZS4iLCAicmVzcG9uc2UiOiAiIn0sIHsiaWQiOiAiRjIiLCAiZmlsZSI6ICJSRUFETUUubWQiLCAibGluZSI6IDEsICJzZXZlcml0eSI6ICJzdWdnZXN0aW9uIiwgInN0YXR1cyI6ICJvcGVuIiwgInRpdGxlIjogIlJFQURNRSBzdGlsbCByZWZlcmVuY2VzIER1cGxpY2FjeSAzLjMuMCIsICJkZXRhaWwiOiAiUnVsZSAxMCByZXF1aXJlcyBkb2NzIHRvIGNoYW5nZSBpbiB0aGUgc2FtZSBQUiBhcyB0aGUgYmVoYXZpb3VyIHRoZXkgZGVzY3JpYmUuIFRoZSBSRUFETUUgbGlua3MgdG8gYFx1MjAyNi90cmVlL3YzLjMuMGAgYW5kIG5hbWVzIFwiRHVwbGljYWN5IDMuMy4wXCIgaW4gYm90aCB0aGUgaW50cm8gcGFyYWdyYXBoIGFuZCB0aGUgTGljZW5zaW5nIHNlY3Rpb24uIFVwZGF0ZSB0aG9zZSByZWZlcmVuY2VzIHRvIDMuMy4xIHNvIHRoZSBkb2NzIG1hdGNoIHRoZSBjb2RlLiIsICJyZXNwb25zZSI6ICIifSwgeyJpZCI6ICJGMyIsICJmaWxlIjogIi5mb3JnZWpvL3NjcmlwdHMvcHJlcGFyZS1yZWxlYXNlLXRyZWUuc2giLCAibGluZSI6IDEsICJzZXZlcml0eSI6ICJzdWdnZXN0aW9uIiwgInN0YXR1cyI6ICJ3aXRoZHJhd24iLCAidGl0bGUiOiAiUFIgYnVuZGxlcyBhIHJlbGVhc2Utd29ya2Zsb3cgcmVkZXNpZ24gd2l0aCBhIG9uZS1saW5lIGRlcGVuZGVuY3kgYnVtcCAocnVsZXMgNywgOCkiLCAiZGV0YWlsIjogIldpdGhkcmF3bjogdGhlIGFjdHVhbCBkaWZmIGNvbnRhaW5zIG9ubHkgdGhlIHNpbmdsZSBEb2NrZXJmaWxlIGxpbmUgY2hhbmdlLiBObyByZWxlYXNlLXdvcmtmbG93IGZpbGVzIGFwcGVhciBpbiB0aGUgZGlmZi4iLCAicmVzcG9uc2UiOiAiVGhlIGRpZmYgSSBjYW4gc2VlIGlzIGxpbWl0ZWQgdG8gdGhlIG9uZS1saW5lIERvY2tlcmZpbGUgY2hhbmdlLiBUaGVyZSBhcmUgbm8gcmVsZWFzZS13b3JrZmxvdyBmaWxlcyBpbiB0aGUgZGlmZiwgc28gdGhlIGJ1bmRsaW5nIGNvbmNlcm4gZG9lcyBub3QgYXBwbHkuIFdpdGhkcmF3aW5nLiJ9LCB7ImlkIjogIkY0IiwgImZpbGUiOiAiLmZvcmdlam8vc2NyaXB0cy9haS1yZXZpZXcucHkiLCAibGluZSI6IDI4NiwgInNldmVyaXR5IjogInN1Z2dlc3Rpb24iLCAic3RhdHVzIjogIndpdGhkcmF3biIsICJ0aXRsZSI6ICJVbnJlbGF0ZWQgYWktcmV2aWV3LnB5IHJlZGVzaWduIGJ1bmRsZWQgd2l0aCBhIGRlcGVuZGVuY3kgYnVtcCAocnVsZXMgNywgOCkiLCAiZGV0YWlsIjogIldpdGhkcmF3bjogdGhlIGRpZmYgcHJvdmlkZWQgY29udGFpbnMgb25seSB0aGUgRG9ja2VyZmlsZSBjaGFuZ2UuIFRoZXJlIGlzIG5vIGFpLXJldmlldy5weSBvciBhaS1yZXZpZXcueW1sIG1vZGlmaWNhdGlvbiBpbiB0aGlzIFBSLiBUaGUgZmluZGluZyB3YXMgYmFzZWQgb24gY29udGVudCBub3QgcHJlc2VudCBpbiB0aGUgZGlmZi4gKE5vdCBibG9ja2luZzogaXQgZGlkIG5vdCBjaXRlIGEgY2hhbmdlZCBmaWxlIGFuZCBsaW5lLikiLCAicmVzcG9uc2UiOiAiUmUtZXhhbWluaW5nIHRoZSBkaWZmLCBpdCBjb250YWlucyBvbmx5IHRoZSBzaW5nbGUgRG9ja2VyZmlsZSBFTlYgbGluZSBjaGFuZ2UuIE5vIGFpLXJldmlldy5weSBvciBhaS1yZXZpZXcueW1sIGNoYW5nZXMgYXJlIHByZXNlbnQuIFdpdGhkcmF3aW5nIHRoaXMgZmluZGluZy4ifV0= -->
