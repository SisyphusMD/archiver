# #186 fix(restore): warn about destination links that lead outside it

- Kind: pull, state: closed, by cody, created 2026-10-08T16:42:24-07:00
- Labels: none; milestone: none
- refs/pull/186/head -> main, merged: True

Early security pass (ADR 42), Codex finding 3: pinned Duplicacy restores through an existing symlinked directory in the destination. A warning, not a refusal, because real service directories hold legitimate links to other volumes and a DR restore must not be blocked. The containment fix (Duplicacy never writes through a link where a directory belongs) is logged for v1 as a patch beside build/duplicacy/*.patch.
