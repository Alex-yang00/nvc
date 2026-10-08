#!/usr/bin/env bash
# Cross-compile release binaries into dist/ with SHA256SUMS.
# Usage: scripts/build.sh 0.1.0
set -euo pipefail
VERSION=${1:?usage: scripts/build.sh <version>}
cd "$(dirname "$0")/.."
rm -rf dist && mkdir -p dist
go vet ./...
go test ./...
for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64; do
  os=${target%/*} arch=${target#*/}
  out="dist/nvc_${VERSION}_${os}_${arch}"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$out/nvc" ./cmd/nvc
  tar -C "$out" -czf "$out.tar.gz" nvc
  rm -rf "$out"
done
(cd dist && sha256sum ./*.tar.gz | sed 's# \./# #' > SHA256SUMS)
ls -lh dist
