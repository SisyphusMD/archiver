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
**AI review** of `186c89d81b`: The version bump to 3.3.1 is correct but the SHA256 integrity hash on the next line was not updated, which will cause the Docker build to fail. The README also still references 3.3.0 per rule 10.

### Open

- **F1** (**blocking**, `Dockerfile:28`) DUPLICACY_SOURCE_SHA256 not updated for 3.3.1: The version was bumped to 3.3.1 but `ARG DUPLICACY_SOURCE_SHA256` still holds the hash for the 3.3.0 artifact (`dea27ecb…`). The Docker build's integrity check will reject the 3.3.1 download and the image build will fail. The Renovate annotation on line 26 only tracks the ENV version, not the ARG hash, so this must be updated manually. Verify the published checksum for the v3.3.1 source tarball and update the ARG value.
  - Reviewer: This is still open. The diff shows the ARG line unchanged (context line, no +/-). The 3.3.1 tarball is built with a different Go version and different source, so its SHA256 will differ. The `Dockerfile download integrity` CI check will fail. Please update the hash to the correct value for the v3.3.1 source tarball.
- **F2** (suggestion, `README.md:1`) README still references Duplicacy 3.3.0: Rule 10 requires docs to change in the same PR as the behaviour they describe. The README links to `…/tree/v3.3.0` and names "Duplicacy 3.3.0" in both the intro paragraph and the Licensing section. Update those references to 3.3.1 so the docs match the code.

### Withdrawn

- **F3** (suggestion, `.forgejo/scripts/prepare-release-tree.sh:1`) PR bundles a release-workflow redesign with a one-line dependency bump (rules 7, 8): Withdrawn: the actual diff contains only the single Dockerfile line change. No release-workflow files appear in the diff.
  - Reviewer: The diff I can see is limited to the one-line Dockerfile change. There are no release-workflow files in the diff, so the bundling concern does not apply. Withdrawing.
