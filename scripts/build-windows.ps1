$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    New-Item -ItemType Directory -Force build\bin | Out-Null
    go test ./...
    go vet ./...
    go build -trimpath -ldflags="-s -w -H=windowsgui -X 'gioui.org/app.ID=io.comicgo.ComicGo'" -o build\bin\comicgo.exe .
} finally {
    Pop-Location
}
