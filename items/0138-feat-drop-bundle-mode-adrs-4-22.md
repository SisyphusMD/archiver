# #138 feat!: drop bundle mode (ADRs 4, 22)

- Kind: pull, state: closed, by cody, created 2026-10-04T14:21:50-07:00
- Labels: none; milestone: none
- refs/pull/138/head -> main, merged: True

v1 reads no bundles. A mounted bundle.tar.enc, a config.sh, a bundle_password secret or BUNDLE_PASSWORD refuses to start and prints the conversion: one `docker run` of the 0.11 image (`run migrate`, released in 0.11.4). Config is env vars + secret files only (config-vars.sh shared by loader and serializer, no file sourced); init writes env-native materials only; bundle export/import and migrate removed (migrate hooks stays). The e2e cross-version test converts current and legacy -k bundles with the pinned 0.11.4 digest and continues the same snapshot IDs on this image. New refuse-bundle host test; bundle-secret, migrate and entrypoint-secrets tests removed. Docs, README (Upgrading from a bundle), compose template and CHANGELOG updated.
