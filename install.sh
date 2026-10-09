#!/bin/sh
# nvc installer: curl -fsSL https://github.com/Alex-yang00/nvc/releases/latest/download/install.sh | sh
# Env: NVC_VERSION (default: latest release), NVC_REPO (default Alex-yang00/nvc),
#      NVC_BASE_URL (mirror serving <base>/v<version>/<asset>), NVC_INSTALL_DIR (default ~/.local/bin)
set -eu

REPO=${NVC_REPO:-Alex-yang00/nvc}
BASE_URL=${NVC_BASE_URL:-"https://github.com/$REPO/releases/download"}
VERSION=${NVC_VERSION:-}
DIR=${NVC_INSTALL_DIR:-"$HOME/.local/bin"}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in darwin|linux) ;; *) echo "nvc: unsupported OS: $os" >&2; exit 1 ;; esac
arch=$(uname -m)
case "$arch" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo "nvc: unsupported arch: $arch" >&2; exit 1 ;; esac

if [ -z "$VERSION" ]; then
  [ -z "${NVC_BASE_URL:-}" ] || { echo "nvc: NVC_VERSION is required with NVC_BASE_URL" >&2; exit 1; }
  # /releases/latest redirects to /releases/tag/v<version>
  latest=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest")
  VERSION=${latest##*/v}
  case "$VERSION" in ""|*/*) echo "nvc: could not determine latest release of $REPO" >&2; exit 1 ;; esac
fi

name="nvc_${VERSION}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading nvc $VERSION ($os/$arch)..."
curl -fsSL "$BASE_URL/v$VERSION/$name" -o "$tmp/$name"
curl -fsSL "$BASE_URL/v$VERSION/SHA256SUMS" -o "$tmp/SHA256SUMS"

want=$(grep " $name\$" "$tmp/SHA256SUMS" | cut -d' ' -f1)
if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "$tmp/$name" | cut -d' ' -f1)
else got=$(shasum -a 256 "$tmp/$name" | cut -d' ' -f1); fi
if [ -z "$want" ] || [ "$want" != "$got" ]; then echo "nvc: checksum mismatch, aborting" >&2; exit 1; fi

mkdir -p "$DIR"
tar -xzf "$tmp/$name" -C "$tmp"
install -m 0755 "$tmp/nvc" "$DIR/nvc"
echo "✓ installed $DIR/nvc ($("$DIR/nvc" version))"

case ":$PATH:" in *":$DIR:"*) ;; *) echo "  add to PATH: export PATH=\"$DIR:\$PATH\"" ;; esac
echo "  next: nvc   (menu)  ·  nvc uninstall to remove"
