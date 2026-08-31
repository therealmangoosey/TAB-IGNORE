#!/data/data/com.termux/files/usr/bin/sh
# Termux:Boot respawn: start the daemon after reboot.
# Copy to $PREFIX/etc/termux/boot/hermit-boot.sh and make it executable.

set -eu

# Give Android a moment to mount shared storage after boot.
sleep 10

if ! pgrep -f 'hmt daemon run' >/dev/null 2>&1; then
  hmt daemon start 2>/dev/null || true
fi

case "${TERMUX_BOOT_TRIGGER:-boot}" in
  boot|boot_completed|*)
    if command -v termux-wake-lock >/dev/null 2>&1; then
      termux-wake-lock 2>/dev/null || true
    fi
    ;;
esac
