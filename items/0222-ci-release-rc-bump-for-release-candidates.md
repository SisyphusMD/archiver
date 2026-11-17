# #222 ci(release): rc bump for release candidates

- Kind: pull, state: closed, by cody, created 2026-10-10T12:59:24-07:00
- Labels: none; milestone: none
- refs/pull/222/head -> main, merged: True

Adds an `rc` bump to the release dispatch: main cuts `X.0.0-rc.N` prereleases (marked prerelease, documented image stays on stable), and an alpha is refused once its version has a release candidate. v1 goes out as rc1 first, which we run ourselves for a while before considering 1.0.0.
