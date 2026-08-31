# hermit developer docs

This directory holds design notes and architecture records. The product
specification lives in `PROJECT_PLAN (1).md`; `README_AI.md` is the coding
handoff.

## Code map

- `cmd/hmt` — CLI/TUI dispatch, no business logic.
- `internal/cfg` — TOML + env + flag merge.
- `internal/db` — SQLite storage with embedded migrations.
- `internal/meta` — TMDB metadata client with cache.
- `internal/provider` — explicit, allow-listed providers (localfs, archive.org,
  user-supplied URL, user-owned debrid link).
- `internal/fetch` — Range downloads, HLS parsing, bounded rate, SHA-256.
- `internal/queue` — persisted job scheduler and retry.
- `internal/lib` — library scanning, `.nomedia`, reclaim, sidecars.
- `internal/srv` — local HTTP Range stream and `/api` surface.
- `internal/rpc` — Unix-socket JSON-RPC used by the TUI and scripts.
- `internal/doctor` — platform diagnostics.
- `internal/tui` — Bubble Tea keyboard-only interface.
- `pkg/hermit` — public data model/RPC types.

## Safety boundaries

- The registry only instantiates `localfs`, `archiveorg`, `genericm3u8`, and
  `debrid`. There are no scraping adapters and no `need_js` path.
- The HTTP transport enforces an origin allow-list and re-checks every redirect.
- Cookies are never persisted.
- No third-party page JavaScript is executed.
