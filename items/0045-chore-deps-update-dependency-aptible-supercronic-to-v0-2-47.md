# #45 chore(deps): update dependency aptible/supercronic to v0.2.47

- Kind: pull, state: closed, by cody, created 2026-07-06T21:03:39-07:00
- Labels: none; milestone: none
- refs/pull/45/head -> main, merged: True

This PR contains the following updates:

| Package | Update | Change |
|---|---|---|
| [aptible/supercronic](https://github.com/aptible/supercronic) | patch | `v0.2.45` → `v0.2.47` |

---

### Release Notes

<details>
<summary>aptible/supercronic (aptible/supercronic)</summary>

### [`v0.2.47`](https://github.com/aptible/supercronic/releases/tag/v0.2.47)

[Compare Source](https://github.com/aptible/supercronic/compare/v0.2.46...v0.2.47)

#### What's Changed

- Update packages & go version by [@&#8203;almathew](https://github.com/almathew) in [#&#8203;238](https://github.com/aptible/supercronic/pull/238)

**Full Changelog**: <https://github.com/aptible/supercronic/compare/v0.2.46...v0.2.47>

### Installation Instructions

#### supercronic-linux-amd64

Add the following stanza to your Dockerfile to install `supercronic-linux-amd64`.

You will need to install `curl` beforehand.

```

# Latest releases available at https://github.com/aptible/supercronic/releases
ENV SUPERCRONIC_URL=https://github.com/aptible/supercronic/releases/download/v0.2.47/supercronic-linux-amd64 \
    SUPERCRONIC_SHA1SUM=712d2ece75da6f6e530192a151488578153e4e96 \
    SUPERCRONIC=supercronic-linux-amd64

RUN curl -fsSLO "$SUPERCRONIC_URL" \
 && echo "${SUPERCRONIC_SHA1SUM}  ${SUPERCRONIC}" | sha1sum -c - \
 && chmod +x "$SUPERCRONIC" \
 && mv "$SUPERCRONIC" "/usr/local/bin/${SUPERCRONIC}" \
 && ln -s "/usr/local/bin/${SUPERCRONIC}" /usr/local/bin/supercronic
```

#### supercronic-linux-386

Add the following stanza to your Dockerfile to install `supercronic-linux-386`.

You will need to install `curl` beforehand.

```

# Latest releases available at https://github.com/aptible/supercronic/releases
ENV SUPERCRONIC_URL=https://github.com/aptible/supercronic/releases/download/v0.2.47/supercronic-linux-386 \
    SUPERCRONIC_SHA1SUM=c3c8bd5d200c2b55cf2f9590542139f205194fb7 \
    SUPERCRONIC=supercronic-linux-386

RUN curl -fsSLO "$SUPERCRONIC_URL" \
 && echo "${SUPERCRONIC_SHA1SUM}  ${SUPERCRONIC}" | sha1sum -c - \
 && chmod +x "$SUPERCRONIC" \
 && mv "$SUPERCRONIC" "/usr/local/bin/${SUPERCRONIC}" \
 && ln -s "/usr/local/bin/${SUPERCRONIC}" /usr/local/bin/supercronic
```

#### supercronic-linux-arm

Add the following stanza to your Dockerfile to install `supercronic-linux-arm`.

You will need to install `curl` beforehand.

```

# Latest releases available at https://github.com/aptible/supercronic/releases
ENV SUPERCRONIC_URL=https://github.com/aptible/supercronic/releases/download/v0.2.47/supercronic-linux-arm \
    SUPERCRONIC_SHA1SUM=dcc8535b46bd9752fbbe177fddd7f6f0451da9a4 \
    SUPERCRONIC=supercronic-linux-arm

RUN curl -fsSLO "$SUPERCRONIC_URL" \
 && echo "${SUPERCRONIC_SHA1SUM}  ${SUPERCRONIC}" | sha1sum -c - \
 && chmod +x "$SUPERCRONIC" \
 && mv "$SUPERCRONIC" "/usr/local/bin/${SUPERCRONIC}" \
 && ln -s "/usr/local/bin/${SUPERCRONIC}" /usr/local/bin/supercronic
```

#### supercronic-linux-arm64

Add the following stanza to your Dockerfile to install `supercronic-linux-arm64`.

You will need to install `curl` beforehand.

```

# Latest releases available at https://github.com/aptible/supercronic/releases
ENV SUPERCRONIC_URL=https://github.com/aptible/supercronic/releases/download/v0.2.47/supercronic-linux-arm64 \
    SUPERCRONIC_SHA1SUM=93323899ddca3f1198f1796a4bf4418ed1e7982e \
    SUPERCRONIC=supercronic-linux-arm64

RUN curl -fsSLO "$SUPERCRONIC_URL" \
 && echo "${SUPERCRONIC_SHA1SUM}  ${SUPERCRONIC}" | sha1sum -c - \
 && chmod +x "$SUPERCRONIC" \
 && mv "$SUPERCRONIC" "/usr/local/bin/${SUPERCRONIC}" \
 && ln -s "/usr/local/bin/${SUPERCRONIC}" /usr/local/bin/supercronic
```

### [`v0.2.46`](https://github.com/aptible/supercronic/releases/tag/v0.2.46)

[Compare Source](https://github.com/aptible/supercronic/compare/v0.2.45...v0.2.46)

### Installation Instructions

#### supercronic-linux-amd64

Add the following stanza to your Dockerfile to install `supercronic-linux-amd64`.

You will need to install `curl` beforehand.

```

# Latest releases available at https://github.com/aptible/supercronic/releases
ENV SUPERCRONIC_URL=https://github.com/aptible/supercronic/releases/download/v0.2.46/supercronic-linux-amd64 \
    SUPERCRONIC_SHA1SUM=5bcefed628e32adc08e32634db2d10e9230dbca0 \
    SUPERCRONIC=supercronic-linux-amd64

RUN curl -fsSLO "$SUPERCRONIC_URL" \
 && echo "${SUPERCRONIC_SHA1SUM}  ${SUPERCRONIC}" | sha1sum -c - \
 && chmod +x "$SUPERCRONIC" \
 && mv "$SUPERCRONIC" "/usr/local/bin/${SUPERCRONIC}" \
 && ln -s "/usr/local/bin/${SUPERCRONIC}" /usr/local/bin/supercronic
```

#### supercronic-linux-386

Add the following stanza to your Dockerfile to install `supercronic-linux-386`.

You will need to install `curl` beforehand.

```

# Latest releases available at https://github.com/aptible/supercronic/releases
ENV SUPERCRONIC_URL=https://github.com/aptible/supercronic/releases/download/v0.2.46/supercronic-linux-386 \
    SUPERCRONIC_SHA1SUM=7630904df9a240b6e0f8171cac46ec799fbe1ad9 \
    SUPERCRONIC=supercronic-linux-386

RUN curl -fsSLO "$SUPERCRONIC_URL" \
 && echo "${SUPERCRONIC_SHA1SUM}  ${SUPERCRONIC}" | sha1sum -c - \
 && chmod +x "$SUPERCRONIC" \
 && mv "$SUPERCRONIC" "/usr/local/bin/${SUPERCRONIC}" \
 && ln -s "/usr/local/bin/${SUPERCRONIC}" /usr/local/bin/supercronic
```

#### supercronic-linux-arm

Add the following stanza to your Dockerfile to install `supercronic-linux-arm`.

You will need to install `curl` beforehand.

```

# Latest releases available at https://github.com/aptible/supercronic/releases
ENV SUPERCRONIC_URL=https://github.com/aptible/supercronic/releases/download/v0.2.46/supercronic-linux-arm \
    SUPERCRONIC_SHA1SUM=04be50a960eafa325185d037c9b4ad6cd88a0e0b \
    SUPERCRONIC=supercronic-linux-arm

RUN curl -fsSLO "$SUPERCRONIC_URL" \
 && echo "${SUPERCRONIC_SHA1SUM}  ${SUPERCRONIC}" | sha1sum -c - \
 && chmod +x "$SUPERCRONIC" \
 && mv "$SUPERCRONIC" "/usr/local/bin/${SUPERCRONIC}" \
 && ln -s "/usr/local/bin/${SUPERCRONIC}" /usr/local/bin/supercronic
```

#### supercronic-linux-arm64

Add the following stanza to your Dockerfile to install `supercronic-linux-arm64`.

You will need to install `curl` beforehand.

```

# Latest releases available at https://github.com/aptible/supercronic/releases
ENV SUPERCRONIC_URL=https://github.com/aptible/supercronic/releases/download/v0.2.46/supercronic-linux-arm64 \
    SUPERCRONIC_SHA1SUM=639ab81a72771990790df7ee87d9acfe88e5fa83 \
    SUPERCRONIC=supercronic-linux-arm64

RUN curl -fsSLO "$SUPERCRONIC_URL" \
 && echo "${SUPERCRONIC_SHA1SUM}  ${SUPERCRONIC}" | sha1sum -c - \
 && chmod +x "$SUPERCRONIC" \
 && mv "$SUPERCRONIC" "/usr/local/bin/${SUPERCRONIC}" \
 && ln -s "/usr/local/bin/${SUPERCRONIC}" /usr/local/bin/supercronic
```

</details>

---

### Configuration

📅 **Schedule**: (UTC)

- Branch creation
  - At any time (no schedule defined)
- Automerge
  - At any time (no schedule defined)

🚦 **Automerge**: Enabled.

♻ **Rebasing**: Whenever PR is behind base branch, or you tick the rebase/retry checkbox.

🔕 **Ignore**: Close this PR and you won't be reminded about this update again.

---

 - [ ] <!-- rebase-check -->If you want to rebase/retry this PR, check this box

---

This PR has been generated by [Mend Renovate](https://github.com/renovatebot/renovate).
<!--renovate-debug:eyJjcmVhdGVkSW5WZXIiOiI0My4yNTMuMCIsInVwZGF0ZWRJblZlciI6IjQzLjI1My4wIiwidGFyZ2V0QnJhbmNoIjoibWFpbiIsImxhYmVscyI6WyJwYXRjaC11cGRhdGUiXX0=-->
