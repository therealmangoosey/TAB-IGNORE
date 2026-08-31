# ADR 0001: Stack choices

Status: accepted

## Context
The target is a low-resource Galaxy Tab A / Termux environment with a 55 MB daemon+TUI
budget and a CGO-free static build for `linux/arm64`.

## Decision
- Go 1.24+, `CGO_ENABLED=0`.
- `modernc.org/sqlite` for persistence (pure Go, no CGO).
- `BurntSushi/toml` for commented, human-editable configuration.
- Bubble Tea + Lipgloss for keyboard-only TUI.
- Standard library `net/rpc/jsonrpc` over a Unix socket.
- Standard library `net/http` with `http.ServeContent` for Range streaming.
- `ffmpeg -c copy` only when available; never transcode.
- Out-of-tree source adapters are not bundled; only legitimate, documented
  providers ship.

## Alternatives rejected
- CGO SQLite (mattn) requires Android NDK/compiler.
- Node/Rust/Python runtimes exceed the memory budget.
- Web frontend is out of scope by design.
