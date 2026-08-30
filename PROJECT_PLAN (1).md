# hermit — CLI media server for a Samsung Galaxy Tab A

**Working name:** `hermit` (small, self-contained, lives in a shell). Binary/subcommand: `hmt`.
Alternates if the GitHub name is taken: `slimscreen`, `tabtube`, `pocket-plex-cli`, `hmt`.
*(Name availability not verified — GitHub's search API rate-limited during research. Check at repo-creation time.)*

**One-line pitch:** a single static Go binary that searches shows, resolves season/episode
sources, downloads them to a local library, and streams them over `127.0.0.1` — all driven
from a numbered terminal menu, on a 4 GB budget tablet in your hands, no server, no cloud, no Docker.

---

## 1. Research findings (what those five sites actually are)

I inspected each origin rather than assuming. This changes the whole design.

| Site | What it is | IDs it keys on | Notes found in the wild |
|---|---|---|---|
| `1shows.org` | Next.js SPA front-end, no real backend of its own | TMDB | Domain registered 2026-04-22 (Tucows); flagged by Malwarebytes as phishing (per r/Piracy reports); review sites score it ~30/100; has already churned `.org → .ru → .org` |
| `1flex.org` | Same operator family (Next.js) | TMDB | Loads the same two third-party scripts as 1Tube |
| `1tube.org` | **Parent of the family.** JS bundle `_app-9493a04f…js` hardcodes `db.1tube.org` (accounts, watch-later, votes) and `api.viduki.net` (the media/link API), proxies art through `wsrv.nl`, and links out to `1shows.org` + `enma.lol` | TMDB | Also has a `/torrent` path and a `/api/search` route |
| `viduki.net` | The actual product: a **"streaming API"** — 4 iframe-embed flavours (Multi Server, Multi Language, Multi Embeds, Premium Embeds), "Only TMDB IDs/IMDB IDs are supported", self-described "~13+ sources", "mostly 1080p" | TMDB / IMDb | Emits `postMessage` events: `viduki:all-servers-failed` (`status:404`, `stage: initial\|manual-switch\|playback-error`) and `MEDIA_DATA` watch-progress payloads keyed `s1e1` |
| `enma.lol` | Separate **anime** site ("HiAnime/AniWatch alternative"), Next.js, AniList-driven art from `s4.anilist.co`, routes `/watch/<slug>-<anilistId>` | AniList | Different ID space, different provider graph |

### The four conclusions that matter for your architecture

1. **You do not need five scrapers.** 1Shows / 1Flex / 1Tube are skins over **one** API. One
   adapter covers three sites. Enma is a second, anime-specific adapter.
2. **Metadata is not theirs.** They use TMDB (and AniList for anime). Your tool should hit
   **TMDB/AniList directly with official keys** — free, stable, documented, no scraping — and
   keep the sketchy layer strictly for *source resolution*. This halves your breakage surface.
3. **Their "download" button is not a URL, it's an embed cascade.** Viduki hands the browser an
   iframe; the iframe resolves one of ~13 hosts; when all fail it posts a 404 message and the site
   swaps to API 2/3/4. So a "download links" feature is really: *resolve → enumerate servers →
   probe each → pick*. Your design must model **N candidate sources per episode with a health
   score**, not one link. (See §7.)
4. **Running this in a tablet browser is unsafe; running it in a hardened Go HTTP client is not.**
   Every one of these origins injects `https://layla.wtf/embed/s_<siteId>.js`, which registers a
   **service worker and subscribes the tab to Web Push with a VAPID key** (`/api/verify`,
   `PushManager.subscribe`). That is the malvertising "you have a virus" push-spam vector. They also
   load `fubuki-umami.space` analytics. **Consequence:** hermit never renders these pages, never
   executes their JS, never registers a SW, and persists no cookies — it talks raw HTTP/JSON with an
   **origin allow-list** (§8.4). This is a genuine reason your CLI approach beats using the sites,
   and it belongs in the README.

### Things to write down now, because they will bite you

- These backends have **no stability or TOS guarantee**. `api.viduki.net` is undocumented;
  `1shows.org` is 4 months old and has already moved domains. So the provider layer must be
  data-driven with hot-swappable base URLs from config — no hostnames in code (§7.3).
- **You will be rate-limited and Cloudflare-challenged.** Budget for a real challenge path:
  `pkg install cloudscraper`? No — instead: session jar, browser-like headers, jittered backoff,
  and a "needs human" state that surfaces in `hmt doctor` instead of crashing (§8.5).
- Copyright exposure is real and it's yours, not the repo's. See §13.

---

## 2. Target device: the performance budget is the spec

Assumed **Galaxy Tab A9 (SM-X110)**, which is the Tab A people buy today:

| | Spec | What it means for hermit |
|---|---|---|
| SoC | MediaTek Helio G99 (TSMC 6 nm): 2× Cortex-A76 @ 2.2 GHz + 6× A55 @ 2.0 GHz | Plenty for I/O-bound fetching. **Do not plan on transcoding.** |
| GPU | Mali-G57 MC2 | Fine for `--vo=gpu` with `--hwdec=mediacodec-copy` |
| RAM | 4 GB LPDDR4X | Android + One UI will leave you ~1.2–1.8 GB for Termux. **Hard budget: 55 MB RSS for daemon+TUI.** |
| Storage | 64 GB **UFS 2.2** + microSDXC to 1 TB | DB on UFS, media on SD (§6). UFS is fast; the SD will be your bottleneck — probe it |
| Display | 8.7" 1340×800, 60 Hz | No point fetching 4K. **Cap at 1080p by default** (native res is 800×1340) |
| Decode | H.264, HEVC (10-bit), VP9 in silicon — **no AV1 hardware decode** on G99 | **Prefer HEVC/H.264, actively demote AV1 sources.** AV1 1080p on A76/A55 = software decode, heat, and ~half the battery |
| Radio | Wi-Fi 5 (1×1) | Realistic sustained 25–45 Mbps → ~4 MB/s. Concurrency 4 is enough; 16 buys nothing and drains battery |
| Battery | 5100 mAh | Download at ≤4%/hr target; gate on charge state (§6.4) |

If the device is actually a **Tab A8 (Unisoc T3100, 3 GB RAM)**: drop quality cap to 720p, concurrency 2,
disable on-device remux entirely (stream the provider's own container), and keep it
 text-only.
`hmt doctor` should detect this and print the applied profile.

---

## 3. Scope

**In:** search → show/season/episode browse → per-episode source picker → season-or-episode
download → local library → HTTP Range streaming on-device → hand off to a real Android player →
watch-progress + next-episode autoplay → status/info/log screens.

**Out (deliberately, to stay lightweight):**
- ❌ No transcoding, no ffmpeg filter graphs, no "compatibility" re-encode. `-c copy` or nothing.
- ❌ No BitTorrent/usenet client built in. Torrents are the right answer for *libraries* and the
  wrong answer for *this* (indexer keys, DHT on battery, ratio maintenance). Keep it as a future adapter.
- ❌ No Docker/Jellyfin/Plex server, no multi-user, no auth realm, no metadata agents.
- ❌ No AI upscaling / spatial audio toys that Viduki advertises. The tablet GPU cannot do per-frame
  CNN inference; those features exist to sell desktop bandwidth. Explicitly non-goal.
- ❌ No TV-out, no Chromecast (v2 maybe — it's a cheap `dial`/`cast` receiver shim).
- ❌ **No GUI, no web frontend, no "app-like" screens.** Everything is text in a terminal (§9):
  menu, lists, progress bars (braille/block), confirmations. Nothing is rendered in a browser and
  nothing on the tablet ever executes remote JS. Side benefit you get for free: every screen has a
  `--json` twin, because there is no other UI layer to hide behind.
- ❌ No touch/mouse support in the TUI. Keyboard + Termux hardware keys only (font scale is the
  only "accessibility" knob; see §9).

---

## 4. Architecture

### 4.1 Process model — daemon + thin TUI

```
┌───────────────────────── /data/data/com.termux ─────────────────────────┐
│  hmt (TUI, BubbleTea)          hmt <verb>  (scripting, one-shot)       │
│        │ jsonrpc over unix socket ($HOME/.hermit/hmt.sock)              │
│        ▼                                                                  │
│  hmt daemon  ── single process, 5 goroutine pools, 24/7 until told off  │
│     ├── meta      TMDB/AniList + SQLite cache (TTL 24 h)                 │
│     ├── provider  registry: resolve/enum/probe/rank                      │
│     ├── queue     job state machine, priorities, retries                 │
│     ├── fetch     ranged HTTP, 4-way segment pool, resume                │
│     ├── mux       ffmpeg -c copy → faststart MP4 (deferred, on charge)   │
│     ├── lib       naming, dedupe, integrity, orphan GC                   │
│     ├── srv       127.0.0.1:8788 Range server + /api                     │
│     └── play      Android `am start` bridge, resume + next-up            │
└──────────────────────────────────────────────────────────────────────────┘
        │ files                                    │ HTTP Range 127.0.0.1
        ▼                                            ▼
/sdcard/Media/hermit/                   mpv-android / VLC (hw decode; hermit stays text)
  Severance/Severance - S01E01 - …
```

**Why a daemon:** your TUI will get backgrounded, resized, rotated and killed constantly on a
tablet. Downloads must not die with the terminal. `net/rpc/jsonrpc` (stdlib) over a unix socket
costs nothing, adds no dependency, and means `hmt search …` from a script and `hmt` the TUI are
the same code path. The TUI also has `--standalone` (embedded daemon) for the "just run one
binary" desktop/CI case.

### 4.2 Package layout

```
hermit/
├── cmd/hmt/main.go            # cobra-ish root; subcommand dispatch; no business logic
├── internal/
│   ├── cfg/                   # TOML + env + flag merge, schema-validated, zero defaults in callers
│   ├── db/                    # modernc.org/sqlite, migrations (go:embed *.sql), WAL, PRAGMA tuning
│   ├── meta/                  # tmdb.go anilist.go cache.go (search, seasons, episodes, art)
│   ├── provider/
│   │   ├── provider.go        # the interfaces in §7  (import-cycle-free core)
│   │   ├── registry.go        # config-driven instantiation, capability negotiation
│   │   ├── scoring.go         # host health EWMA, §7.4
│   │   ├── localfs/           # ✅ ships: your own files on the SD card
│   │   ├── archiveorg/        # ✅ ships: public-domain / CC media, well-documented API
│   │   ├── genericm3u8/       # ✅ ships: `hmt get <m3u8-url>` — general, source-agnostic plumbing
│   │   ├── debrid/            # ✅ ships (opt-in): resolves *your own* cloud drive
│   │   └── README.md          # how to write an adapter; the unofficial ones live here, not upstream
│   ├── queue/                 # job.go state.go scheduler.go retry.go
│   ├── fetch/                 # httpclient.go (allow-listed transport), ranges.go, parts.go, m3u8.go
│   ├── mux/                   # ffmpeg.go (remux/faststart only), probe.go
│   ├── lib/                   # library.go naming.go dedupe.go gc.go verify.go
│   ├── srv/                   # range.go play.go api.go lan.go
│   ├── play/                  # android.go (am start), progress.go (watch_later parser)
│   ├── rpc/                   # exported jsonrpc surface = the public contract for TUI *and* scripts
│   ├── doctor/                # §9.3 platform probes
│   └── tui/                   # model.go, screen_info|status|search|show|season|sources|library|…
├── pkg/hermit/                # the ONE public Go package (rpc types) so others can build UIs
│   ├── label/                 # ONE formatter for "show — episode" (§6.3): TUI, --json, logs, files
│   ├── scrub/                 # container-metadata + filename normalisation on write (§6.3)
│   └── disk/                  # free space, 3 GB reserve, projected-fit answers (§6.5)
├── platform/termux/         # install.sh, hermit-daemon (runit), Termux:Boot script, .deb via tur
├── testdata/fixtures/       # recorded JSON, m3u8 samples, tiny 2-s clip for end-to-end
└── docs/                    # PLAN/ADR files (§14)
```

`internal/` for everything, `pkg/hermit` for the RPC types only. That's how you keep a project like
this from turning into an API you have to maintain forever.

---

## 5. Stack choices (with the reasons, so you can defend them)

| Choice | Why | What I rejected |
|---|---|---|
| **Go 1.24+** | One static ~10 MB binary, `CGO_ENABLED=0`, trivially cross-compiles to `linux/arm64`, goroutines map perfectly onto "many segment downloads + one HTTP server + a TUI", tiny idle RSS | **Node** (Termux V8 + 60 MB+ before you import anything; you'd need nodejs in the deps list — heavy for this box). **Rust** — great fit, but `reqwest`+`rustls`+`ratatui` cross-compiles drag a C toolchain, and Termux CI pain is real. **Python** — `yt-dlp` alone costs 40 MB RAM and 300 ms startup; only keep it as an *optional external* downloader |
| **`modernc.org/sqlite`** | Pure-Go, transpiled SQLite → **no CGO**, so `CGO_ENABLED=0 GOOS=linux GOARCH=arm64` just works from a laptop or CI. Alternative `ncruces/go-sqlite3` (WASM) is faster on some queries; both are CGO-free, so it's a free switch later | `mattn/go-sqlite3` — needs CGO; you'd be building on the tablet or fighting the NDK |
| **Bubble Tea (+Bubbles/Lipgloss)** | The de-facto Go TUI lib: flexbox layout, focus handling, key messages, and it degrades sanely in Termux's 80×25-ish terminal. Used **keyboard-only** (§3): no mouse, no web layer | Raw ANSI (re-inventing focus/resize), `tview` (cruder for a multi-screen app), Electron/web-first (defeats "CLI, lightweight") |
| **stdlib `net/http`** server | `http.ServeContent` gives correct Range/206/If-Range for free — this is the *entire* streaming requirement | Serving HLS with ffmpeg in a loop (CPU), or bundling Jellyfin (400 MB, Java, overkill) |
| **`am start` intent handoff** | mpv-android/VLC give you hardware decode, subtitles, PiP and a touch UI for free — that UI is *the player's*, not hermit's, which is why hermit is allowed to stay text-only. You write 20 lines, not a media player | Building playback into the TUI (`--vo=gpu` inside Termux: works, but no touch controls and you fight mpv's terminal frontend on a 8.7" screen) |
| **TOML config** (`BurntSushi/toml`) | Comments in config = self-documenting for a device-specific file where every value is a trade-off | YAML (parse foot-guns), env-only (unmanageable for provider lists) |
| **`goreleaser`** | Matrix of static binaries + checksums + a `install.sh` in one `make release` | Hand-rolled build scripts |

**Pin the deps.** For a repo whose whole value prop is "runs on a potato," a transitive-dependency
surprise is the #1 support issue. `go.mod` + `dependabot` + a CI check that `go list -deps` count
stays under ~45 modules.

---

## 6. Storage design (where a tablet project lives or dies)

### 6.1 Two volumes, on purpose

```
internal UFS 2.2 (fast, always mounted, private)   shared SD (slow, huge, portable)
$HOME/.hermit/                                       /storage/emulated/0/Media/hermit/
├── hermit.db          (SQLite, WAL)                 ├── Severance/                    ← one folder per show
├── tmp/<job>/parts/*   (staging)                     │   ├── Severance - S01E01 - Good News About Tilly.mp4
├── cache/art/*.webp    (posters)                     │   ├── Severance - S01E01 - Good News About Tilly.en.srt
├── logs/                                             │   └── .hermit.json            ← sidecar, §6.3
└── reserve/.keep                                     └── The Last of Us/…
```

- **SQLite never touches the SD card.** exFAT + WAL + Android's FUSE/sdcardfs layer is a
  correctness and latency trap (fsync semantics are not what SQLite expects). DB in `$HOME/.hermit/`.
- **Media on the SD** so VLC/mpv can read it without storage shenanigans and so a 1 TB card is
  your library, not your OS partition. Needs `termux-setup-storage` (and Android 13 "All files
  access" for reliable writes).
- **Format the card portable/exFAT, not "adopted".** Adopted storage encrypts and ties the volume
  to that one device, and Termux's path disappears if you ever reformat.

### 6.2 Staging: the cross-filesystem trap

`mv` from UFS→SD is a *copy*, so staging internally doubles SD writes for a 1 GB file
(≈2.5 min extra on a cheap A1 card, plus card wear).

`hmt doctor` therefore **benchmarks the card** (`dd` 64 MB, sync, 3 passes) and picks:
- SD sequential ≥ 20 MB/s → **stage directly on SD** (`.part` next to final, atomic rename). One write.
- SD < 20 MB/s or `fsync` p95 > 250 ms → **stage on internal**, single `rename`-when-same-fs else
  `sendfile`-copy, and *tell the user in the UI* which mode is active and why (`hmt info`).

Store `storage.staging = auto|int|lib` for override. This one decision is worth more throughput
than any concurrency tuning you'll do.

### 6.3 Naming, labels, and what gets written into the file

Three separate things, and conflating them is why media tools end up with filenames like
`Severance.S01E01.1080p.WEB.H264-SOMEGROUP.mkv`. One formatter in `internal/label` feeds all three,
so a label can never drift between the TUI, `--json`, the log file, and the disk.

**(a) Folder per show — one directory, named after the show, nothing else.**

```
<Library>/<Show Title>/<Show Title> - S<season:02>E<episode:02> - <Episode Title>.<ext>
<Library>/<Show Title>/<Show Title> - S<season:02>E<episode:02> - <Episode Title>.<lang>.srt

Library/Severance/Severance - S01E01 - Good News About Tilly.mp4
Library/Severance/Severance - S01E01 - Good News About Tilly.en.srt
Library/Severance/.hermit.json
```
Movie case (no season tokens): `Movie/<Title> (2019).mp4` — the year disambiguates remakes, and it's
the one place a number other than S/E belongs in a name.

Sanitisation rules, in this order (pure function, `label.Filename`, unit-tested — 40 lines, and the
only thing standing between you and `EXDEV`/`ENOENT` weirdness on exFAT):
1. NFKC-normalise; strip emoji and variation selectors (they break Samsung's file picker).
2. `/\:*?"<>|` → `-`; collapse whitespace runs; trim trailing `. ` (Windows-compat, matters if you
   ever plug the card into a laptop).
3. Transliterate non-ASCII to ASCII if the result is non-empty, else keep original
   (`One Piece` fine, `ONE PIECE 海賊もの` → `ONE PIECE`).
4. **Episode title truncation: 64 runes**, cut at the last word boundary, no ellipsis. exFAT's limit
   is 255 *UTF-16 code units per component*, and a long anime title + long show name can reach it —
   that's a real `ENAMETOOLONG` mid-download, which is the worst possible failure (bytes on disk,
   no way to move them). Budget: `len(dir)+len(name)+ext ≤ 240`.
5. Never, ever put into a path or a name: the provider ID, the host, the URL, the quality, the
   codec, the source resolution, "WEB-DL", the group, the site, the filename the source used.
   `label.Filename` takes only `(showTitle, season, episode, episodeTitle, ext)` — the type
   signature makes the leak impossible, which is the point.

**.hermit.json** sidecar per show (not per episode) holds `tmdb_id`, `anilist_id`, and the
episode→hash table. Deliberately tiny, deliberately free of source info, and it means
`hmt lib reindex` can rebuild the DB from the SD card after you wipe the app data. No `.nfo`, no
`.jpg` next to files unless `library.art = true` — a 1 TB card full of poster JPEGs is wasted SD.

**(b) What the UI says.** Your rule: show name + episode name. That's it.

```
  2  Downloads      2 running · 1 queued · 3.1 MB/s · ETA 14m
     ▸ Severance — S02E07 · Dead and Eaten            41%  216/540 MB   3m left
     ▸ Severance — S02E08 · What's for Dinner?        ✓ remux queued
     ▸ One Piece — E1122 · ...                        ⚠ parked: retry in 4h
```
No quality, no provider, no host, no filename, no URL — those exist, they're just not on the face of
it. `Enter` on a row opens a details pane (source, score, bytes, parts, last error); the info is one
keystroke away, which is the right place for it. `label.Line(row)` = `Show — S01E01 · Title` and it
truncates to the terminal width, keeping `SxxEyy` (the only un-losable token: it's how you find the
file again on disk). Movie rows drop `SxxEyy`: `Titanic — Title` → just `Titanic (1997)`.

The redaction is applied to `--json` output **and log files** too, not just the screen, or the
"clean" property evaporates the first time someone pipes output into a bug report:
`hmt status --json` gives `provider: "p7"`, `host: "h3"`, `url: null` unless you pass `--debug`,
which prints everything and writes a `WARNING: redaction off` banner into the log. Two-tier
visibility (screen clean / `--debug` explicit) is what "hide most info" should mean in a tool people
screenshot.

**(c) What ends up inside the container.** Provider streams arrive with whatever junk the muxer left
(`title` = a 90-char file name with the group tag, `comment` = a CDN URL, `encoder` = `ffmpeg-N-12345`).
`internal/scrub` normalises on the atomic rename, as part of the same `ffmpeg -c copy` pass that
adds `+faststart` — so it costs one extra `-map_metadata` flag, not a second read:

```
MP4:  -map_metadata -1 -metadata title="<Show> - S01E01 - <Episode>"       -metadata album="<Show>" -metadata genre=<TMDB genre> -metadata date=<air year>
MKV:  -map_metadata -1 -map_chapters -1 -metadata title=<same>
```
`-map_metadata -1` drops *all* global metadata; chapters and per-stream `language`/`title` tags are
kept (`-map_metadata -1` doesn't touch stream tags) so subtitle/audio language switching still works.
Scrubbing is a *hygiene* feature — it makes the library look like a library instead of a dump, and
stops your own files and logs from being the thing that tells you where they came from. It is not
concealment of anything on the network; see §13.1 for what it does and does not do about that.

**(d) Dedupe + integrity.** Dedupe key `(media_id, season, episode)` for "already have it", plus
`sha256` for "already have this exact file under a wrong name" (`hmt lib scan` finds those). Verify
with a **streaming** hash computed while downloading — you're already reading every byte, so it's
free, and it's the only cheap proof you didn't keep a truncated fragment. Keep the sha256 in `job`
and in `.hermit.json`. Renames/moves are `label.Filename`-driven via `hmt lib rename --apply`, with
`--dry-run` printing the exact old→new table first (users' libraries exist before you wrote the
rules; a migration that can't be previewed is a library-destroying bug).

### 6.4 Battery & thermal policy (make this a first-class module, not a footnote)

```toml
[power]
wake_lock           = "auto"      # acquire termux-wake-lock only while queue non-empty
min_battery_pct       = 12        # refuse new jobs below this unless charging
defer_remux_to_charge = true      # -c copy remux is I/O+CPU; queue it instead of doing it now
concurrency_battery   = 4
concurrency_charging  = 8
max_bytes_per_sec     = "4MiB"    # ~32 Mbps: at Wi-Fi-5 saturation, more just heats the radio
nice_remux            = 10        # keep the TUI responsive
pause_when_thermal    = "moderate" # read via termux-api / thermal_zone sysfs if readable
```

`termux-battery-status` (from `termux-api`) is the cheap source of truth. Samsung's aggressive
Doze/`Adaptive battery` will still murder a background daemon — the runbook in `platform/termux/`
must include "Battery → Unrestricted", "Don't add to sleep apps", and a `Termux:Boot` + `tmux`
respawn loop, or users will file a bug against *you* (see §9.1).

### 6.5 Disk headroom: the spare-storage rule (3 GB held back)

A full SD card is how you get a half-written 1.2 GB file, a corrupt rename, and an Android that
starts stuttering system-wide. So hermit never reports "free space"; it reports **spare** space, and
every decision uses `spare`, not `free`.

```
spare      = free(library_fs) − reserve        // reserve default = 3 GB
fits(job)  = bytes_total > 0 ? bytes_total + margin ≤ spare
                                                    : unknown → use Σ(probe size) or refuse-to-guarantee
margin     = 64 MB (muxing needs a whole second copy of the file while the first still exists, §6.2)
```

Displayed in the TUI header on every screen, and in `hmt info` / `hmt df` / the queue-summary line:

```
 hermit · Tab A9 · spare 38.0 GB  (41.0 free − 3.0 held) · ⚡ 68% · ♨ ok · 2/2 sources ✓ · up 3h12m
```

Rules, all enforced in `internal/disk`, all testable without any network:
1. **Refuse before starting, not mid-write.** A job whose `bytes_total+margin > spare` is parked at
   `resolve` with `err_kind = disk_headroom` and a fix line: `"needs 1.3 GB, spare 0.8 GB — free 1 GB
   or set disk.reserve = 1GB"`. Never a partial file.
2. **Season-level check up front.** Screen 3 step 5 shows `2.4 GB → spare 41.0 → 38.6 GB after
   (3.0 held)`. If it doesn't fit, the picker tells you *there*, and offers `[T] trim to what fits`
   (newest-first) — the single highest-value UX win for a 64 GB tablet with a 22-episode season.
3. **Hard floor during transfer.** The monitor polls `statfs` every 5 s while anything is writing;
   below `reserve/2` it stops starting new *parts*, below `reserve/4` it pauses everything, flushes,
   finalises in-flight files, and parks the queue with a banner. `Android` also needs this headroom
   for itself — the SD is shared with the media gallery, WhatsApp cache, downloads, etc.
4. **`reserve` is configurable, and applies to both volumes.** `[disk] reserve = "3GB"` for the
   library, `[disk] reserve_internal = "512MB"` for the private dir (Termux dies if `$HOME` fills,
   and that's a far worse day than a parked job). Staging-on-internal (§6.2) draws on the internal
   reserve, and `doctor` says so.
5. `hmt df --json` is what the TUI, `--watch` mode and any future automation all read. One source.

Also worth 20 lines in M7: `hmt lib reclaim` — delete oldest *watched-complete* episodes with a
`--keep-fitting <GB>` flag, so the fix for "out of space, 3 more episodes to go" is one keystroke
instead of a file manager.

---

## 7. The provider layer (the part everyone gets wrong)

### 7.1 Contract

```go
// internal/provider/provider.go
type Kind string // KindMovie, KindTV, KindAnime

type Ref struct {            // every provider speaks in *external* IDs, not URLs
    Kind      Kind
    TMDBID    int    // 0 = unknown
    IMDbID    string
    AniListID int
    Tconstv   string // reserved
    Season    int    // 0 for movies
    Episode   int
    Title     string // last-resort text match, providers may ignore
    Year      int
}

type Caps struct {
    HasSearch, HasEpisodeEnum, HasSubtitles, MultiAudio bool
    Progressive, HLS, DASH                               bool
    NeedsReferer, NeedsCookie, NeedsJS                   bool // NeedsJS=true → we refuse to run it
    Qualities                                            []Quality // 720p, 1080p, …
    CodecPreference                                      []Codec   // ["hevc","avc"] — see §2 AV1 note
    BaseTTL                                              time.Duration // how long a resolved source stays trusted
}

type Source struct {
    ID, Label      string     // "viduki-api1-server3", "Filehost A"
    Kind           Kind       // direct | hls | dash | embed
    URL            string     // what fetch needs
    Referer        string
    Quality        Quality
    Codec          Codec      // hevc|avc|av1|unknown
    SizeBytes      int64      // 0 = unknown
    HasSubtitles   bool
    Languages      []string
    Score          float64    // §7.4, filled by registry, not the adapter
    ExpiresAt      time.Time
    Raw            json.RawMessage // opaque, persisted for retry
}

type Provider interface {
    ID() string
    Caps() Caps
    // optional; registry type-asserts
    Searcher interface{ Search(ctx context.Context, q string, kind Kind, page int) ([]Hit, error) }
    Resolver interface{ Resolve(ctx context.Context, r Ref) ([]Source, error) }   // "the download button"
    Subtitler interface{ Subtitles(ctx context.Context, r Ref) ([]Sub, error) }
    Probe(ctx context.Context, s Source) (ProbeResult, error) // HEAD/partial-GET; cheap
}
```

`NeedsJS=true` is the interesting field: any source that requires executing site JS to produce a
URL is **refused by default** and must be opted into explicitly, per source, with the origin
allow-list widened and no cookie jar. That one flag encodes everything in §1.4 as an invariant of
the type system instead of a README warning.

### 7.2 Ship adapters, host none of the unofficial ones

`localfs` (your SD-card library), `archiveorg` (public domain/CC; documented API), `genericm3u8`
(`hmt get <url>` — the general HLS→file plumbing that works for anything you have rights to),
`debrid` (opt-in, resolves *your own* drive contents). Each is a real, testable provider so the
abstraction stays honest rather than theoretical.

The unofficial ones (`viduki`, `1tube`, `enma`) are documented as **contract + how to write the
adapter** in `internal/provider/README.md`, and distributed as out-of-tree plugins
(`hermit-provider-<x>` in their own repo, discovered via `providers.extra_paths`). Practical
result: the fetch/queue/stream/TUI engine stays complete and upstream-safe, and the part with the
legal and domain-churn risk stays a 150-line file the user owns. (See §13.)

### 7.3 Base URLs are data

```toml
[[providers.entries]]
id = "viduki"
enabled = true
base = "https://api.viduki.net"       # churn-proof: 1shows already moved .org→.ru→.org
embed_hosts = ["https://viduki.net"]
fallback_chain = ["viduki.api1", "viduki.api2", "viduki.api3"]  # mirror their own API1→4 cascade
allow_unofficial = true
[[providers.entries]]
id = "enma"
base = "https://www.enma.lol"
id_space = "anilist"                  # different ID namespace — do not conflate with TMDB
```

Registry instantiates from config, so a domain change is a **config edit on the tablet**, not a
release. `hmt doctor` probes each `base` and prints ✓/✗ with the last error, which turns the most
common failure mode of this genre of project into a legible one-line diagnostic.

### 7.4 Source scoring (this is the feature)

Their API has no SLA; 13 hosts, some dead, some 480p, some rate-limited. So the UX answer to
"it shows you much links" is: *rank them and show why*.

```
score = 0.45·quality + 0.20·codec_pref + 0.15·host_ewma_success
      + 0.10·(1 − latency_ms/2000) + 0.05·size_known − 0.15·(codec==av1)   # no AV1 hw on G99
```
- `host_ewma_success`: per-origin EWMA (α=0.2) of `ok/fail`, persisted in `host_stats`. A host that
  404s three times in a row sinks to the bottom automatically for a week.
- `Probe` runs **before** the picker screen is shown, ≤ 8 in parallel, ≤ 3 s each, Range-`bytes=0-65535`
  (cheap, and it also proves the container is seekable). Results cached in `availability` with TTL.
- UI shows: `1080p · HEVC · ~540 MB · 1.2 s · ok 11/12   [best]` / `480p · AV1 · unknown size · slow · ⚠`.
- Auto-pick = highest score that passes `min_quality`/`prefer_codec`/`max_size`. **`auto` must be the
  default** — on a 4 GB tablet nobody wants to compare 9 links per episode for a 22-episode season.

---

## 8. Download + stream engine

### 8.1 Job state machine

```
queued → resolving → probing → downloading → verifying → [remuxing] → done
             │            │          │             │           │
             └────────────┴──────────┴─────────────┴───────────┴──→ failed(retryable?, backoff)
                                    any → paused / canceled (graceful, keep parts)
```
Persisted transitions (WAL), so a `SIGKILL` from Android mid-download resumes: `hmt resume`
re-derives parts from `job_part.done` + Range probes. Retry ladder: `1m, 5m, 25m, 2h, 8h` with
±20 % jitter, then park with a reason; a parked job must still be human-readable in screen 2.

### 8.2 Two fetch paths — this answers "download **and** local stream"

| Mode | What happens | Use |
|---|---|---|
| `hmt get` (grab) | All segments → parts → concat → verify → (remux `-c copy -movflags +faststart`) → library | Offline, airplane-mode-watching, best battery |
| `hmt play` (proxy) | No disk. srv fetches the provider's manifest/byte-range on demand, rewrites it to a local `master.m3u8`/Range-able stream, streams to mpv | "I just want to watch this episode now"; 0 bytes written |

Both share `internal/fetch` + `internal/srv`, which is the whole point of the split: one
ranged-HTTP implementation, two policies. Watching-while-downloading = `play` pointing at the
`get` job's partial output: for progressive sources serve up to `bytes_done`; for HLS, remux into
fragmented MP4 (`-frag_keyframe -empty_moov +faststart`) which plays as it grows. Mark it v1.1 —
it's the feature everyone asks for and the one most likely to have seeking edge cases.

### 8.3 Mux: never encode

Only ever `ffmpeg -i in -c copy -movflags +faststart out.mp4` (+ `-bsf:a aac_adtstoasc` when TS→MP4,
`-c:s mov_text` for embedded subs, else ship `.srt` beside the file and let mpv pick it up).
**`+faststart` is mandatory**: it moves `moov` to the head, which is what makes HTTP Range seeking
work without the whole file. If the source is already faststart MP4 → skip ffmpeg entirely
(probe with `ffprobe -v error -select_streams v -show_entries format=…,moov_atom…` or a small pure-Go
box reader; prefer the pure-Go reader to save ~120 ms of process spawn per episode on this SoC).
Do **not** plan on `ffmpeg` hardware *encoding* here: the Termux MediaCodec encoder path has known
breakage and software x264 1080p on A55 cores will cook the tablet. Not needed — we never re-encode.

### 8.4 The hardened client (from §1.4)

- `http.Transport` wrapped in an **origin allow-list** middleware built from `providers.entries`;
  30× redirect re-checked at each hop (this is how embed cascades actually work — follow, but vet).
- Referrer/UA policy **per provider** (`viduki` needs `referer = <embed_host>`; the sites themselves
  proxy art via `wsrv.nl` precisely because referrer policy matters on CDNs).
- Cookie jar: in-memory, per-provider, never on disk; `providers.persist_cookies = false` default.
- No JS. No SW. No `<webview>`. No `termux-open-url` to a provider origin (that would hand the tab to
  a real browser and re-expose the push-subscription script). Posters come from TMDB/AniList directly,
  cached as w185/w500 WebP — that also means the TUI renders instantly offline.
- Per-provider `max_bytes_per_sec`, `dialer.KeepAlive`, `IdleConnTimeout 30s`, `MaxConnsPerHost 4`.

### 8.5 Failure taxonomy → one screen

`dead_host`, `geo_or_cloudflare`, `needs_js`, `quality_below_cap`, `no_audio`, `truncated`,
`disk_full`, `sd_remounted` (Android unmounts SD on some Samsung quirks!), `battery_gate`,
`storage_perm_lost` (user revoked All-files-access). Each maps to a *fix* string printed in screen 2.
This taxonomy is what makes the repo get stars instead of "doesn't work for me" issues.

---

## 9. The UI: exactly the numbered menu you described

```
 hermit · Tab A9 · spare 38.0 GB (41.0 − 3.0 held) · ⚡ 68% · ♨ ok · 2/2 sources ✓ · up 3h12m
 ────────────────────────────────────────────────────────────────────────────────────────────
  1  Info            library 214 eps · 61.2 GB · db 3.4 MB · queue 0 active
  2  Downloads       2 running · 1 queued · 1 parked · 3.1 MB/s · spare 38.0 GB · ETA 14m
      ▸ Severance — S02E07 · Dead and Eaten        41%  216/540 MB   3m left
      ▸ Severance — S02E08 · What's for Dinner?     ✓ remux queued (on charge)
      ▸ One Piece — E1122 · The Two Shadows        ⚠ parked: retry in 4h
  3  Search          find a show, pick a season, pick episodes
  4  Library         browse what's on disk, play it
  5  Continue        s02e07 · 12 min left
  6  Sources         3 providers · probe all · health
  7  Settings        quality cap 1080p · codec pref hevc>avc · storage · power
  8  Diagnostics     hmt doctor · logs · export bundle
 ────────────────────────────────────────────────────────────────────────────────────────────
  j/k move · enter open · 1-8 jump · / search · P pause · R retry · D delete · ? help · q quit
```

**Screen 3 → 5 flow** (the core loop, so it gets its own spec):

```
search "severan"
 → 1) The Severance (2022)  tv  ·  TMDB 95396 · 2 seasons · 21 eps   [enter]
 → 2) Seasons table — with *availability per season*, from the availability cache:
      S1  9 eps   2022   ✓ 9/9 sources      [enter]
      S2 10 eps   2025   ⚠ 4/10 (partial)   [enter]
      S3  —       upcoming (0)              [disabled]
 → 3) "S2 · 10 episodes · 4 available now · est 4.1 GB @1080p HEVC"
      [S] whole season (10)     [E] pick episodes     [R] range  → 1-6,8
      [N] next unwatched        [F] fill: newest 3    [Esc] back
 → 4) episode checkboxes (space=toggle, a=all, n=none) + per-job quality/source (auto)
 → 5) summary: Severance — S02E01..E06,E08 · 6 jobs · 2.4 GB
      spare 41.0 GB → 38.6 GB after (3.0 GB held) · fits ✓ · on battery, 4-way → [y]
      (if it didn't fit: spare 1.1 GB → ✗ needs 2.4 · [T] trim to newest 2 that fit · [F] free space)
```
Season counts, "upcoming" and the ⚠ partial marker come from **TMDB**; the "✓ 9/9 sources" comes
from **probe**. Showing the two side-by-side is what makes a 5-site aggregator usable — the user
learns *before* queueing that S2 is half-missing on this backend, instead of 4 parked jobs later.

**Non-interactive twin of every screen** (this is why CLI > GUI here — it scriptifies):

```bash
hmt search severance --json | jq '.[0].tmdb_id'
hmt add tmdb:95396 --season 2 --range 1-6,8 --quality 1080p --codec hevc --when charging
hmt ls --state active,queued --watch          # live ETA lines, no TUI
hmt status --once                              # cron-able / Termux:Boot healthcheck
hmt play tmdb:95396:2:5 --resume --next         # hand to mpv at saved position, auto next ep
hmt info --pretty
```
Every TUI action is a thin client over `internal/rpc`; keep that invariant and the two never drift.
Add `hmt completion bash|zsh|fish` early — it costs nothing and it's what sells "real CLI tool".

**Keyboard-only, deliberately** (§3): no mouse handling, no web frontend, no "app" screens. The
practical consequence is that `hmt doctor`, `--json`, and `hmt status --watch` are not afterthoughts —
they *are* the escape hatch when a soft keyboard makes typing painful, so keep every screen reachable
by a one-key jump (`1`-`8`) and every list searchable with `/`. Provide a big-font profile (Termux
font size 18-22 on 8.7" is the usable floor), and never encode ⚠/✓ in colour alone (One UI + a TFT
panel at an angle washes it out).

**Redaction invariant (§6.3b):** `label` is the only formatter and `label.Redact` is applied to
`--json`, log files, and the `hmt doctor --json` bundle users paste into issues. Test for it: assert
that no `providers.entries[].base` hostname appears anywhere in default `--json` output or in
`~/.hermit/logs/*` — a string-scan test in CI, 15 lines, and the only thing keeping the promise.

---

## 10. Data model

```sql
PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA busy_timeout=5000;
PRAGMA cache_size=-8000; PRAGMA temp_store=MEMORY;   -- 8 MB page cache, sized for a 4 GB device

media(id PK, kind, tmdb_id UNIQUE, imdb_id, anilist_id, slug, title, original_title,
      poster_path, backdrop_path, overview, first_air, runtime_min, season_count,
      episode_count, raw JSON, meta_fetched_at, meta_ttl)
season(id PK, media_id FK, season, name, air_year, episode_count,
       availability_json, UNIQUE(media_id, season))
episode(id PK, media_id FK, season, episode, tmdb_ep_id, title, air_date,
        runtime_s, still_path, UNIQUE(media_id, season, episode))
job(id PK, episode_id FK NULL, media_id FK, season, episode, provider, source JSON,
    quality, codec, state, priority INT DEFAULT 100, bytes_total, bytes_done, parts_total,
    parts_done, attempts, last_error, err_kind, sha256, target_path, tmp_path,
    created_at, started_at, finished_at, next_retry_at, UNIQUE(episode_id, provider, quality))
job_part(job_id FK, idx, url, bytes_expected, bytes_done, state, PRIMARY KEY(job_id, idx))
availability(provider, episode_id, ok, quality, codec, size_bytes, score, checked_at, ttl, note,
             PRIMARY KEY(provider, episode_id))
host_stats(origin PK, ewma_success REAL DEFAULT 0.7, samples INT, median_latency_ms INT,
           last_ok_at, last_fail_at, banned_until)
playback(episode_id PK, position_s, duration_s, completed, updated_at, source)
settings(key PK, value JSON)      -- mirrors the TUI's writable settings, survives app updates
```
Indexes: `job(state, priority, next_retry_at)`, `episode(media_id, season, episode)`,
`availability(episode_id)`, `playback(completed)`. **All writes via one `*sql.DB` with
`SetMaxOpenConns(1)` for writes** (serialize writers; SQLite on UFS loves this) and a read pool of 4.
`hmt db vacuum` in M5; migrations `go:embed`'d, forward-only, tested against the last 3 versions.

Watch-progress: reuse their *shape*, `s{season}e{episode}` + `progress{watched,duration}` — that way
an import from their format is trivial and you already know how you'll model per-episode resume.

---

## 11. Termux deployment (`platform/termux/install.sh`)

```bash
# 1. Termux from F-Droid/GitHub releases — NOT the Play Store build (abandoned, wrong ABI)
pkg update -y
pkg install -y termux-api ripgrep wget tmux ffmpeg
termux-setup-storage                       # grants shared storage; re-run after revokes
curl -fsSL https://github.com/<you>/hermit/releases/latest/download/hermit-termux-arm64.deb | dpkg -i   # or install.sh
hmt doctor            # SD speed, hw codecs, ports, perms, battery gate, provider reachability
hmt daemon start      # tmux session 'hermit' + termux-wake-lock while the queue is non-empty
```
Then, in order, the three Samsung gotchas (put these at the TOP of the README, they are the #1
support drain): **Battery → Termux → Unrestricted**; **Settings → Apps → Special access →
Ignore battery optimizations**; and do **not** let Samsung's task-swipe "close" Termux (it kills the
process group — provide `hmt daemon stop` and say so). `Termux:Boot` + a 3-line respawn script gets
survival across reboots. Optional stretch that buys real stars: a **`tur` repo** so `pkg install
hermit` works, and a `proot-distro` note for people who want `apt`'s yt-dlp as an external downloader.

Serving on the same tablet = `127.0.0.1` (nothing else needs network access); `--lan <token>` for
casting to another device, with the token in the URL and no other route exposed.

---

## 12. Quality gates

**Budgets (CI must enforce, they're cheap to measure and expensive to regress):**
`go build` → ≤ 12 MB per binary · daemon idle RSS ≤ 25 MB, daemon+TUI ≤ 55 MB · cold start ≤ 400 ms
· 100 k-row library list query ≤ 20 ms · download of a 1 GB fixture at 4 MB/s adds ≤ 15 % of one
A76 core (measure with `perf stat` on CI arm64) · **no new dependency without a `docs/adr/` file**.

**Tests:** unit on the m3u8/Range/mux parsers (table-driven + `testing/quick` — truncation and
off-by-one bugs live here) · provider tests against **replayed fixtures** (record once with a tiny
`go:embed`'d JSON corpus, or `go-vcr`-style `http.RoundTripper`; CI must never hit a live
sketchy origin — it'd make the suite red for the wrong reason) · `go test -race` · end-to-end on a
committed **2-second H.264 MP4 + 6-segment m3u8** through get→verify→Range-seek→206 · fuzz the
playlist and box parsers · `golangci-lint` + `govulncheck` + `go vet`.

**CI matrix (`ubuntu-24.04-arm` for native arm64 — GitHub's arm runners are free for public repos,
so you test on the *actual architecture*, not amd64-only):** lint · test -race ·
build for `linux/{amd64,arm64,arm}` `darwin/arm64` `windows/amd64` with `CGO_ENABLED=0` ·
`goreleaser` draft on tags · optional `appuio/qemu` job for 32-bit arm (Tab A8-ish devices).

---

## 13. Rights, risk, and why the repo is shaped this way

The five sites you listed are unauthorized redistribution of licensed TV. Two separate things to
keep straight:

- **Engineering is fine.** Search → resolve → ranged download → remux → Range-serve → hand to a
  player is a normal systems project, and hermit is built to be source-agnostic: `localfs`,
  `archiveorg` (public-domain/CC), and `genericm3u8` ship in-tree and are genuinely useful.
- **I'm not going to write the code that pulls copyrighted episodes off those specific pirate
  hosts**, and shipping it in a public GitHub repo is the fast path to a DMCA takedown of *your*
  account (repo deletion is routine for scraper+downloader tools; `huggingface.co` appearing in
  1tube's own bundle is a reminder of how these projects get taken down in the wild).

So the plan deliberately puts the risky 150 lines outside the repo (§7.2): plugin in its own repo or
just your local build tag, upstream stays about the engine. That's also *better engineering* — it
insulates you from their domain churn and gives you clean separation when a source dies. Practical
hygiene if you do use such a source anyway: read-only account, no personal/second-factor reuse, a
throwaway email, and **never** browse those origins on the tablet's browser (the push-SW from §1.4
lands on the device profile, not just the tab), plus the assumption that anything you download is
logged somewhere upstream.

### 13.1 Hiding the downloads from the network, without a VPN: not possible as posed

You asked for "hide these downloads, but no VPN because the IP must not change." hermit will not grow
a feature for this, and the reason is a mechanism problem before it's a policy one:

- Without a VPN/proxy/Tor, *your* IP opens *your* TCP connections to the provider's servers, over the
  only path available: the ISP's. Anything you control ends at your NIC. You cannot make your
  destination opaque to the network between you and the destination **and** keep using your own
  address — removing the destination from view *is* what changing the path means. That's not a
  missing feature; it's the definition of the thing you ruled out.
- TLS (all five origins are `https://`) already hides what you'd want hidden and nothing more:
  the URL paths/queries, cookies, and the response bytes are not visible. Destination IP, port, SNI,
  connection count, timing, and byte volume *are* visible. And `api.viduki.net` resolves to a CDN IP
  anyone can look up the owner of, so "they see a big HTTPS transfer to a hosting ASN" is the honest
  worst case.
- The alternatives that show up in this genre of discussion (SSH tunnel to a box you rent, a
  Cloudflare Worker that re-fetches, Tor) are all "a relay changes the path" = a VPN with extra steps
  and a worse trust profile: you've swapped a company you pay for a machine you own or a stranger.
  If you'd accept any of those, you would accept a VPN, which you've excluded. So the only architectures
  that genuinely change *what your device talks to* are: a **debrid-style service** (their servers
  fetch; you pull from *their* CDN, so you never connect to the source host — still not concealment,
  your volume to the debrid host is plainly visible), or **downloading somewhere else** and transferring
  the finished files to the tablet over local Wi-Fi.

Worth knowing, because the threat is usually mis-modelled: DMCA abuse notices are generated from
**observed peers** — BitTorrent swarms, and servers distributing files. hermit is an HTTPS *client*
to a CDN: not announcing, not seeding, not listening on a port (§3 refuses the torrent client, §11
binds `127.0.0.1`). That distinction is what keeps a "client" out of the notice pipeline entirely, and
it's already designed in. Volume and duration still show up in your ISP's metering (that's billing, not
policing), so if the concern is the *meter*, `max_bytes_per_sec` and the 1080p cap are the levers — that
part is real and configurable. The part you asked for is not, and I'd rather say so in the plan than let
you build a fake version of it and trust it.

### 13.2 Hiding at rest: solvable, so let's actually do it

That's the version of "hide the downloads" that has real answers, and it costs little. Threat: someone
glances at the tablet, the tablet gets lost, or you hand it to someone.

| Measure | What it does | Cost |
|---|---|---|
| `library.nomedia = true` → write a `.nomedia` in every show dir | Android's media scanner skips the whole subtree: no gallery thumbnails, no "Recent" leakage, no MTP listing. `hmt lib secure --nomedia` (re)applies after imports | ~0 |
| Termux (and mpv) inside **Samsung Secure Folder** | Separate Android profile, separate FBE key, locked behind its own biometric/PIN; library invisible to every other app and to the gallery. Works unrooted, which matters: Termux **cannot** FUSE-mount on Android 12+, so `gocryptfs`/`rclone mount` are off the table, and there's no `losetup` without root — Secure Folder is the only real at-rest encryption available here | You lose the 1 TB microSD (must live on the 64 GB volume); mpv must be installed inside Secure Folder too, or it can't read the files |
| `label` redaction (§6.3b) | Nothing on screen, in `--json`, in the logs, or in a pasted `doctor` bundle names a source | 0 |
| `scrub` metadata stripping (§6.3c) | Container has `title/album/genre/date`, no encoder junk, no URLs, no group tags | 1 extra ffmpeg flag |
| `disk.reserve` (§6.5) + no `.part` litter | A crashed download doesn't leave `Severance.S02E07.1080p.WEB…viduki…mp4.part` sitting in the card root | 0 |
| **Explicit trade-off:** §6.1 says portable exFAT, not adopted | Portable = unencrypted. Pull the card, read it on any laptop. Chosen because adopted storage breaks Termux paths and dies with the device. If at-rest secrecy outranks that, the library goes in Secure Folder (row 2) and you lose the SD | accept or flip |

`hmt doctor` prints a `privacy` block: `.nomedia` present on all show dirs, `bind` is loopback,
`lan=false`, DB not on SD, `logs` free of provider hostnames, `scrub` on. Anything that fails is a red
line with a one-command fix. Two-line non-goal for the README, because it kills a whole class of
feature requests: *hermit does not encrypt, disguise, proxy, or hide traffic. If you need that, use a
VPN or don't use this.*

---

## 14. Milestones (each one leaves a working binary on the tablet)

| | Deliverable | Exit criteria |
|---|---|---|
| **M0** · skeleton (1 day) | repo, `hmt` with `info`/`version`, CI matrix, goreleaser, `docs/adr/0001-stack.md`, license, issue templates, this plan as `docs/PLAN.md` | install.sh runs on the Tab A9 and `hmt info` prints the detected device profile |
| **M1** · platform spike (2-3 d) | `hmt doctor` + the player handoff spike | SD speed number, `h264/hevc_mediacodec` availability, **one working `am start` → mpv URL handoff**, and a *decision recorded in an ADR* for resume handling (candidates: mpv-android `watch_later` file parsing, or TUI-side position with `--start` when using mpv-in-Termux). This is the riskiest unknown; do it before writing a line of the queue |
| **M2** · metadata + DB (3 d) | migrations, TMDB+AniList with cache/TTL, `hmt search/show/seasons/episodes --json` | offline second run of the same search is <30 ms and does zero network I/O |
| **M3** · fetch engine (4-5 d) | ranged client, parts, resume, verify, remux-to-faststart, `genericm3u8` adapter | kill -9 mid-download → `hmt resume` completes byte-identical; a 2 s fixture plays from disk in both players |
| **M4** · queue + provider registry (4 d) | state machine, priorities, scoring/EWMA, availability cache, `localfs`+`archiveorg` | 40 queued jobs survive a daemon restart; probe results cached with TTL; a flaky fake host demonstrably sinks in ranking |
| **M5** · srv + play (3 d) | Range server, `hmt play` proxy mode, intents, `playback` table, next-up | mpt seek across a 1 GB file starts <150 ms; `--lan` token flow works from a laptop on the same Wi-Fi |
| **M6** · TUI (5 d) | the 8 screens in §9, keyboard-only, font/theme profiles, `hmt completion` | the *entire* season-vs-episode flow from §9, keys only, on the tablet, in <60 s for a known show |
| **M6.5** · naming, labels, reserve (2-3 d, **can run parallel to M5**) | `internal/label`, `internal/scrub`, `internal/disk` (§6.3/§6.5), `.nomedia`, `hmt df`, `lib rename --dry-run/--apply`, `lib secure` | screen 2 rows read `Show — S01E01 · Title` and nothing else; a full-season queue is refused with `disk_headroom` + a fix line when `spare < need`; CI string-scan finds no provider hostname in default `--json` or in `~/.hermit/logs/*`; `ffprobe` on an output file shows no `comment`/URL/encoder/artist tags |
| **M7** · power/robustness (3 d) | power module, failure taxonomy, parked-job UX, backup/restore of `hermit.db`, `--lan` hardening, `doctor` privacy block (§13.2) | 1 h download at ≤5 % battery drain and no `SIGKILL` from Doze; every `err_kind` has a printed fix |
| **M8** · polish/ship (2-3 d) | README (with the ASCII menu transcript + the Samsung gotchas), CHANGELOG, v0.1.0 release, discussion enabled, `good-first-issue` labels | fresh Termux install → first episode playing, following the README *verbatim*, by someone who didn't write it |

Then v1: watching-while-downloading (§8.2), subtitle auto-fetch, `hmt import <their s1e1 progress JSON>`,
720p-on-battery auto-degrade, `next unwatched` auto-queue, and `hmt lib reclaim --keep-fitting`.
**Not** on this list, ever: a web UI (§3), a torrent client (§3), and anything that disguises traffic
(§13.1).

**Realistic total: ~4 focused weeks part-time.** M1 and M3 are where projects like this die;
do them first, in that order, and don't beautify the TUI before M3 is green.

---

## 15. Top risks

| Risk | Mitigation |
|---|---|
| Backend/domain disappears (already happened once) | `providers` from config; `doctor` prints reachability; unofficial adapters out-of-tree so upstream doesn't rot |
| Android kills the daemon | `termux-wake-lock` while active + Samsung Unrestricted + `Termux:Boot` respawn; document as top-of-README, not a footnote |
| microSD too slow / card wear | `doctor` benchmarks and auto-selects staging mode (§6.2); hash while streaming (no second read) |
| AV1/4K sources that won't play | hard 1080p cap; codec preference in scoring with an AV1 penalty (§7.4); `probe` rejects un-decodable containers before queueing |
| Seeking bugs on partial files | v1.1 scope, fragmented-MP4 approach isolated to `srv/partial.go`, behind a flag |
| 4 GB RAM OOM | one DB write conn, bounded part buffers (256 KB chunks, 4 in flight), TUI renders 50-row windows, RSS asserted in CI |
| Dependency churn breaks a potato | dep ceiling check in CI, `go.mod` pinning, ADR per dependency |
| Source names leaking into filenames, logs, or pasted bug reports | `label` is the only formatter, `scrub` on write, `label.Redact` on `--json`/logs, and a CI test that greps for provider hostnames (§6.3b, M6.5) |
| Library visible in the gallery / readable off a pulled card | `.nomedia` on every show dir, optional Secure Folder placement, `doctor` privacy block; SD is unencrypted by design (§13.2) |
| "Add a VPN mode" feature requests | refuse in `docs/adr/000x-no-traffic-cloaking.md` with §13.1's explanation, so it's answered once, in public, for everyone |
| Cloudflare/JS challenge | `needs_js` failure kind + explicit "run `hmt sources --probe`, this source needs a manual cookie" path; no headless browser (100 MB+ — dead on arrival here) |

---

## 16. GitHub project + repo setup (day 0 checklist)

- Repo `hermit`, **MIT**, description: *"CLI media server for potato hardware: search, download, stream — one static Go binary, runs in Termux."* Topics: `golang`, `termux`, `android`, `tui`, `bubbletea`, `media-server`, `cli`, `hls`, `self-hosted`, `arm64`.
- Branch protection on `main`: build + test + lint + `govulncheck` required. Conventional commits + `semantic-release`/`goreleaser` changelog.
- Issue templates: *bug* (must paste `hmt doctor --json` — that single field kills 80 % of back-and-forth), *provider request*, *idea*.
- `SECURITY.md` stating the origin-allow-list/no-JS policy from §8.4 and the "no traffic cloaking, at-rest only" boundary from §13 (it's an unusual, credible property for this genre of tool).
- Projects board (columns): **Backlog · M0-M1 (risk-out the platform) · M2-M5 (engine) · M6 (UI) · v1 · Needs-repro**. 14 ready-to-import titles in `github/issues.jsonl`, import with `github/import-issues.sh`.
- README order that converts: the numbered-menu transcript (ASCII, since it's text-only) → 8-line quickstart → *Samsung battery gotchas* → "why a CLI, and what it refuses to do" → perf budget table → FAQ.
