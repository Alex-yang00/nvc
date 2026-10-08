#!/bin/sh
# nvc installer.
#   Private repo (default): gh release download -R Alex-yang00/nvc -p install.sh -O - | sh
#   Plain HTTP mirror:      curl -fsSL <mirror>/install.sh | NVC_BASE_URL=<mirror> sh
# Env: NVC_VERSION (default: latest release), NVC_REPO (default Alex-yang00/nvc),
#      NVC_BASE_URL (download via curl from <base>/v<version>/ instead of gh), NVC_INSTALL_DIR (default ~/.local/bin)
set -eu

REPO=${NVC_REPO:-Alex-yang00/nvc}
VERSION=${NVC_VERSION:-}
DIR=${NVC_INSTALL_DIR:-"$HOME/.local/bin"}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in darwin|linux) ;; *) echo "nvc: unsupported OS: $os" >&2; exit 1 ;; esac
arch=$(uname -m)
case "$arch" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo "nvc: unsupported arch: $arch" >&2; exit 1 ;; esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if [ -n "${NVC_BASE_URL:-}" ]; then
  [ -n "$VERSION" ] || { echo "nvc: NVC_VERSION is required with NVC_BASE_URL" >&2; exit 1; }
  name="nvc_${VERSION}_${os}_${arch}.tar.gz"
  echo "Downloading nvc $VERSION ($os/$arch)..."
  curl -fsSL "$NVC_BASE_URL/v$VERSION/$name" -o "$tmp/$name"
  curl -fsSL "$NVC_BASE_URL/v$VERSION/SHA256SUMS" -o "$tmp/SHA256SUMS"
else
  command -v gh >/dev/null 2>&1 || { echo "nvc: needs the GitHub CLI (gh) to download from private repo $REPO — brew install gh && gh auth login" >&2; exit 1; }
  tag=${VERSION:+v$VERSION}
  if [ -z "$tag" ]; then
    tag=$(gh release view -R "$REPO" --json tagName -q .tagName) || { echo "nvc: cannot read releases of $REPO (gh auth login?)" >&2; exit 1; }
  fi
  VERSION=${tag#v}
  name="nvc_${VERSION}_${os}_${arch}.tar.gz"
  echo "Downloading nvc $VERSION ($os/$arch) from $REPO..."
  gh release download "$tag" -R "$REPO" -D "$tmp" -p "$name" -p SHA256SUMS
fi

want=$(grep " $name\$" "$tmp/SHA256SUMS" | cut -d' ' -f1)
if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "$tmp/$name" | cut -d' ' -f1)
else got=$(shasum -a 256 "$tmp/$name" | cut -d' ' -f1); fi
if [ -z "$want" ] || [ "$want" != "$got" ]; then echo "nvc: checksum mismatch, aborting" >&2; exit 1; fi

mkdir -p "$DIR"
tar -xzf "$tmp/$name" -C "$tmp"
install -m 0755 "$tmp/nvc" "$DIR/nvc"
echo "✓ installed $DIR/nvc ($("$DIR/nvc" version))"

case ":$PATH:" in *":$DIR:"*) ;; *) echo "  add to PATH: export PATH=\"$DIR:\$PATH\"" ;; esac
echo "  next: nvc login && nvc claude"
