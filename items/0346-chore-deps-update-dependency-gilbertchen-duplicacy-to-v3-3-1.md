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
**AI review** of `224339f20c`: The Duplicacy version is bumped to 3.3.1 but the SHA256 integrity hash is not updated, which will cause the Docker build to fail. The README still references 3.3.0. The previously flagged ai-review.py changes are not present in this diff.

### Open

- **F1** (**blocking**, `Dockerfile:28`) DUPLICACY_SOURCE_SHA256 not updated for 3.3.1: The version was bumped to 3.3.1 but `ARG DUPLICACY_SOURCE_SHA256` still holds the hash for the 3.3.0 artifact (`dea27ecb…`). The Docker build's integrity check will reject the 3.3.1 download and the image build will fail. Update the hash to the value published for the v3.3.1 release.
- **F2** (suggestion, `README.md:1`) README still references Duplicacy 3.3.0: Rule 10 requires docs to change in the same PR as the behaviour they describe. The README links to `…/tree/v3.3.0` and names "Duplicacy 3.3.0" in both the intro paragraph and the Licensing section. Update those references to 3.3.1 so the docs match the code.

### Withdrawn

- **F3** (suggestion, `.forgejo/scripts/prepare-release-tree.sh:1`) PR bundles a release-workflow redesign with a one-line dependency bump (rules 7, 8): Withdrawn: the actual diff contains only the single Dockerfile line change. No release-workflow files appear in the diff.
  - Reviewer: The diff I can see is limited to the one-line Dockerfile change. There are no release-workflow files in the diff, so the bundling concern does not apply. Withdrawing.