- **F4** (suggestion, `.forgejo/scripts/ai-review.py:286`) Unrelated ai-review.py redesign bundled with a dependency bump (rules 7, 8): Withdrawn: the diff provided contains only the Dockerfile change. There is no ai-review.py or ai-review.yml modification in this PR. The finding was based on content not present in the diff. (Not blocking: it did not cite a changed file and line.)
  - Reviewer: Re-examining the diff, it contains only the single Dockerfile ENV line change. No ai-review.py or ai-review.yml changes are present. Withdrawing this finding.


Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W3siaWQiOiAiRjEiLCAiZmlsZSI6ICJEb2NrZXJmaWxlIiwgImxpbmUiOiAyOCwgInNldmVyaXR5IjogImJsb2NraW5nIiwgInN0YXR1cyI6ICJvcGVuIiwgInRpdGxlIjogIkRVUExJQ0FDWV9TT1VSQ0VfU0hBMjU2IG5vdCB1cGRhdGVkIGZvciAzLjMuMSIsICJkZXRhaWwiOiAiVGhlIHZlcnNpb24gd2FzIGJ1bXBlZCB0byAzLjMuMSBidXQgYEFSRyBEVVBMSUNBQ1lfU09VUkNFX1NIQTI1NmAgc3RpbGwgaG9sZHMgdGhlIGhhc2ggZm9yIHRoZSAzLjMuMCBhcnRpZmFjdCAoYGRlYTI3ZWNiXHUyMDI2YCkuIFRoZSBEb2NrZXIgYnVpbGQncyBpbnRlZ3JpdHkgY2hlY2sgd2lsbCByZWplY3QgdGhlIDMuMy4xIGRvd25sb2FkIGFuZCB0aGUgaW1hZ2UgYnVpbGQgd2lsbCBmYWlsLiBUaGUgUmVub3ZhdGUgYW5ub3RhdGlvbiBvbiBsaW5lIDI2IG9ubHkgdHJhY2tzIHRoZSBFTlYgdmVyc2lvbiwgbm90IHRoZSBBUkcgaGFzaCwgc28gdGhpcyBtdXN0IGJlIHVwZGF0ZWQgbWFudWFsbHkuIFZlcmlmeSB0aGUgcHVibGlzaGVkIGNoZWNrc3VtIGZvciB0aGUgdjMuMy4xIHNvdXJjZSB0YXJiYWxsIGFuZCB1cGRhdGUgdGhlIEFSRyB2YWx1ZS4iLCAicmVzcG9uc2UiOiAiVGhpcyBpcyBzdGlsbCBvcGVuLiBUaGUgZGlmZiBzaG93cyB0aGUgQVJHIGxpbmUgdW5jaGFuZ2VkIChjb250ZXh0IGxpbmUsIG5vICsvLSkuIFRoZSAzLjMuMSB0YXJiYWxsIGlzIGJ1aWx0IHdpdGggYSBkaWZmZXJlbnQgR28gdmVyc2lvbiBhbmQgZGlmZmVyZW50IHNvdXJjZSwgc28gaXRzIFNIQTI1NiB3aWxsIGRpZmZlci4gVGhlIGBEb2NrZXJmaWxlIGRvd25sb2FkIGludGVncml0eWAgQ0kgY2hlY2sgd2lsbCBmYWlsLiBQbGVhc2UgdXBkYXRlIHRoZSBoYXNoIHRvIHRoZSBjb3JyZWN0IHZhbHVlIGZvciB0aGUgdjMuMy4xIHNvdXJjZSB0YXJiYWxsLiJ9LCB7ImlkIjogIkYyIiwgImZpbGUiOiAiUkVBRE1FLm1kIiwgImxpbmUiOiAxLCAic2V2ZXJpdHkiOiAic3VnZ2VzdGlvbiIsICJzdGF0dXMiOiAib3BlbiIsICJ0aXRsZSI6ICJSRUFETUUgc3RpbGwgcmVmZXJlbmNlcyBEdXBsaWNhY3kgMy4zLjAiLCAiZGV0YWlsIjogIlJ1bGUgMTAgcmVxdWlyZXMgZG9jcyB0byBjaGFuZ2UgaW4gdGhlIHNhbWUgUFIgYXMgdGhlIGJlaGF2aW91ciB0aGV5IGRlc2NyaWJlLiBUaGUgUkVBRE1FIGxpbmtzIHRvIGBcdTIwMjYvdHJlZS92My4zLjBgIGFuZCBuYW1lcyBcIkR1cGxpY2FjeSAzLjMuMFwiIGluIGJvdGggdGhlIGludHJvIHBhcmFncmFwaCBhbmQgdGhlIExpY2Vuc2luZyBzZWN0aW9uLiBVcGRhdGUgdGhvc2UgcmVmZXJlbmNlcyB0byAzLjMuMSBzbyB0aGUgZG9jcyBtYXRjaCB0aGUgY29kZS4iLCAicmVzcG9uc2UiOiAiIn0sIHsiaWQiOiAiRjMiLCAiZmlsZSI6ICIuZm9yZ2Vqby9zY3JpcHRzL3ByZXBhcmUtcmVsZWFzZS10cmVlLnNoIiwgImxpbmUiOiAxLCAic2V2ZXJpdHkiOiAic3VnZ2VzdGlvbiIsICJzdGF0dXMiOiAid2l0aGRyYXduIiwgInRpdGxlIjogIlBSIGJ1bmRsZXMgYSByZWxlYXNlLXdvcmtmbG93IHJlZGVzaWduIHdpdGggYSBvbmUtbGluZSBkZXBlbmRlbmN5IGJ1bXAgKHJ1bGVzIDcsIDgpIiwgImRldGFpbCI6ICJXaXRoZHJhd246IHRoZSBhY3R1YWwgZGlmZiBjb250YWlucyBvbmx5IHRoZSBzaW5nbGUgRG9ja2VyZmlsZSBsaW5lIGNoYW5nZS4gTm8gcmVsZWFzZS13b3JrZmxvdyBmaWxlcyBhcHBlYXIgaW4gdGhlIGRpZmYuIiwgInJlc3BvbnNlIjogIlRoZSBkaWZmIEkgY2FuIHNlZSBpcyBsaW1pdGVkIHRvIHRoZSBvbmUtbGluZSBEb2NrZXJmaWxlIGNoYW5nZS4gVGhlcmUgYXJlIG5vIHJlbGVhc2Utd29ya2Zsb3cgZmlsZXMgaW4gdGhlIGRpZmYsIHNvIHRoZSBidW5kbGluZyBjb25jZXJuIGRvZXMgbm90IGFwcGx5LiBXaXRoZHJhd2luZy4ifSwgeyJpZCI6ICJGNCIsICJmaWxlIjogIi5mb3JnZWpvL3NjcmlwdHMvYWktcmV2aWV3LnB5IiwgImxpbmUiOiAyODYsICJzZXZlcml0eSI6ICJzdWdnZXN0aW9uIiwgInN0YXR1cyI6ICJ3aXRoZHJhd24iLCAidGl0bGUiOiAiVW5yZWxhdGVkIGFpLXJldmlldy5weSByZWRlc2lnbiBidW5kbGVkIHdpdGggYSBkZXBlbmRlbmN5IGJ1bXAgKHJ1bGVzIDcsIDgpIiwgImRldGFpbCI6ICJXaXRoZHJhd246IHRoZSBkaWZmIHByb3ZpZGVkIGNvbnRhaW5zIG9ubHkgdGhlIERvY2tlcmZpbGUgY2hhbmdlLiBUaGVyZSBpcyBubyBhaS1yZXZpZXcucHkgb3IgYWktcmV2aWV3LnltbCBtb2RpZmljYXRpb24gaW4gdGhpcyBQUi4gVGhlIGZpbmRpbmcgd2FzIGJhc2VkIG9uIGNvbnRlbnQgbm90IHByZXNlbnQgaW4gdGhlIGRpZmYuIChOb3QgYmxvY2tpbmc6IGl0IGRpZCBub3QgY2l0ZSBhIGNoYW5nZWQgZmlsZSBhbmQgbGluZS4pIiwgInJlc3BvbnNlIjogIlJlLWV4YW1pbmluZyB0aGUgZGlmZiwgaXQgY29udGFpbnMgb25seSB0aGUgc2luZ2xlIERvY2tlcmZpbGUgRU5WIGxpbmUgY2hhbmdlLiBObyBhaS1yZXZpZXcucHkgb3IgYWktcmV2aWV3LnltbCBjaGFuZ2VzIGFyZSBwcmVzZW50LiBXaXRoZHJhd2luZyB0aGlzIGZpbmRpbmcuIn1d -->
