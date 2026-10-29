#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
version=$(tr -d '\r\n' < VERSION)
short_version=${version%%-*}
bundle="build/LiveMyGo.app"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
go run ./cmd/icon
iconutil -c icns build/AppIcon.iconset -o "$bundle/Contents/Resources/AppIcon.icns"
go build -o "$bundle/Contents/MacOS/live-mygo" .
cat > "$bundle/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>live-mygo</string>
<key>CFBundleIdentifier</key><string>dev.livemygo.verify</string>
<key>CFBundleName</key><string>Live MyGo</string>
<key>CFBundleDisplayName</key><string>Live MyGo</string>
<key>CFBundleIconFile</key><string>AppIcon</string>
<key>CFBundleVersion</key><string>1</string>
<key>CFBundleShortVersionString</key><string>0.4.0</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSMinimumSystemVersion</key><string>12.0</string>
<key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST
plutil -replace CFBundleShortVersionString -string "$short_version" "$bundle/Contents/Info.plist"
plutil -insert LiveMyGoReleaseVersion -string "$version" "$bundle/Contents/Info.plist"
plutil -insert CFBundleGetInfoString -string "Live MyGo $version 测试版" "$bundle/Contents/Info.plist"
if [ "${LIVE_MYGO_BUNDLE_PLAYER:-1}" != "0" ]; then
  go run ./cmd/bundle-player "$bundle"
fi
mkdir -p "$bundle/Contents/Resources/licenses/LiveMyGo"
cp LICENSE "$bundle/Contents/Resources/licenses/LiveMyGo/LICENSE"
cp assets/icons/LICENSE "$bundle/Contents/Resources/licenses/Lucide-LICENSE"
go list -m -f '{{if not .Main}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}' all > build/go-modules.txt
while IFS='|' read -r module version module_dir; do
  [ -n "$module" ] || continue
  target="$bundle/Contents/Resources/licenses/go/$module"
  mkdir -p "$target"
  printf '%s %s\n' "$module" "$version" > "$target/module.txt"
  for notice in "$module_dir"/LICENSE* "$module_dir"/COPYING* "$module_dir"/NOTICE*; do
    if [ -f "$notice" ]; then install -m 644 "$notice" "$target/$(basename "$notice")"; fi
  done
done < build/go-modules.txt
cp THIRD_PARTY.md "$bundle/Contents/Resources/licenses/THIRD_PARTY.md"
codesign --force --sign - "$bundle"
printf '%s\n' "$PWD/$bundle"
