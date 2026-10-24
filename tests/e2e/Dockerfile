# Test runner for the black-box suite. It drives archiver containers through the host's
# Docker socket, so the images under test run as siblings, never nested.
FROM docker:29.9.2-cli@sha256:5e6baf67325a79f64ed3bed6863954d5e05d0f3d4e57cbdb14809346814bbfdb AS dockercli

FROM golang:1.27.3-trixie@sha256:fbb7ae40b62ff8afe31662df570d39b4f5052ac32b90dae8e60d7657a20a74f4
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
