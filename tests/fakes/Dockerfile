# The cloud storage fakes for tests/integration/backends.sh (ADR 47). Stdlib only, so the
# build needs no module downloads.
FROM golang:1.27.3-trixie@sha256:80a86f24490f252511a683a1e1ee4c915b4a96e1f5519530f9ba1fdc9862b7c5 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go vet ./... && CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 go build -o /fakes .

FROM debian:trixie-20261027-slim@sha256:26c37952d274874448dd4f0943fc79bfb7c3d9cc8c8004ac4dc987489ca931be
COPY --from=build /fakes /usr/local/bin/fakes
ENTRYPOINT ["/usr/local/bin/fakes"]
