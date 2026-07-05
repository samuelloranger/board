#!/bin/sh
# board installer — downloads the binary and registers it across AI clients.
set -eu

REPO="samuelloranger/board"
INSTALL_DIR="${BOARD_HOME:-$HOME/.board}/bin"
BIN="$INSTALL_DIR/board"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux|darwin) : ;;
  *) echo "unsupported os: $os" >&2; exit 1 ;;
esac
target="board_${os}_${arch}"
echo "target: $target"

pass_yes=""
for a in "$@"; do [ "$a" = "--yes" ] && pass_yes="--yes"; done

if [ "${BOARD_DRY_RUN:-}" = "1" ]; then
  echo "would download: https://github.com/$REPO/releases/latest/download/$target"
  echo "would run: $BIN setup $pass_yes"
  exit 0
fi

mkdir -p "$INSTALL_DIR"
if [ "${BOARD_SKIP_DOWNLOAD:-}" != "1" ]; then
  url="https://github.com/$REPO/releases/latest/download/$target"
  echo "downloading $url"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$BIN"
  else
    wget -qO "$BIN" "$url"
  fi
  chmod +x "$BIN"
fi

echo "installed to $BIN"
"$BIN" setup $pass_yes
echo "done. Restart your AI clients to load the board MCP server."
