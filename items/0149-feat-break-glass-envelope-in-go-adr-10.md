# #149 feat: break-glass envelope in Go (ADR 10)

- Kind: pull, state: closed, by cody, created 2026-10-05T15:46:16-07:00
- Labels: none; milestone: none
- refs/pull/149/head -> main, merged: True

ADR 10 step 3, envelope: `archiver envelope [DIR]`, `envelope confirm`, and the envelope check after each kit refresh run in Go (`internal/envelope`: page, fingerprint, HTML, PDF, check/confirm). `lib/scripts/envelope-check.sh` is gone; the kit calls the check in-process.

- `tests/integration/envelope-parity.sh`: for local + SFTP (break-glass key and user) + B2 (break-glass) + S3 (backup credentials, non-ASCII), bash and Go give the same fingerprint and byte-identical HTML and PDF; also with text the PDF fonts cannot show (no PDF from either) and a credential too long for a QR code. A page confirmed under bash is current under Go and confirms there.
- The PDF layout is a line-by-line port of `envelope-pdf.awk` (awk word splitting, Windows-1252 byte widths, the QR float). qrencode is still the QR source.

The bash envelope stays until the bash pipeline goes (step 4).
