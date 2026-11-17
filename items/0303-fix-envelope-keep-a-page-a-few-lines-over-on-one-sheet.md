# #303 fix(envelope): keep a page a few lines over on one sheet

- Kind: pull, state: closed, by cody, created 2026-11-01T09:46:33-08:00
- Labels: none; milestone: none
- refs/pull/303/head -> main, merged: True

The NAS envelope (local, B2 break-glass, SFTP full access + ed25519 key) is 2 pages on rc4: its last paragraph's second line spills. When a layout runs over, the PDF is laid out again with vertical spacing at 95/90/85% and the first one-page result is kept; font sizes and QR codes unchanged, a page needing more keeps full spacing. Only the PDF renderer changes: Page elements and the fingerprint do not, so a confirmed envelope stays confirmed.

Verified: a 5-storage envelope is /Count 2 on rc4 and /Count 1 with this build; unit test TestEnvelopeFitsOneSheetWhenItCan.
