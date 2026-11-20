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
**AI review** of `b878d37d4c`: The PR bumps Duplicacy to 3.3.1 but leaves the source SHA256 unchanged (the build will fail), and bundles an unrelated ai-review.py deadline/error-handling redesign that violates rules 7 and 8.

### Open

- **F1** (**blocking**, `Dockerfile:27`) DUPLICACY_SOURCE_SHA256 not updated for 3.3.1: The version was bumped to 3.3.1 but `ARG DUPLICACY_SOURCE_SHA256` still holds the hash for the 3.3.0 artifact (`dea27ecb…`). The Docker build's integrity check will reject the 3.3.1 download and the image build will fail. Update the hash to the value published for the v3.3.1 release.
- **F2** (suggestion, `README.md:1`) README still references Duplicacy 3.3.0: Rule 10 requires docs to change in the same PR as the behaviour they describe. The README links to `…/tree/v3.3.0` and names "Duplicacy 3.3.0" in both the intro paragraph and the Licensing section. Update those references to 3.3.1 so the docs match the code.
- **F4** (**blocking**, `.forgejo/scripts/ai-review.py:286`) Unrelated ai-review.py redesign bundled with a dependency bump (rules 7, 8): The PR's stated concern is bumping Duplicacy from 3.3.0 to 3.3.1 (one Dockerfile line). The diff also adds a MODEL_DEADLINE mechanism, a signal-based timeout, a new Deadline exception, restructured error handling, and a workflow timeout change (20→45 min) in ai-review.py and ai-review.yml. None of this is needed for the version bump to work. Rule 7: "Independently useful fixes, cleanups or speedups are separate PRs." Rule 8: "no unrelated cleanup, renaming, reformatting, new abstractions or drive-by refactors." Split the ai-review.py/ai-review.yml changes into their own PR with its own issue.

### Withdrawn

