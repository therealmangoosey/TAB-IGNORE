# hermit / hmt

`hermit` is a lightweight, terminal-first media library and downloader designed for Linux/arm64 and Termux. The project specification is in `PROJECT_PLAN (1).md`.

## Current state

The repository contains the main CLI, daemon/RPC layer, SQLite storage, provider system, downloader, local HTTP server, LAN UPnP/DLNA media server, library tools, diagnostics, TUI, tests, and Termux-oriented build structure. CI builds a CGO-free Linux/arm64 binary.

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

## Smart TV / LAN media server

Hermit now runs a lightweight UPnP/DLNA-style media server alongside the daemon. Compatible Smart TVs and media players can discover **Hermit** on the same LAN, browse the library, and stream files directly from the device.

The media server uses its own HTTP listener so the normal loopback control API does not need to be exposed to the network. The default media-server listener is `0.0.0.0:8789` and SSDP discovery uses the standard `239.255.255.250:1900` multicast group. The media server advertises a `MediaServer:1` device with a ContentDirectory service and provides browsable folders plus direct HTTP video resources with Range support. UPnP MediaServer devices require a ContentDirectory service, and the Browse action is the standard mechanism used to enumerate content. citeturn540975search12turn540975search13

Start Hermit's normal daemon as usual:

```sh
hmt daemon start
```

Then open the TV's **Media Server**, **Network**, **DLNA**, or equivalent section. Look for:

```text
Hermit
```

The library is browsed from the actual folders under your configured library path. Media files supported by the server are `.mp4`, `.m4v`, `.mkv`, `.webm`, and `.ts`.

The server is enabled by default. Disable it when you do not want LAN discovery:

```sh
export HERMIT_MEDIA_SERVER=0
```

Change its name:

```sh
export HERMIT_MEDIA_SERVER_NAME="My Hermit Server"
```

Change its HTTP listener if port `8789` is already in use:

```sh
export HERMIT_MEDIA_SERVER_ADDR="0.0.0.0:8899"
```

The TV must be on the same local network as the Termux device, and Android/network settings must allow local multicast traffic. Compatibility still depends on the TV's supported DLNA profiles and codecs. Hermit performs direct streaming and does not transcode by default, so a TV that cannot decode a particular file may reject playback.

## Split-tunnel VPN for downloads

`hmt` can use a WireGuard tunnel without replacing the network route for the rest of the device. Only sockets opened by `hmt`'s HTTP downloader are marked for the dedicated VPN route. Other Android apps and unrelated Termux projects continue using the normal route.

This mode requires Linux/Android root networking support because per-process socket marking uses `SO_MARK` and policy routing. Without root or `CAP_NET_ADMIN`, the VPN commands refuse to start rather than silently sending downloads outside the tunnel.

### Free VPN configuration

A free WireGuard configuration can be generated from Proton VPN's account download page. Proton documents that Free-plan users can create a WireGuard `.conf`. 

```sh
mkdir -p ~/.hermit
cp ~/storage/downloads/proton-free.conf ~/.hermit/proton-free.conf
chmod 600 ~/.hermit/proton-free.conf
```

On a rooted Android/Termux environment:

```sh
su
export HERMIT_STATE_DIR="$HOME/.hermit"
hmt vpn up ~/.hermit/proton-free.conf
hmt vpn status
hmt daemon start
```

Or:

```sh
hmt vpn start ~/.hermit/proton-free.conf
```

When finished:

```sh
hmt vpn down
```

Actual VPN speed depends on the selected free server, network conditions, and download source, so Hermit does not promise a particular throughput.

## Start the daemon

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
hmt vpn up <conf>       enable split-tunnel WireGuard
hmt vpn down            disable split-tunnel WireGuard
hmt vpn status          show VPN status
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
