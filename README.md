# ComicGo

A local comic-library reader for a folder of comic archives. Each comic is one
ZIP/CBZ file; its metadata lives in the archive **comment** as plain
`key=value` lines (UTF-8).

Built with **Go + Wails v2 + Svelte 5 (Vite)** and pure-Go SQLite.

## Features

- Scrollable, **virtualized cover shelf** (only visible covers are decoded/rendered).
- **Lazy cover loading** with an on-disk JPEG thumbnail cache.
- Per-section **filter + sort by plugin rules**; rules are compile-time Go
  plugins (add one = one package + one import line).
- Config-file **layout** (`layout.json`) arranges the shelf into sections.
- Embedded **page reader** with lazy page reads straight from the zip, a
  small page-byte LRU, and persisted reading progress + favorites.
- **Pure UTF-8** handling; malformed or non-comic `.zip` files are skipped.

## Requirements

- Go ≥ 1.23
- Node ≥ 20 + npm
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation)
- Windows (WebView2 runtime — ships with Windows 11)

## Development

```bash
wails dev        # hot-reload dev run
```

## Build

```bash
wails build      # -> build/bin/comicgo.exe
```

## Where things live

Per-user app data (created on first run):

- `%APPDATA%\comicgo\comicgo.db` — SQLite index (library, metadata, favorites, progress)
- `%APPDATA%\comicgo\thumbs\` — cover thumbnail cache
- `%APPDATA%\comicgo\settings.json` — library roots, media transport
- `%APPDATA%\comicgo\layout.json` — shelf layout (user editable)

## Layout file

`layout.json` defines the shelf as an ordered list of sections:

```json
{
  "version": 1,
  "sections": [
    { "id": "all",     "title": "Library",    "kind": "grid", "sortRuleId": "bymodtime" },
    { "id": "fav",     "title": "Favorites",  "kind": "grid", "filterRuleId": "favorites", "sortRuleId": "bytitle" },
    { "id": "statsp",  "title": "Stats",      "kind": "card", "cardId": "stats" }
  ]
}
```

`kind: "grid"` sections support `filterRuleId`, `sortRuleId` and `pageSize`;
`kind: "card"` sections render a plugin card by id.

## Media transport

Covers/pages are normally served from a **loopback HTTP server**
(`127.0.0.1:<random>`); each URL carries a per-session token. Because the app
origin is `wails://`, WebView2 is launched with `--allow-insecure-localhost`
(loopback is exempted from mixed-content blocking). If images ever fail to
load, set `"mediaTransport": "blob"` in `settings.json` to fall back to inline
`data:` URLs.

## Package map

```
main.go / app.go         Wails entry + the single bound App (all bindings)
internal/config          settings.json + layout.json
internal/comic           zip reading, natural-sorted page map
internal/meta            zip-comment "key=value" parser
internal/imgutil         decode (jpeg/png/gif/webp/bmp) + thumbnail
internal/index           SQLite schema + queries (modernc.org/sqlite)
internal/scan            resumable worker-pool scanner + fingerprint skip
internal/cache           on-disk thumbs + size-bounded LRU
internal/plugins         plugin API + registry (compile-time rules/cards)
internal/serve           loopback media server (covers/pages)
frontend/                Svelte 5 UI (virtualized grid, reader, cards)
```

See [docs/plugins.md](docs/plugins.md) for how to add rules, cards, and new
sections.
