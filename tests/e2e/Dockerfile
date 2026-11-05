# Test runner for the black-box suite. It drives archiver containers through the host's
# Docker socket, so the images under test run as siblings, never nested.
FROM docker:29.9.3-cli@sha256:2357639ad1ea7d0194134bf3292622fd4e562483966ed674e5d7c945b5f3b28d AS dockercli

FROM golang:1.27.3-trixie@sha256:af1bfc2614ebb1dbd31af514b78740fd678850e76498762b425ecee4c5c6ea80
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
