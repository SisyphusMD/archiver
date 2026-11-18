# #360 Review the release promotion helpers before switching the workflow

- Kind: issue, state: closed, by cody, created 2026-11-17T19:54:09-08:00
- Labels: none; milestone: none

Part of #352. Archiver #355 passed native integration and trust checks, but the hosted review has repeatedly failed to return a verdict on the combined workflows and helpers. Land the two inert release promotion helpers and their fixtures independently, then bring #355 up to date so the activation can review a smaller diff. Existing release dispatches and tags stay in service during this step.
