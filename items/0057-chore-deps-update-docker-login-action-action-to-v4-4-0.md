# #57 chore(deps): update docker/login-action action to v4.4.0

- Kind: pull, state: closed, by cody, created 2026-07-10T19:03:06-07:00
- Labels: none; milestone: none
- refs/pull/57/head -> main, merged: True

This PR contains the following updates:

| Package | Type | Update | Change |
|---|---|---|---|
| [docker/login-action](https://github.com/docker/login-action) | action | minor | `v4.1.0` → `v4.4.0` |

---

### Release Notes

<details>
<summary>docker/login-action (docker/login-action)</summary>

### [`v4.4.0`](https://github.com/docker/login-action/releases/tag/v4.4.0)

[Compare Source](https://github.com/docker/login-action/compare/v4.3.0...v4.4.0)

- Skip empty `registry-auth` secret mask by [@&#8203;crazy-max](https://github.com/crazy-max) in [#&#8203;1035](https://github.com/docker/login-action/pull/1035)
- Bump [@&#8203;aws-sdk/client-ecr](https://github.com/aws-sdk/client-ecr) and [@&#8203;aws-sdk/client-ecr-public](https://github.com/aws-sdk/client-ecr-public) to 3.1077.0 [#&#8203;1034](https://github.com/docker/login-action/pull/1034)

**Full Changelog**: <https://github.com/docker/login-action/compare/v4.3.0...v4.4.0>

### [`v4.3.0`](https://github.com/docker/login-action/releases/tag/v4.3.0)

[Compare Source](https://github.com/docker/login-action/compare/v4.2.0...v4.3.0)

- Preserve names in esbuild bundle by [@&#8203;crazy-max](https://github.com/crazy-max) in [#&#8203;1022](https://github.com/docker/login-action/pull/1022)
- Bump [@&#8203;aws-sdk/client-ecr](https://github.com/aws-sdk/client-ecr) and [@&#8203;aws-sdk/client-ecr-public](https://github.com/aws-sdk/client-ecr-public) to 3.1076.0 [#&#8203;999](https://github.com/docker/login-action/pull/999) [#&#8203;1030](https://github.com/docker/login-action/pull/1030)
- Bump [@&#8203;docker/actions-toolkit](https://github.com/docker/actions-toolkit) from 0.90.0 to 0.92.0 in [#&#8203;1004](https://github.com/docker/login-action/pull/1004) [#&#8203;1027](https://github.com/docker/login-action/pull/1027)
- Bump [@&#8203;sigstore/core](https://github.com/sigstore/core) from 3.1.0 to 3.2.1 in [#&#8203;1023](https://github.com/docker/login-action/pull/1023)
- Bump [@&#8203;sigstore/verify](https://github.com/sigstore/verify) from 3.1.0 to 3.1.1 in [#&#8203;1029](https://github.com/docker/login-action/pull/1029)
- Bump http-proxy-agent and https-proxy-agent to 9.1.0 in [#&#8203;1017](https://github.com/docker/login-action/pull/1017)
- Bump js-yaml from 4.1.1 to 5.2.0 in [#&#8203;1028](https://github.com/docker/login-action/pull/1028)
- Bump sigstore from 4.1.0 to 4.1.1 in [#&#8203;1031](https://github.com/docker/login-action/pull/1031)
- Bump tmp from 0.2.5 to 0.2.7 in [#&#8203;1002](https://github.com/docker/login-action/pull/1002)
- Bump undici from 6.24.1 to 6.27.0 in [#&#8203;1020](https://github.com/docker/login-action/pull/1020)
- Bump vite from 7.3.3 to 7.3.6 in [#&#8203;1019](https://github.com/docker/login-action/pull/1019)

**Full Changelog**: <https://github.com/docker/login-action/compare/v4.2.0...v4.3.0>

### [`v4.2.0`](https://github.com/docker/login-action/releases/tag/v4.2.0)

[Compare Source](https://github.com/docker/login-action/compare/v4.1.0...v4.2.0)

- Bump [@&#8203;actions/core](https://github.com/actions/core) from 3.0.0 to 3.0.1 in [#&#8203;976](https://github.com/docker/login-action/pull/976)
- Bump [@&#8203;aws-sdk/client-ecr](https://github.com/aws-sdk/client-ecr) and [@&#8203;aws-sdk/client-ecr-public](https://github.com/aws-sdk/client-ecr-public) to 3.1050.0 in [#&#8203;960](https://github.com/docker/login-action/pull/960)
- Bump [@&#8203;docker/actions-toolkit](https://github.com/docker/actions-toolkit) from 0.86.0 to 0.90.0 in [#&#8203;970](https://github.com/docker/login-action/pull/970)
- Bump brace-expansion from 2.0.1 to 5.0.6 in [#&#8203;993](https://github.com/docker/login-action/pull/993)
- Bump fast-xml-builder from 1.1.4 to 1.2.0 in [#&#8203;985](https://github.com/docker/login-action/pull/985)
- Bump fast-xml-parser from 5.3.6 to 5.8.0 in [#&#8203;963](https://github.com/docker/login-action/pull/963)
- Bump http-proxy-agent and https-proxy-agent to 9.0.0 in [#&#8203;961](https://github.com/docker/login-action/pull/961)
- Bump postcss from 8.5.6 to 8.5.10 in [#&#8203;979](https://github.com/docker/login-action/pull/979)
- Bump tar from 6.2.1 to 7.5.15 in [#&#8203;991](https://github.com/docker/login-action/pull/991)
- Bump vite from 7.3.1 to 7.3.3 in [#&#8203;986](https://github.com/docker/login-action/pull/986)

**Full Changelog**: <https://github.com/docker/login-action/compare/v4.1.0...v4.2.0>

</details>

---

### Configuration

📅 **Schedule**: (UTC)

- Branch creation
  - At any time (no schedule defined)
- Automerge
  - At any time (no schedule defined)

🚦 **Automerge**: Disabled by config. Please merge this manually once you are satisfied.

♻ **Rebasing**: Whenever PR becomes conflicted, or you tick the rebase/retry checkbox.

🔕 **Ignore**: Close this PR and you won't be reminded about this update again.

---

 - [ ] <!-- rebase-check -->If you want to rebase/retry this PR, check this box

---

This PR has been generated by [Mend Renovate](https://github.com/renovatebot/renovate).
<!--renovate-debug:eyJjcmVhdGVkSW5WZXIiOiI0My4yNTcuNyIsInVwZGF0ZWRJblZlciI6IjQzLjI1Ny43IiwidGFyZ2V0QnJhbmNoIjoibWFpbiIsImxhYmVscyI6WyJtaW5vci11cGRhdGUiXX0=-->
