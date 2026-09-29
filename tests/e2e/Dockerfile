# Test runner for the black-box suite. It drives archiver containers through the host's
# Docker socket, so the images under test run as siblings, never nested.
FROM docker:29.8.1-cli@sha256:018edbc908e08fcc9dbf029c812c34251e9b4719e6f71ca0e5eae2a987d014ca AS dockercli

FROM golang:1.27.1-trixie@sha256:433790e515d27dc6003e847e644cc0af956985cf315c1c58a3b73ee2dd305183
COPY --from=dockercli /usr/local/bin/docker /usr/local/bin/docker
WORKDIR /src
COPY go.mod ./
COPY tests/e2e ./tests/e2e
RUN go vet ./tests/e2e/...
ENTRYPOINT ["go", "test", "-count=1", "./tests/e2e/..."]
