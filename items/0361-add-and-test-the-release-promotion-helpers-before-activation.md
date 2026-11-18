# #361 Add and test the release promotion helpers before activation

- Kind: pull, state: closed, by cody, created 2026-11-17T19:59:12-08:00
- Labels: none; milestone: none
- refs/pull/361/head -> main, merged: True

Closes #360. Refs #352.

Extract the exact promotion and verification helpers from #355, with their CI fixture. The existing release dispatches keep using their current scripts. This lets the helper and activation changes receive independent hosted verdicts.

The release fixture exercised active minor, alpha and RC versions, an older maintenance patch, tag replay and rejection of a replaced candidate. Local unittest, exact script ShellCheck, YAML lint, Ruff and pipeline lint passed. The independent review prompted wiring the tests and ShellCheck into the existing CI workflow.

## forgejo-actions, 2026-11-17T20:02:52-08:00

<!-- ai-review -->
**AI review** of `6e8af17155`: The PR adds two inert release promotion helpers (prepare-release-tree.sh and verify-release-pr.py) with an end-to-end test and CI wiring. The scripts are correct, the test covers the main paths (reproduce, tag replay, candidate rejection) across all four bump types, and the CI integration is minimal and appropriate. No blocking issues found.

### Open

- **F1** (suggestion, `.forgejo/scripts/verify-release-pr.py:30`) reproduce() uses `git add -u` which silently misses new files: `git add -u` only stages modifications and deletions of already-tracked files. If a future revision of `prepare-release-tree.sh` introduces a new file into the release tree (e.g. a generated manifest), the reproduced tree would silently omit it and the comparison against the candidate tree would pass incorrectly. Consider `git add -A` (or `git add .`) to stage new files as well, or add a comment documenting the invariant that the script only modifies existing tracked files.
- **F2** (suggestion, `tests/test_release_promotion.py:1`) Error-path branches in verify-release-pr.py are untested: The test exercises the happy path (reproduce, tag replay) and one rejection (head SHA set to merge commit). Roughly a dozen `raise ValueError` branches in `verify()` (unmerged PR, wrong repo, invalid channel, tree mismatch, changelog/version mismatch, tag pointing elsewhere, PR-ref mismatch) have no test. Rule 13 requires 99% branch coverage overall; adding ~20 uncovered branches in a new file may push the project below that threshold. If the CI coverage gate does not flag it, consider adding a few focused negative tests (e.g. unmerged PR, wrong repository, tree mismatch) to keep the margin comfortable.


Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W3siaWQiOiAiRjEiLCAiZmlsZSI6ICIuZm9yZ2Vqby9zY3JpcHRzL3ZlcmlmeS1yZWxlYXNlLXByLnB5IiwgImxpbmUiOiAzMCwgInNldmVyaXR5IjogInN1Z2dlc3Rpb24iLCAic3RhdHVzIjogIm9wZW4iLCAidGl0bGUiOiAicmVwcm9kdWNlKCkgdXNlcyBgZ2l0IGFkZCAtdWAgd2hpY2ggc2lsZW50bHkgbWlzc2VzIG5ldyBmaWxlcyIsICJkZXRhaWwiOiAiYGdpdCBhZGQgLXVgIG9ubHkgc3RhZ2VzIG1vZGlmaWNhdGlvbnMgYW5kIGRlbGV0aW9ucyBvZiBhbHJlYWR5LXRyYWNrZWQgZmlsZXMuIElmIGEgZnV0dXJlIHJldmlzaW9uIG9mIGBwcmVwYXJlLXJlbGVhc2UtdHJlZS5zaGAgaW50cm9kdWNlcyBhIG5ldyBmaWxlIGludG8gdGhlIHJlbGVhc2UgdHJlZSAoZS5nLiBhIGdlbmVyYXRlZCBtYW5pZmVzdCksIHRoZSByZXByb2R1Y2VkIHRyZWUgd291bGQgc2lsZW50bHkgb21pdCBpdCBhbmQgdGhlIGNvbXBhcmlzb24gYWdhaW5zdCB0aGUgY2FuZGlkYXRlIHRyZWUgd291bGQgcGFzcyBpbmNvcnJlY3RseS4gQ29uc2lkZXIgYGdpdCBhZGQgLUFgIChvciBgZ2l0IGFkZCAuYCkgdG8gc3RhZ2UgbmV3IGZpbGVzIGFzIHdlbGwsIG9yIGFkZCBhIGNvbW1lbnQgZG9jdW1lbnRpbmcgdGhlIGludmFyaWFudCB0aGF0IHRoZSBzY3JpcHQgb25seSBtb2RpZmllcyBleGlzdGluZyB0cmFja2VkIGZpbGVzLiIsICJyZXNwb25zZSI6ICIifSwgeyJpZCI6ICJGMiIsICJmaWxlIjogInRlc3RzL3Rlc3RfcmVsZWFzZV9wcm9tb3Rpb24ucHkiLCAibGluZSI6IDEsICJzZXZlcml0eSI6ICJzdWdnZXN0aW9uIiwgInN0YXR1cyI6ICJvcGVuIiwgInRpdGxlIjogIkVycm9yLXBhdGggYnJhbmNoZXMgaW4gdmVyaWZ5LXJlbGVhc2UtcHIucHkgYXJlIHVudGVzdGVkIiwgImRldGFpbCI6ICJUaGUgdGVzdCBleGVyY2lzZXMgdGhlIGhhcHB5IHBhdGggKHJlcHJvZHVjZSwgdGFnIHJlcGxheSkgYW5kIG9uZSByZWplY3Rpb24gKGhlYWQgU0hBIHNldCB0byBtZXJnZSBjb21taXQpLiBSb3VnaGx5IGEgZG96ZW4gYHJhaXNlIFZhbHVlRXJyb3JgIGJyYW5jaGVzIGluIGB2ZXJpZnkoKWAgKHVubWVyZ2VkIFBSLCB3cm9uZyByZXBvLCBpbnZhbGlkIGNoYW5uZWwsIHRyZWUgbWlzbWF0Y2gsIGNoYW5nZWxvZy92ZXJzaW9uIG1pc21hdGNoLCB0YWcgcG9pbnRpbmcgZWxzZXdoZXJlLCBQUi1yZWYgbWlzbWF0Y2gpIGhhdmUgbm8gdGVzdC4gUnVsZSAxMyByZXF1aXJlcyA5OSUgYnJhbmNoIGNvdmVyYWdlIG92ZXJhbGw7IGFkZGluZyB+MjAgdW5jb3ZlcmVkIGJyYW5jaGVzIGluIGEgbmV3IGZpbGUgbWF5IHB1c2ggdGhlIHByb2plY3QgYmVsb3cgdGhhdCB0aHJlc2hvbGQuIElmIHRoZSBDSSBjb3ZlcmFnZSBnYXRlIGRvZXMgbm90IGZsYWcgaXQsIGNvbnNpZGVyIGFkZGluZyBhIGZldyBmb2N1c2VkIG5lZ2F0aXZlIHRlc3RzIChlLmcuIHVubWVyZ2VkIFBSLCB3cm9uZyByZXBvc2l0b3J5LCB0cmVlIG1pc21hdGNoKSB0byBrZWVwIHRoZSBtYXJnaW4gY29tZm9ydGFibGUuIiwgInJlc3BvbnNlIjogIiJ9XQ== -->
