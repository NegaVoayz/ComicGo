# ComicGo

A native cross-platform comic-library reader built entirely in Go with
[Gio](https://gioui.org). It runs on Windows, Linux, and macOS. Each comic is
one ZIP/CBZ file; its metadata lives in the archive comment as plain
`key=value` UTF-8 lines.

## Features

- Native Gio desktop UI with no WebView, JavaScript, Node, or Wails runtime.
- Virtualized cover shelf with lazy thumbnail decoding and a size-bounded image
  cache.
- Sections, filters, sorting, tag chips, and search driven by `layout.json`.
- Zip/CBZ reader with neighbor-page prefetch and persisted reading progress.
- Integrated dark window chrome with Windows 11 Snap behavior, Linux
  window-manager integration, and macOS traffic-light controls.
- Native file/folder pickers on every supported platform. Native drag-and-drop
  import is available on Windows.
- Compile-time Go filter, sort, and Gio card plugins.
- Local SQLite index using `modernc.org/sqlite`.

## Requirements

- Windows 10/11 x64
- Linux x64 with X11 or Wayland
- macOS 13 or newer on Intel or Apple Silicon
- Go 1.25 or newer

No external runtime or JavaScript toolchain is required. Gio uses cgo on Linux
and macOS, so those platforms require a C compiler and desktop development
libraries.

## Build

Windows:

```powershell
.\scripts\build-windows.ps1
```

Linux dependencies on Ubuntu/Debian:

```bash
sudo apt-get install pkg-config libwayland-dev libx11-dev libx11-xcb-dev \
  libxkbcommon-dev libxkbcommon-x11-dev libgles2-mesa-dev \
  libegl1-mesa-dev libffi-dev libvulkan-dev libxcursor-dev \
  libxrandr-dev libxinerama-dev libxi-dev libxxf86vm-dev \
  libegl-mesa0 libglx-mesa0 libgl1-mesa-dri libgbm1 libegl1 \
  mesa-vulkan-drivers xdg-desktop-portal xdg-desktop-portal-gtk
```

Build and create the `.deb` package:

```bash
VERSION=0.1.0 ARCH=amd64 ./scripts/package-linux.sh
```

macOS:

```bash
VERSION=0.1.0 ./scripts/package-macos.sh
```

Artifacts are written to `build/bin/`:

- `comicgo.exe` on Windows
- `comicgo_<version>_amd64.deb` on Linux
- `ComicGo.app` on macOS

On Linux, drag-and-drop import is not available; use **Import → From archive
file** or **From image folder**. File pickers require a running
freedesktop desktop portal; minimal or headless WSL sessions must start D-Bus
and `xdg-desktop-portal` first. Windows retains drag-and-drop support.

Run the full checks directly with:

```bash
go test ./...
go vet ./...
```

## Data locations

Per-user application data follows each platform's native configuration
directory:

- Windows: `%APPDATA%\comicgo\`
- Linux: `${XDG_CONFIG_HOME:-~/.config}/comicgo/`
- macOS: `~/Library/Application Support/comicgo/`

Each directory contains:

- `comicgo.db` - SQLite index
- `thumbs/` - cached cover thumbnails
- `settings.json` - library roots, reader preferences, and legacy settings
- `layout.json` - user-editable shelf layout

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
