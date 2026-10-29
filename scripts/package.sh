#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
version=$(tr -d '\r\n' < VERSION)
output="build/releases"
name="LiveMyGo-v$version-macos-arm64"
mkdir -p "$output"
codesign --verify --deep --strict build/LiveMyGo.app
go run ./cmd/installer
tiffutil -cathidpicheck build/installer/background.png build/installer/background@2x.png -out build/installer/background.tiff
"${DMGBUILD:-dmgbuild}" -s scripts/dmg-settings.py "Live MyGo $version" "$output/$name.dmg"
mountdir=$(mktemp -d "${TMPDIR:-/tmp}/live-mygo-package.XXXXXX")
trap 'hdiutil detach -quiet "$mountdir" 2>/dev/null || true; rmdir "$mountdir" 2>/dev/null || true' EXIT
hdiutil attach -quiet -readonly -nobrowse -mountpoint "$mountdir" "$output/$name.dmg"
codesign --verify --deep --strict "$mountdir/LiveMyGo.app"
hdiutil detach -quiet "$mountdir"
rmdir "$mountdir"
trap - EXIT
ditto -c -k --sequesterRsrc --keepParent build/LiveMyGo.app "$output/$name.zip"
(cd "$output" && shasum -a 256 "$name.dmg" "$name.zip" > SHA256SUMS)
printf '%s\n' "$PWD/$output/$name.dmg" "$PWD/$output/$name.zip"
