# #190 fix(notify): Apprise tag all when APPRISE_TAGS names none

- Kind: pull, state: closed, by cody, created 2026-10-08T18:45:01-07:00
- Labels: none; milestone: none
- refs/pull/190/head -> main, merged: True

Follow-up to #188, found by review: apprise-api views.py sets selected_tag = content tag or ("all" if key is None else None), so a stateful /notify/<key> request without a tag reaches only untagged URLs. Unmapped kinds now send tag all; README/CHANGELOG/comments corrected.
