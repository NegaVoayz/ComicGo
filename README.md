# ComicGo

A native Windows comic-library reader built entirely in Go with
[Gio](https://gioui.org). Each comic is one ZIP/CBZ file; its metadata lives in
the archive comment as plain `key=value` UTF-8 lines.

## Features

- Native Gio desktop UI with no WebView, JavaScript, Node, or Wails runtime.
- Virtualized cover shelf with lazy thumbnail decoding and a size-bounded image
  cache.
- Sections, filters, sorting, tag chips, and search driven by `layout.json`.
- Zip/CBZ reader with neighbor-page prefetch and persisted reading progress.
- Native Windows file/folder pickers and drag-and-drop import.
- Compile-time Go filter, sort, and Gio card plugins.
- Local SQLite index using `modernc.org/sqlite`.

## Requirements

- Windows 10/11
- Go 1.25 or newer

No external runtime or C compiler is required.

## Build

```powershell
go build -trimpath -ldflags="-s -w" -o build\bin\comicgo.exe .
```

Run the application:

```powershell
.\build\bin\comicgo.exe
```

Run the tests:

```powershell
go test ./...
go vet ./...
```

## Data locations

Per-user application data:

- `%APPDATA%\comicgo\comicgo.db` - SQLite index
- `%APPDATA%\comicgo\thumbs\` - cached cover thumbnails
- `%APPDATA%\comicgo\settings.json` - library roots and legacy settings
- `%APPDATA%\comicgo\layout.json` - user-editable shelf layout

The obsolete `mediaTransport` settings key is accepted for compatibility but
has no effect: covers and pages are decoded directly by Gio.

## Layout file

`layout.json` defines the shelf as an ordered list of sections:

```json
{
  "version": 1,
  "sections": [
    { "id": "all", "title": "Library", "kind": "grid", "sortRuleId": "bymodtime" },
    { "id": "fav", "title": "Favorites", "kind": "grid", "filterRuleId": "favorites", "sortRuleId": "bytitle" },
    { "id": "stats", "title": "Stats", "kind": "card", "cardId": "stats" }
  ]
}
```

Grid sections support `filterRuleId`, `sortRuleId`, and `pageSize`. Card
sections render a registered Gio card by `cardId`.

## Package map

```
main.go, app.go          entry point, library service, scanning, and imports
ui*.go                   Gio event loop, shelf, detail, reader, dialogs, drop
internal/config          settings.json and layout.json
internal/comic           ZIP/CBZ reading and page ordering
internal/meta            ZIP-comment metadata parser
internal/index           SQLite schema and queries
internal/scan            resumable library scanning
internal/cache           thumbnail disk cache and page-byte LRU
internal/plugins         compile-time rules and Gio card specs
```

See [docs/plugins.md](docs/plugins.md) for plugin guidance.