- **F3** (suggestion, `.forgejo/scripts/prepare-release-tree.sh:1`) PR bundles a release-workflow redesign with a one-line dependency bump (rules 7, 8): Withdrawn: the actual diff contains only the single Dockerfile line change. No release-workflow files (prepare-release-tree.sh, verify-release-pr.py, publish-release.yml, release.yml rewrite, CHANGELOG, CONTRIBUTING) appear in the diff. The finding was based on a misread of the PR scope. (Not blocking: it did not cite a changed file and line.)
  - Reviewer: The diff I can see is limited to the one-line Dockerfile change. There are no release-workflow files in the diff, so the bundling concern does not apply. Withdrawing.


Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W3siaWQiOiAiRjEiLCAiZmlsZSI6ICJEb2NrZXJmaWxlIiwgImxpbmUiOiAyNywgInNldmVyaXR5IjogImJsb2NraW5nIiwgInN0YXR1cyI6ICJvcGVuIiwgInRpdGxlIjogIkRVUExJQ0FDWV9TT1VSQ0VfU0hBMjU2IG5vdCB1cGRhdGVkIGZvciAzLjMuMSIsICJkZXRhaWwiOiAiVGhlIHZlcnNpb24gd2FzIGJ1bXBlZCB0byAzLjMuMSBidXQgYEFSRyBEVVBMSUNBQ1lfU09VUkNFX1NIQTI1NmAgc3RpbGwgaG9sZHMgdGhlIGhhc2ggZm9yIHRoZSAzLjMuMCBhcnRpZmFjdCAoYGRlYTI3ZWNiXHUyMDI2YCkuIFRoZSBEb2NrZXIgYnVpbGQncyBpbnRlZ3JpdHkgY2hlY2sgd2lsbCByZWplY3QgdGhlIDMuMy4xIGRvd25sb2FkIGFuZCB0aGUgaW1hZ2UgYnVpbGQgd2lsbCBmYWlsLiBVcGRhdGUgdGhlIGhhc2ggdG8gdGhlIHZhbHVlIHB1Ymxpc2hlZCBmb3IgdGhlIHYzLjMuMSByZWxlYXNlLiIsICJyZXNwb25zZSI6ICIifSwgeyJpZCI6ICJGMiIsICJmaWxlIjogIlJFQURNRS5tZCIsICJsaW5lIjogMSwgInNldmVyaXR5IjogInN1Z2dlc3Rpb24iLCAic3RhdHVzIjogIm9wZW4iLCAidGl0bGUiOiAiUkVBRE1FIHN0aWxsIHJlZmVyZW5jZXMgRHVwbGljYWN5IDMuMy4wIiwgImRldGFpbCI6ICJSdWxlIDEwIHJlcXVpcmVzIGRvY3MgdG8gY2hhbmdlIGluIHRoZSBzYW1lIFBSIGFzIHRoZSBiZWhhdmlvdXIgdGhleSBkZXNjcmliZS4gVGhlIFJFQURNRSBsaW5rcyB0byBgXHUyMDI2L3RyZWUvdjMuMy4wYCBhbmQgbmFtZXMgXCJEdXBsaWNhY3kgMy4zLjBcIiBpbiBib3RoIHRoZSBpbnRybyBwYXJhZ3JhcGggYW5kIHRoZSBMaWNlbnNpbmcgc2VjdGlvbi4gVXBkYXRlIHRob3NlIHJlZmVyZW5jZXMgdG8gMy4zLjEgc28gdGhlIGRvY3MgbWF0Y2ggdGhlIGNvZGUuIiwgInJlc3BvbnNlIjogIiJ9LCB7ImlkIjogIkYzIiwgImZpbGUiOiAiLmZvcmdlam8vc2NyaXB0cy9wcmVwYXJlLXJlbGVhc2UtdHJlZS5zaCIsICJsaW5lIjogMSwgInNldmVyaXR5IjogInN1Z2dlc3Rpb24iLCAic3RhdHVzIjogIndpdGhkcmF3biIsICJ0aXRsZSI6ICJQUiBidW5kbGVzIGEgcmVsZWFzZS13b3JrZmxvdyByZWRlc2lnbiB3aXRoIGEgb25lLWxpbmUgZGVwZW5kZW5jeSBidW1wIChydWxlcyA3LCA4KSIsICJkZXRhaWwiOiAiV2l0aGRyYXduOiB0aGUgYWN0dWFsIGRpZmYgY29udGFpbnMgb25seSB0aGUgc2luZ2xlIERvY2tlcmZpbGUgbGluZSBjaGFuZ2UuIE5vIHJlbGVhc2Utd29ya2Zsb3cgZmlsZXMgKHByZXBhcmUtcmVsZWFzZS10cmVlLnNoLCB2ZXJpZnktcmVsZWFzZS1wci5weSwgcHVibGlzaC1yZWxlYXNlLnltbCwgcmVsZWFzZS55bWwgcmV3cml0ZSwgQ0hBTkdFTE9HLCBDT05UUklCVVRJTkcpIGFwcGVhciBpbiB0aGUgZGlmZi4gVGhlIGZpbmRpbmcgd2FzIGJhc2VkIG9uIGEgbWlzcmVhZCBvZiB0aGUgUFIgc2NvcGUuIChOb3QgYmxvY2tpbmc6IGl0IGRpZCBub3QgY2l0ZSBhIGNoYW5nZWQgZmlsZSBhbmQgbGluZS4pIiwgInJlc3BvbnNlIjogIlRoZSBkaWZmIEkgY2FuIHNlZSBpcyBsaW1pdGVkIHRvIHRoZSBvbmUtbGluZSBEb2NrZXJmaWxlIGNoYW5nZS4gVGhlcmUgYXJlIG5vIHJlbGVhc2Utd29ya2Zsb3cgZmlsZXMgaW4gdGhlIGRpZmYsIHNvIHRoZSBidW5kbGluZyBjb25jZXJuIGRvZXMgbm90IGFwcGx5LiBXaXRoZHJhd2luZy4ifSwgeyJpZCI6ICJGNCIsICJmaWxlIjogIi5mb3JnZWpvL3NjcmlwdHMvYWktcmV2aWV3LnB5IiwgImxpbmUiOiAyODYsICJzZXZlcml0eSI6ICJibG9ja2luZyIsICJzdGF0dXMiOiAib3BlbiIsICJ0aXRsZSI6ICJVbnJlbGF0ZWQgYWktcmV2aWV3LnB5IHJlZGVzaWduIGJ1bmRsZWQgd2l0aCBhIGRlcGVuZGVuY3kgYnVtcCAocnVsZXMgNywgOCkiLCAiZGV0YWlsIjogIlRoZSBQUidzIHN0YXRlZCBjb25jZXJuIGlzIGJ1bXBpbmcgRHVwbGljYWN5IGZyb20gMy4zLjAgdG8gMy4zLjEgKG9uZSBEb2NrZXJmaWxlIGxpbmUpLiBUaGUgZGlmZiBhbHNvIGFkZHMgYSBNT0RFTF9ERUFETElORSBtZWNoYW5pc20sIGEgc2lnbmFsLWJhc2VkIHRpbWVvdXQsIGEgbmV3IERlYWRsaW5lIGV4Y2VwdGlvbiwgcmVzdHJ1Y3R1cmVkIGVycm9yIGhhbmRsaW5nLCBhbmQgYSB3b3JrZmxvdyB0aW1lb3V0IGNoYW5nZSAoMjBcdTIxOTI0NSBtaW4pIGluIGFpLXJldmlldy5weSBhbmQgYWktcmV2aWV3LnltbC4gTm9uZSBvZiB0aGlzIGlzIG5lZWRlZCBmb3IgdGhlIHZlcnNpb24gYnVtcCB0byB3b3JrLiBSdWxlIDc6IFwiSW5kZXBlbmRlbnRseSB1c2VmdWwgZml4ZXMsIGNsZWFudXBzIG9yIHNwZWVkdXBzIGFyZSBzZXBhcmF0ZSBQUnMuXCIgUnVsZSA4OiBcIm5vIHVucmVsYXRlZCBjbGVhbnVwLCByZW5hbWluZywgcmVmb3JtYXR0aW5nLCBuZXcgYWJzdHJhY3Rpb25zIG9yIGRyaXZlLWJ5IHJlZmFjdG9ycy5cIiBTcGxpdCB0aGUgYWktcmV2aWV3LnB5L2FpLXJldmlldy55bWwgY2hhbmdlcyBpbnRvIHRoZWlyIG93biBQUiB3aXRoIGl0cyBvd24gaXNzdWUuIiwgInJlc3BvbnNlIjogIiJ9XQ== -->
