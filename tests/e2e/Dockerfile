# Test runner for the black-box suite. It drives archiver containers through the host's
# Docker socket, so the images under test run as siblings, never nested.
FROM docker:29.10.1-cli@sha256:4e60c31615e83523afbb59773b25d055c0da9edcd5f80a4c6a6eebad49de1a25 AS dockercli

FROM golang:1.27.4-trixie@sha256:430985542100b871e108ccc00e8dcc5eef08ca14ee5825e98ebf563d8ffec4ee
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
