#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
app="$root/build/bin/ComicGo.app"
contents="$app/Contents"
version="${VERSION:-0.1.0}"

cd "$root"
go test ./...
go vet ./...

rm -rf "$app"
mkdir -p "$contents/MacOS" "$contents/Resources"
go build -trimpath -ldflags="-s -w -X gioui.org/app.ID=io.comicgo.ComicGo" \
  -o "$contents/MacOS/comicgo" .

iconset="$root/build/package/ComicGo.iconset"
rm -rf "$iconset"
mkdir -p "$iconset"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" build/appicon.png --out "$iconset/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z "$double" "$double" build/appicon.png --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$contents/Resources/ComicGo.icns"

cat > "$contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDisplayName</key>
  <string>ComicGo</string>
  <key>CFBundleExecutable</key>
  <string>comicgo</string>
  <key>CFBundleIconFile</key>
  <string>ComicGo</string>
  <key>CFBundleIdentifier</key>
  <string>io.comicgo.ComicGo</string>
  <key>CFBundleName</key>
  <string>ComicGo</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>$version</string>
  <key>CFBundleVersion</key>
  <string>$version</string>
  <key>LSMinimumSystemVersion</key>
  <string>13.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF

printf 'Created %s\n' "$app"
