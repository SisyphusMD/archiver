# The cloud storage fakes for tests/integration/backends.sh (ADR 47). Stdlib only, so the
# build needs no module downloads.
FROM golang:1.27.3-trixie@sha256:c124b9ad3452f46061b5054607d26ea51730dae5372c5e469920063ba2bc6dcc AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go vet ./... && CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 go build -o /fakes .

FROM debian:trixie-20261027-slim@sha256:26c37952d274874448dd4f0943fc79bfb7c3d9cc8c8004ac4dc987489ca931be
COPY --from=build /fakes /usr/local/bin/fakes
ENTRYPOINT ["/usr/local/bin/fakes"]
