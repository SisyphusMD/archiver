# The cloud storage fakes for tests/integration/backends.sh (ADR 47). Stdlib only, so the
# build needs no module downloads.
FROM golang:1.27.3-trixie@sha256:2336eda24930976591abea806d9b7d99eeae38238f19755c5deec02facdc1594 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go vet ./... && CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 go build -o /fakes .

FROM debian:trixie-20261104-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2
COPY --from=build /fakes /usr/local/bin/fakes
ENTRYPOINT ["/usr/local/bin/fakes"]
