# #130 fix: keep the Pushover credentials off argv (0.11.3 candidate)

- Kind: pull, state: closed, by cody, created 2026-10-01T07:10:00-07:00
- Labels: none; milestone: none
- refs/pull/130/head -> release/0.11, merged: True

Cherry-pick of main PR #129 (e4030c1, bd519af) for the 0.11 line. Test first: TestConfigSecretsStayHidden with a notifier fails on 0.11.2.
