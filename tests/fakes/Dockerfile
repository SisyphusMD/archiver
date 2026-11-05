# The cloud storage fakes for tests/integration/backends.sh (ADR 47). Stdlib only, so the
# build needs no module downloads.
FROM golang:1.27.3-trixie@sha256:af1bfc2614ebb1dbd31af514b78740fd678850e76498762b425ecee4c5c6ea80 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go vet ./... && CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 go build -o /fakes .

FROM debian:trixie-20261027-slim@sha256:f6b08b09a05725f9431502d3b5714f5c4576e82c1a84fa1debe0c24a7e0c3ec4
COPY --from=build /fakes /usr/local/bin/fakes
ENTRYPOINT ["/usr/local/bin/fakes"]
