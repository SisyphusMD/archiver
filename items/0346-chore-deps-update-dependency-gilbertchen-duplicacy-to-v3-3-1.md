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
**AI review** of `1158f807db`: This PR is titled as a Duplicacy 3.3.0→3.3.1 patch bump but bundles an entire release-workflow redesign (~636 non-test lines) with the one-line dependency change, violating rules 7 and 8. The SHA-256 for the new Duplicacy artifact is also still the 3.3.0 value, which will break the image build.

### Open

- **F1** (**blocking**, `Dockerfile:27`) DUPLICACY_SOURCE_SHA256 not updated for 3.3.1: The version was bumped to 3.3.1 but `ARG DUPLICACY_SOURCE_SHA256` still holds the hash for the 3.3.0 source/binary (`dea27ecb…`). The Docker build's integrity check will reject the 3.3.1 artifact and the image build will fail. Update the hash to the value published for the v3.3.1 release.
- **F2** (suggestion, `README.md:1`) README still references Duplicacy 3.3.0: Rule 10 requires docs to change in the same PR as the behaviour they describe. The README links to `…/tree/v3.3.0` and names "Duplicacy 3.3.0" in both the intro paragraph and the Licensing section. Update those references to 3.3.1 so the docs match the code.
- **F3** (**blocking**, `.forgejo/scripts/prepare-release-tree.sh:1`) PR bundles a release-workflow redesign with a one-line dependency bump (rules 7, 8): The PR title and description say this is a Duplicacy 3.3.0→3.3.1 patch update (one line in the Dockerfile). Instead it also introduces a complete release-workflow redesign: `prepare-release-tree.sh` (68 lines), `verify-release-pr.py` (104 lines), `publish-release.yml` (84 lines), a full rewrite of `release.yml` (~360 changed lines), plus CHANGELOG and CONTRIBUTING edits. That is ~636 non-test lines, well over rule 8's 400-line threshold, and the description carries no `Large diff: <reason>`. Rule 7 requires one concern per PR; the release-workflow work belongs in its own PR closing its own issue (#352, per the CHANGELOG entry). Split the dependency bump (Dockerfile + README + SHA) into this PR and move the release-workflow changes to a separate PR.


Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W3siaWQiOiAiRjEiLCAiZmlsZSI6ICJEb2NrZXJmaWxlIiwgImxpbmUiOiAyNywgInNldmVyaXR5IjogImJsb2NraW5nIiwgInN0YXR1cyI6ICJvcGVuIiwgInRpdGxlIjogIkRVUExJQ0FDWV9TT1VSQ0VfU0hBMjU2IG5vdCB1cGRhdGVkIGZvciAzLjMuMSIsICJkZXRhaWwiOiAiVGhlIHZlcnNpb24gd2FzIGJ1bXBlZCB0byAzLjMuMSBidXQgYEFSRyBEVVBMSUNBQ1lfU09VUkNFX1NIQTI1NmAgc3RpbGwgaG9sZHMgdGhlIGhhc2ggZm9yIHRoZSAzLjMuMCBzb3VyY2UvYmluYXJ5IChgZGVhMjdlY2JcdTIwMjZgKS4gVGhlIERvY2tlciBidWlsZCdzIGludGVncml0eSBjaGVjayB3aWxsIHJlamVjdCB0aGUgMy4zLjEgYXJ0aWZhY3QgYW5kIHRoZSBpbWFnZSBidWlsZCB3aWxsIGZhaWwuIFVwZGF0ZSB0aGUgaGFzaCB0byB0aGUgdmFsdWUgcHVibGlzaGVkIGZvciB0aGUgdjMuMy4xIHJlbGVhc2UuIiwgInJlc3BvbnNlIjogIiJ9LCB7ImlkIjogIkYyIiwgImZpbGUiOiAiUkVBRE1FLm1kIiwgImxpbmUiOiAxLCAic2V2ZXJpdHkiOiAic3VnZ2VzdGlvbiIsICJzdGF0dXMiOiAib3BlbiIsICJ0aXRsZSI6ICJSRUFETUUgc3RpbGwgcmVmZXJlbmNlcyBEdXBsaWNhY3kgMy4zLjAiLCAiZGV0YWlsIjogIlJ1bGUgMTAgcmVxdWlyZXMgZG9jcyB0byBjaGFuZ2UgaW4gdGhlIHNhbWUgUFIgYXMgdGhlIGJlaGF2aW91ciB0aGV5IGRlc2NyaWJlLiBUaGUgUkVBRE1FIGxpbmtzIHRvIGBcdTIwMjYvdHJlZS92My4zLjBgIGFuZCBuYW1lcyBcIkR1cGxpY2FjeSAzLjMuMFwiIGluIGJvdGggdGhlIGludHJvIHBhcmFncmFwaCBhbmQgdGhlIExpY2Vuc2luZyBzZWN0aW9uLiBVcGRhdGUgdGhvc2UgcmVmZXJlbmNlcyB0byAzLjMuMSBzbyB0aGUgZG9jcyBtYXRjaCB0aGUgY29kZS4iLCAicmVzcG9uc2UiOiAiIn0sIHsiaWQiOiAiRjMiLCAiZmlsZSI6ICIuZm9yZ2Vqby9zY3JpcHRzL3ByZXBhcmUtcmVsZWFzZS10cmVlLnNoIiwgImxpbmUiOiAxLCAic2V2ZXJpdHkiOiAiYmxvY2tpbmciLCAic3RhdHVzIjogIm9wZW4iLCAidGl0bGUiOiAiUFIgYnVuZGxlcyBhIHJlbGVhc2Utd29ya2Zsb3cgcmVkZXNpZ24gd2l0aCBhIG9uZS1saW5lIGRlcGVuZGVuY3kgYnVtcCAocnVsZXMgNywgOCkiLCAiZGV0YWlsIjogIlRoZSBQUiB0aXRsZSBhbmQgZGVzY3JpcHRpb24gc2F5IHRoaXMgaXMgYSBEdXBsaWNhY3kgMy4zLjBcdTIxOTIzLjMuMSBwYXRjaCB1cGRhdGUgKG9uZSBsaW5lIGluIHRoZSBEb2NrZXJmaWxlKS4gSW5zdGVhZCBpdCBhbHNvIGludHJvZHVjZXMgYSBjb21wbGV0ZSByZWxlYXNlLXdvcmtmbG93IHJlZGVzaWduOiBgcHJlcGFyZS1yZWxlYXNlLXRyZWUuc2hgICg2OCBsaW5lcyksIGB2ZXJpZnktcmVsZWFzZS1wci5weWAgKDEwNCBsaW5lcyksIGBwdWJsaXNoLXJlbGVhc2UueW1sYCAoODQgbGluZXMpLCBhIGZ1bGwgcmV3cml0ZSBvZiBgcmVsZWFzZS55bWxgICh+MzYwIGNoYW5nZWQgbGluZXMpLCBwbHVzIENIQU5HRUxPRyBhbmQgQ09OVFJJQlVUSU5HIGVkaXRzLiBUaGF0IGlzIH42MzYgbm9uLXRlc3QgbGluZXMsIHdlbGwgb3ZlciBydWxlIDgncyA0MDAtbGluZSB0aHJlc2hvbGQsIGFuZCB0aGUgZGVzY3JpcHRpb24gY2FycmllcyBubyBgTGFyZ2UgZGlmZjogPHJlYXNvbj5gLiBSdWxlIDcgcmVxdWlyZXMgb25lIGNvbmNlcm4gcGVyIFBSOyB0aGUgcmVsZWFzZS13b3JrZmxvdyB3b3JrIGJlbG9uZ3MgaW4gaXRzIG93biBQUiBjbG9zaW5nIGl0cyBvd24gaXNzdWUgKCMzNTIsIHBlciB0aGUgQ0hBTkdFTE9HIGVudHJ5KS4gU3BsaXQgdGhlIGRlcGVuZGVuY3kgYnVtcCAoRG9ja2VyZmlsZSArIFJFQURNRSArIFNIQSkgaW50byB0aGlzIFBSIGFuZCBtb3ZlIHRoZSByZWxlYXNlLXdvcmtmbG93IGNoYW5nZXMgdG8gYSBzZXBhcmF0ZSBQUi4iLCAicmVzcG9uc2UiOiAiIn1d -->
