# hermit / hmt

`hermit` is a lightweight, terminal-first media library and downloader designed for Linux/arm64 and Termux. The project specification is in `PROJECT_PLAN (1).md`.

## Current state

The repository contains the main CLI, daemon/RPC layer, SQLite storage, provider system, downloader, local HTTP server, library tools, diagnostics, TUI, tests, and Termux-oriented build structure. CI builds a CGO-free Linux/arm64 binary.

## Termux setup

### 1. Install Termux packages

```sh
pkg update
pkg install git golang
```

For Android player handoff, install a compatible player such as VLC or mpv-android separately.

### 2. Clone the repository

```sh
git clone https://github.com/therealmangoosey/TAB-IGNORE.git
cd TAB-IGNORE
```

### 3. Build

```sh
go mod download
go test ./...
go build -trimpath -ldflags='-s -w' -o "$PREFIX/bin/hmt" ./cmd/hmt
```

Check the installation:

```sh
hmt version
hmt doctor
```

### 4. Configure

The default configuration is created from sensible Termux paths under `~/.hermit` and `~/Media/hermit`.

Create `~/.hermit/config.toml` only when you need overrides. Common environment variables are:

```sh
export HERMIT_STATE_DIR="$HOME/.hermit"
export HERMIT_LIBRARY_PATH="$HOME/Media/hermit"
export HERMIT_SERVER_ADDR="127.0.0.1:8788"
export HERMIT_RPC_SOCKET="$HOME/.hermit/hmt.sock"
export HERMIT_TMDB_KEY="your_tmdb_key"
```

Do not commit API keys or other secrets.

### 5. Start the daemon

```sh
hmt daemon start
hmt daemon status
```

Run the TUI with:

```sh
hmt
```

Run a one-shot command without the TUI, for example:

```sh
hmt doctor
hmt status
hmt lib scan
hmt ls
```

## Useful commands

```text
hmt                     open the TUI
hmt doctor              hardware/runtime diagnostics
hmt status              daemon and library status
hmt search <query>      metadata/provider search
hmt add <url>           queue a download
hmt get <url>           download a direct/HLS URL
hmt ls                  list jobs
hmt play <target>       hand media to an Android player
hmt lib list            list library files
hmt lib scan            scan the library
hmt lib secure          write .nomedia markers
hmt df                  show disk headroom
hmt sources             show provider status
hmt db vacuum           compact SQLite
hmt daemon start        start the background daemon
hmt daemon stop         stop the daemon
```

Most status/list commands also support `--json` for scripting.

## Performance

The design is intentionally conservative for Android/Termux: bounded network concurrency, capped download rate, streaming I/O, SQLite caching, no transcoding in the normal path, and no remote webpage JavaScript execution.

## Provider boundaries

The initial provider layer is allow-listed and modular. It supports local files, public-domain sources, generic user-supplied media URLs, and user-owned cloud/debrid integrations. It does not include piracy-site scraping, DRM bypassing, CAPTCHA bypassing, or Cloudflare/access-control circumvention.

## Development

Read these before making substantial changes:

- `PROJECT_PLAN (1).md` — product and architecture specification
- `README_AI.md` — coding/maintenance rules
- `TODO.txt` — remaining work
- `docs/README.md` — code map and safety boundaries

Before submitting changes:

```sh
gofmt -w .
go test ./...
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o hmt ./cmd/hmt
```
