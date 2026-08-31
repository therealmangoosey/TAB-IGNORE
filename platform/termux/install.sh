#!/data/data/com.termux/files/usr/bin/sh
# hermit Termux installer.
#
# This installs the dependencies and configures the battery/storage settings
# needed for a background daemon. You must run termux-setup-storage after
# installing Termux from F-Droid (not the Play Store build).

set -eu

HMT_BIN="${HERMIT_INSTALL_BIN:-$PREFIX/bin/hmt}"
HMT_ARCH="${HMT_ARCH:-$(uname -m)}"

pkg update -y
pkg install -y curl ripgrep tmux ffmpeg termux-api

# Use the release asset for the current ABI.
case "$HMT_ARCH" in
  aarch64) goarch="arm64" ;;
  armv7l|armv8l) goarch="arm" ;;
  x86_64) goarch="amd64" ;;
  *) echo "unsupported arch: $HMT_ARCH"; exit 1 ;;
esac

mkdir -p "$HOME/.hermit/logs" "$HOME/.hermit/cache" "$HOME/.hermit/tmp"
mkdir -p "$HOME/Media/hermit"

if [ -n "${HERMIT_RELEASE_URL:-}" ]; then
  curl -fsSL "$HERMIT_RELEASE_URL" -o "$HMT_BIN"
  chmod +x "$HMT_BIN"
else
  echo "HERMIT_RELEASE_URL not set; unable to download a binary."
  echo "Build it with: CGO_ENABLED=0 GOOS=linux GOARCH=$goarch go build -o hmt ./cmd/hmt"
  echo "Then copy the binary to $HMT_BIN."
  exit 2
fi

termux-setup-storage || true
cat <<'EOF'

hermit installed.

Samsung / Android setup (required for a background daemon):
  1. Settings -> Apps -> Termux -> Unrestricted (or "Battery -> Do not optimize").
  2. Add Termux to Settings -> Apps -> Special access -> Ignore battery optimizations.
  3. Termux:Boot is installed and enabled, then:
       cp platform/termux/boot.sh $PREFIX/etc/termux/boot/hermit-boot.sh
  4. Run `hmt doctor` to verify storage, ffmpeg, and provider reachability.
EOF
