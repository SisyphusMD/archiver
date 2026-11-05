# Test runner for the black-box suite. It drives archiver containers through the host's
# Docker socket, so the images under test run as siblings, never nested.
FROM docker:29.9.3-cli@sha256:4d742ab934c2149b628bf26369ac3ec51cb91e2e28fe6722a0e86a6d6efc4bd5 AS dockercli

FROM golang:1.27.3-trixie@sha256:ce01baa8d47f3917e015fd28dad23b42b7e2332070bf85d8ae3e9c2f035ef2da
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
