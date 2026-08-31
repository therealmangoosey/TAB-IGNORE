# Maintainer / coding handoff

Read this file before making implementation changes.

1. Treat `PROJECT_PLAN (1).md` as the product specification and source of truth for architecture and performance goals.
2. Keep the project lightweight on Android/Termux. Do not add a framework or service unless it has a clear benefit.
3. Keep business logic out of `cmd/hmt/main.go`; put implementation under `internal/`.
4. Preserve a CGO-free Linux/arm64 build path.
5. Never execute third-party webpage JavaScript, register remote service workers, or persist third-party cookies as part of source resolution.
6. Provider code must be allow-listed and configurable. Initial adapters are limited to local files, documented public-domain sources, generic user-supplied media URLs, and user-owned cloud/debrid integrations.
7. Do not add scraping logic for sites that are not explicitly supported and documented.
8. Do not add transcoding, BitTorrent, Docker, a web frontend, or a multi-user server to the base scope.
9. Add tests and fixtures before claiming a subsystem is complete.
10. Keep CLI output usable on small Termux terminals and provide machine-readable output for automation as the design evolves.

Before merging substantial work, verify:
- `gofmt` has been applied.
- `go test ./...` passes.
- `go vet ./...` passes where supported.
- Linux/arm64 compilation succeeds.
- No secret/API key has been committed.
