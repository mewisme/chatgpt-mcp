#!/bin/sh
#
# chatgpt-mcp installer for macOS / Linux.
#
# curl -fsSL https://raw.githubusercontent.com/mewisme/chatgpt-mcp/main/install.sh | sh
#
# Uninstall:
# curl -fsSL https://raw.githubusercontent.com/mewisme/chatgpt-mcp/main/install.sh | sh -s -- --uninstall
#
# Environment:
#   CHATGPT_MCP_VERSION      release tag (default: latest)
#   CHATGPT_MCP_INSTALL_DIR  bundle location (default: ~/.chatgpt-mcp)
#   CHATGPT_MCP_BIN_DIR      symlink location (default: ~/.local/bin)
set -eu

REPO="mewisme/chatgpt-mcp"
INSTALL_DIR="${CHATGPT_MCP_INSTALL_DIR:-$HOME/.chatgpt-mcp}"
BIN_DIR="${CHATGPT_MCP_BIN_DIR:-$HOME/.local/bin}"

if [ "${1:-}" = "--uninstall" ]; then
	rm -f "$BIN_DIR/chatgpt-mcp"
	rm -rf "$INSTALL_DIR"
	echo "chatgpt-mcp uninstalled (removed $INSTALL_DIR and $BIN_DIR/chatgpt-mcp)."
	exit 0
fi

os="$(uname -s)"
arch="$(uname -m)"
case "$os" in
	Darwin) os="darwin" ;;
	Linux) os="linux" ;;
	*) echo "chatgpt-mcp: unsupported OS '$os'." >&2; exit 1 ;;
esac
case "$arch" in
	arm64|aarch64) arch="arm64" ;;
	x86_64|amd64) arch="amd64" ;;
	*) echo "chatgpt-mcp: unsupported architecture '$arch'." >&2; exit 1 ;;
esac

version="${CHATGPT_MCP_VERSION:-}"
if [ -z "$version" ]; then
	version="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" \
		| sed -n 's#.*/releases/tag/##p')"
fi
if [ -z "$version" ]; then
	version="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
		| sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"
fi
[ -n "$version" ] || {
	echo "chatgpt-mcp: could not resolve latest version; set CHATGPT_MCP_VERSION (e.g. v0.1.0)." >&2
	exit 1
}
case "$version" in v*) ;; *) version="v$version" ;; esac
ver="${version#v}"

url="https://github.com/$REPO/releases/download/$version/chatgpt-mcp_${ver}_${os}_${arch}.tar.gz"
echo "Installing chatgpt-mcp $version ($os/$arch)..."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$url" -o "$tmp/chatgpt-mcp.tar.gz" || {
	echo "chatgpt-mcp: download failed: $url" >&2
	exit 1
}

dest="$INSTALL_DIR/versions/$version"
rm -rf "$dest"
mkdir -p "$dest"
tar -xzf "$tmp/chatgpt-mcp.tar.gz" -C "$dest"
[ -f "$dest/chatgpt-mcp" ] || {
	echo "chatgpt-mcp: binary missing from archive." >&2
	exit 1
}
chmod +x "$dest/chatgpt-mcp"

mkdir -p "$BIN_DIR"
ln -sf "$dest/chatgpt-mcp" "$BIN_DIR/chatgpt-mcp"
ln -sfn "$dest" "$INSTALL_DIR/current"

if [ -d "$INSTALL_DIR/versions" ]; then
	for d in "$INSTALL_DIR/versions"/*; do
		[ -d "$d" ] || continue
		[ "$d" = "$dest" ] || rm -rf "$d"
	done
fi

echo "Installed to $dest"
echo "Linked $BIN_DIR/chatgpt-mcp"

on_path=0
oldifs="$IFS"
IFS=:
for dir in $PATH; do
	[ "$dir" = "$BIN_DIR" ] && on_path=1 && break
done
IFS="$oldifs"

if [ "$on_path" -eq 0 ]; then
	echo ""
	echo "$BIN_DIR is not on your PATH. Add it:"
	echo "  export PATH=\"$BIN_DIR:\$PATH\""
fi

echo ""
echo "Done. Run: chatgpt-mcp --help"
