#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="${VERSION:-0.1.0}"
arch="${ARCH:-amd64}"
package="comicgo_${version}_${arch}"
output="$root/build/bin/comicgo_${version}_${arch}.deb"
staging="$(mktemp -d "${TMPDIR:-/tmp}/comicgo-deb.XXXXXX")"
trap 'rm -rf "$staging"' EXIT
chmod 0755 "$staging"

cd "$root"
go test ./...
go vet ./...

mkdir -p \
  "$staging/DEBIAN" \
  "$staging/usr/bin" \
  "$staging/usr/share/applications" \
  "$staging/usr/share/icons/hicolor/1024x1024/apps"

go build -trimpath -ldflags="-s -w -X gioui.org/app.ID=io.comicgo.ComicGo" \
  -o "$staging/usr/bin/comicgo" .
install -m 0644 build/appicon.png \
  "$staging/usr/share/icons/hicolor/1024x1024/apps/comicgo.png"

cat > "$staging/DEBIAN/control" <<EOF
Package: comicgo
Version: $version
Section: graphics
Priority: optional
Architecture: $arch
Maintainer: ComicGo Contributors
Depends: libc6, libx11-6, libxkbcommon0, libwayland-client0, libgl1, libegl1, libvulkan1
Recommends: xdg-desktop-portal, xdg-desktop-portal-gtk
Description: Native comic-library reader built with Gio
 ComicGo indexes ZIP/CBZ and image-folder comics, provides a virtualized
 cover shelf, metadata editing, and a directional one/two-page reader.
EOF

cat > "$staging/usr/share/applications/comicgo.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=ComicGo
Comment=Read and organize your comic library
Exec=comicgo
Icon=comicgo
Terminal=false
Categories=Graphics;Viewer;
StartupWMClass=io.comicgo.ComicGo
EOF

mkdir -p "$root/build/bin"
dpkg-deb --root-owner-group --build "$staging" "$output"
printf 'Created %s\n' "$output"