- **F4** (suggestion, `.forgejo/scripts/ai-review.py:286`) Unrelated ai-review.py redesign bundled with a dependency bump (rules 7, 8): Withdrawn: the diff provided contains only the Dockerfile change. There is no ai-review.py or ai-review.yml modification in this PR. The finding was based on content not present in the diff. (Not blocking: it did not cite a changed file and line.)
  - Reviewer: Re-examining the diff, it contains only the single Dockerfile ENV line change. No ai-review.py or ai-review.yml changes are present. Withdrawing this finding.


Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W3siaWQiOiAiRjEiLCAiZmlsZSI6ICJEb2NrZXJmaWxlIiwgImxpbmUiOiAyOCwgInNldmVyaXR5IjogImJsb2NraW5nIiwgInN0YXR1cyI6ICJvcGVuIiwgInRpdGxlIjogIkRVUExJQ0FDWV9TT1VSQ0VfU0hBMjU2IG5vdCB1cGRhdGVkIGZvciAzLjMuMSIsICJkZXRhaWwiOiAiVGhlIHZlcnNpb24gd2FzIGJ1bXBlZCB0byAzLjMuMSBidXQgYEFSRyBEVVBMSUNBQ1lfU09VUkNFX1NIQTI1NmAgc3RpbGwgaG9sZHMgdGhlIGhhc2ggZm9yIHRoZSAzLjMuMCBhcnRpZmFjdCAoYGRlYTI3ZWNiXHUyMDI2YCkuIFRoZSBEb2NrZXIgYnVpbGQncyBpbnRlZ3JpdHkgY2hlY2sgd2lsbCByZWplY3QgdGhlIDMuMy4xIGRvd25sb2FkIGFuZCB0aGUgaW1hZ2UgYnVpbGQgd2lsbCBmYWlsLiBVcGRhdGUgdGhlIGhhc2ggdG8gdGhlIHZhbHVlIHB1Ymxpc2hlZCBmb3IgdGhlIHYzLjMuMSByZWxlYXNlLiIsICJyZXNwb25zZSI6ICIifSwgeyJpZCI6ICJGMiIsICJmaWxlIjogIlJFQURNRS5tZCIsICJsaW5lIjogMSwgInNldmVyaXR5IjogInN1Z2dlc3Rpb24iLCAic3RhdHVzIjogIm9wZW4iLCAidGl0bGUiOiAiUkVBRE1FIHN0aWxsIHJlZmVyZW5jZXMgRHVwbGljYWN5IDMuMy4wIiwgImRldGFpbCI6ICJSdWxlIDEwIHJlcXVpcmVzIGRvY3MgdG8gY2hhbmdlIGluIHRoZSBzYW1lIFBSIGFzIHRoZSBiZWhhdmlvdXIgdGhleSBkZXNjcmliZS4gVGhlIFJFQURNRSBsaW5rcyB0byBgXHUyMDI2L3RyZWUvdjMuMy4wYCBhbmQgbmFtZXMgXCJEdXBsaWNhY3kgMy4zLjBcIiBpbiBib3RoIHRoZSBpbnRybyBwYXJhZ3JhcGggYW5kIHRoZSBMaWNlbnNpbmcgc2VjdGlvbi4gVXBkYXRlIHRob3NlIHJlZmVyZW5jZXMgdG8gMy4zLjEgc28gdGhlIGRvY3MgbWF0Y2ggdGhlIGNvZGUuIiwgInJlc3BvbnNlIjogIiJ9LCB7ImlkIjogIkYzIiwgImZpbGUiOiAiLmZvcmdlam8vc2NyaXB0cy9wcmVwYXJlLXJlbGVhc2UtdHJlZS5zaCIsICJsaW5lIjogMSwgInNldmVyaXR5IjogInN1Z2dlc3Rpb24iLCAic3RhdHVzIjogIndpdGhkcmF3biIsICJ0aXRsZSI6ICJQUiBidW5kbGVzIGEgcmVsZWFzZS13b3JrZmxvdyByZWRlc2lnbiB3aXRoIGEgb25lLWxpbmUgZGVwZW5kZW5jeSBidW1wIChydWxlcyA3LCA4KSIsICJkZXRhaWwiOiAiV2l0aGRyYXduOiB0aGUgYWN0dWFsIGRpZmYgY29udGFpbnMgb25seSB0aGUgc2luZ2xlIERvY2tlcmZpbGUgbGluZSBjaGFuZ2UuIE5vIHJlbGVhc2Utd29ya2Zsb3cgZmlsZXMgYXBwZWFyIGluIHRoZSBkaWZmLiIsICJyZXNwb25zZSI6ICJUaGUgZGlmZiBJIGNhbiBzZWUgaXMgbGltaXRlZCB0byB0aGUgb25lLWxpbmUgRG9ja2VyZmlsZSBjaGFuZ2UuIFRoZXJlIGFyZSBubyByZWxlYXNlLXdvcmtmbG93IGZpbGVzIGluIHRoZSBkaWZmLCBzbyB0aGUgYnVuZGxpbmcgY29uY2VybiBkb2VzIG5vdCBhcHBseS4gV2l0aGRyYXdpbmcuIn0sIHsiaWQiOiAiRjQiLCAiZmlsZSI6ICIuZm9yZ2Vqby9zY3JpcHRzL2FpLXJldmlldy5weSIsICJsaW5lIjogMjg2LCAic2V2ZXJpdHkiOiAic3VnZ2VzdGlvbiIsICJzdGF0dXMiOiAid2l0aGRyYXduIiwgInRpdGxlIjogIlVucmVsYXRlZCBhaS1yZXZpZXcucHkgcmVkZXNpZ24gYnVuZGxlZCB3aXRoIGEgZGVwZW5kZW5jeSBidW1wIChydWxlcyA3LCA4KSIsICJkZXRhaWwiOiAiV2l0aGRyYXduOiB0aGUgZGlmZiBwcm92aWRlZCBjb250YWlucyBvbmx5IHRoZSBEb2NrZXJmaWxlIGNoYW5nZS4gVGhlcmUgaXMgbm8gYWktcmV2aWV3LnB5IG9yIGFpLXJldmlldy55bWwgbW9kaWZpY2F0aW9uIGluIHRoaXMgUFIuIFRoZSBmaW5kaW5nIHdhcyBiYXNlZCBvbiBjb250ZW50IG5vdCBwcmVzZW50IGluIHRoZSBkaWZmLiAoTm90IGJsb2NraW5nOiBpdCBkaWQgbm90IGNpdGUgYSBjaGFuZ2VkIGZpbGUgYW5kIGxpbmUuKSIsICJyZXNwb25zZSI6ICJSZS1leGFtaW5pbmcgdGhlIGRpZmYsIGl0IGNvbnRhaW5zIG9ubHkgdGhlIHNpbmdsZSBEb2NrZXJmaWxlIEVOViBsaW5lIGNoYW5nZS4gTm8gYWktcmV2aWV3LnB5IG9yIGFpLXJldmlldy55bWwgY2hhbmdlcyBhcmUgcHJlc2VudC4gV2l0aGRyYXdpbmcgdGhpcyBmaW5kaW5nLiJ9XQ== -->
