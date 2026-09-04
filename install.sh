#!/bin/sh
#
# chatgpt-mcp bootstrap installer for macOS / Linux.
#
# curl -fsSL https://get.mewis.me/chatgpt-mcp.sh | sh
# curl -fsSL https://get.mewis.me/chatgpt-mcp.sh | sh -s -- --no-alias
# curl -fsSL https://get.mewis.me/chatgpt-mcp.sh | sh -s -- --uninstall
#
# Environment:
#   CHATGPT_MCP_VERSION      release tag (default: latest)
#   CHATGPT_MCP_INSTALL_DIR  bundle location (default: ~/.chatgpt-mcp)
#   CHATGPT_MCP_BIN_DIR      command location (default: ~/.local/bin)
set -eu

REPO="mewisme/chatgpt-mcp"
INSTALL_DIR="${CHATGPT_MCP_INSTALL_DIR:-$HOME/.chatgpt-mcp}"
BIN_DIR="${CHATGPT_MCP_BIN_DIR:-$HOME/.local/bin}"
NO_ALIAS=0

for arg in "$@"; do
	case "$arg" in
		--no-alias) NO_ALIAS=1 ;;
		--uninstall)
			rm -f "$BIN_DIR/chatgpt-mcp" "$BIN_DIR/cgm" "$BIN_DIR/cmcp"
			rm -rf "$INSTALL_DIR"
			echo "chatgpt-mcp uninstalled from $INSTALL_DIR"
			exit 0
			;;
		*) echo "chatgpt-mcp: unknown installer option '$arg'." >&2; exit 1 ;;
	esac
done

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
	version="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" | sed -n 's#.*/releases/tag/##p')"
fi
if [ -z "$version" ]; then
	version="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"
fi
[ -n "$version" ] || {
	echo "chatgpt-mcp: could not resolve latest version; set CHATGPT_MCP_VERSION (e.g. v0.1.0)." >&2
	exit 1
}
case "$version" in v*) ;; *) version="v$version" ;; esac
ver="${version#v}"
asset="chatgpt-mcp_${ver}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$version/$asset"
checksums_url="https://github.com/$REPO/releases/download/$version/checksums.txt"

echo "Installing chatgpt-mcp $version ($os/$arch)..."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
archive="$tmp/$asset"
curl -fsSL "$url" -o "$archive" || {
	echo "chatgpt-mcp: download failed: $url" >&2
	exit 1
}
curl -fsSL "$checksums_url" -o "$tmp/checksums.txt" || {
	echo "chatgpt-mcp: checksum download failed: $checksums_url" >&2
	exit 1
}
expected="$(awk -v asset="$asset" '$2 == asset { print $1; exit }' "$tmp/checksums.txt")"
[ -n "$expected" ] || {
	echo "chatgpt-mcp: checksum missing for $asset" >&2
	exit 1
}
if command -v sha256sum >/dev/null 2>&1; then
	actual="$(sha256sum "$archive" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
	actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
else
	echo "chatgpt-mcp: sha256sum or shasum is required to verify the release archive" >&2
	exit 1
fi
[ "$actual" = "$expected" ] || {
	echo "chatgpt-mcp: checksum verification failed for $asset" >&2
	exit 1
}

extract="$tmp/extract"
mkdir -p "$extract"
tar -xzf "$archive" -C "$extract"
binary="$extract/chatgpt-mcp"
[ -f "$binary" ] || {
	echo "chatgpt-mcp: binary missing from archive." >&2
	exit 1
}
chmod +x "$binary"

if [ "$NO_ALIAS" -eq 1 ]; then
	"$binary" install --no-alias
else
	"$binary" install
fi

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
if [ "$NO_ALIAS" -eq 1 ]; then
	echo "Done. Run: chatgpt-mcp --help"
else
	echo "Done. Run: chatgpt-mcp --help or cgm --help"
fi
