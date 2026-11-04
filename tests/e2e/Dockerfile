# Test runner for the black-box suite. It drives archiver containers through the host's
# Docker socket, so the images under test run as siblings, never nested.
FROM docker:29.9.3-cli@sha256:4d742ab934c2149b628bf26369ac3ec51cb91e2e28fe6722a0e86a6d6efc4bd5 AS dockercli

FROM golang:1.27.3-trixie@sha256:d9fa6e7ba43678ee1e5d1617007db9a134612032a74e2d86b44a9f582f1564cc
COPY --from=dockercli /usr/local/bin/docker /usr/local/bin/docker
WORKDIR /src
# CI points this at the NAS artifact cache; the default keeps the runner buildable anywhere.
ARG GOPROXY=https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN go mod download
COPY tests/e2e ./tests/e2e
RUN go vet ./tests/e2e/...
# The suite runs 7-9 minutes; go test's 10-minute default fails it on a busy runner.
ENTRYPOINT ["go", "test", "-count=1", "-timeout=30m", "./tests/e2e/..."]
