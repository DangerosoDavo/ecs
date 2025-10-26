#!/usr/bin/env bash

set -euo pipefail

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is not available on PATH; install Docker to use this script." >&2
  exit 1
fi

IMAGE="${ECS_GO_IMAGE:-golang:1.25.3-alpine3.22}"
WORKDIR="/workspace"
APK_PACKAGES="${ECS_GO_APK_PACKAGES:-build-base}"

# Allow callers to forward additional go test arguments, defaulting to -race ./...
if [ "$#" -gt 0 ]; then
  GO_TEST_ARGS=("$@")
else
  GO_TEST_ARGS=(-race ./...)
fi

docker run --rm \
  -v "$PWD":"$WORKDIR" \
  -w "$WORKDIR" \
  -e CGO_ENABLED=1 \
  -e GOCACHE=/tmp/go-build \
  "$IMAGE" \
  sh -ec 'apk add --no-cache '"$APK_PACKAGES"' >/dev/null && go test "$@"' go-test "${GO_TEST_ARGS[@]}"
