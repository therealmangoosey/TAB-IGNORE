# hermit / hmt

This repository is the implementation base for the `hermit` media-library CLI described in `PROJECT_PLAN (1).md`.

## Base status

The initial commit intentionally establishes the project shape, build entrypoint, safety boundaries, documentation handoff points, and CI scaffold. It is not presented as a completed media application.

## Intended direction

- Go 1.24+, CGO-free, Linux/arm64 first.
- Termux-friendly and resource-conscious.
- Terminal/TUI only; no remote webpage execution.
- Metadata adapters should use documented APIs.
- Provider adapters must be explicit and allow-listed.
- Local/public-domain/self-owned media paths are the initial implementation targets.

## Documentation handoff

Read `docs/README.md` and `README_AI.md` before extending the codebase. `TODO.txt` records the work that remains after the base commit.
