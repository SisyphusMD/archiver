# #356 Retire the direct-write release workflow on release/0.11

- Kind: issue, state: open, by cody, created 2026-11-17T18:54:57-08:00
- Labels: status:blocked, type:chore; milestone: none

Once #352 provides the verified maintenance-channel release flow from main, replace release/0.11’s old direct branch and tag writer with guidance to dispatch Prepare Release from main targeting release/0.11. Keep tag-triggered archive publishing and verify patch releases through the new path before requiring protection.

Depends on #352.
