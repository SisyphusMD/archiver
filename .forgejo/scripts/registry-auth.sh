#!/usr/bin/env bash
# registry-auth.sh VOLUME IMAGE HOST=USER:VAR...
#
# Writes VOLUME:/auth.json, a registry login file (docker and skopeo read the same format)
# with a login per HOST, its token read from the environment variable VAR, for skopeo's
# --src-authfile and --dest-authfile. No token is ever a command-line argument (docker's or
# skopeo's), where another process on the runner could read it: the file is built with
# shell builtins and reaches the volume on stdin.
set -euo pipefail

vol=$1 image=$2
shift 2
json='{"auths":{'
sep=
for spec in "$@"; do
	host=${spec%%=*}
	rest=${spec#*=}
	user=${rest%%:*}
	var=${rest#*:}
	[ -n "${!var:-}" ] || { echo "registry-auth: $var is empty" >&2; exit 1; }
	auth=$(printf '%s:%s' "$user" "${!var}" | base64 -w0)
	json+="${sep}\"${host}\":{\"auth\":\"${auth}\"}"
	sep=,
done
json+='}}'
docker volume create "$vol" >/dev/null
printf '%s' "$json" | docker run --rm -i -v "$vol":/auth --entrypoint sh "$image" -c 'umask 077 && cat > /auth/auth.json'
