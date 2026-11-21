#!/bin/sh
# Baut Bondr.app (Apple Silicon und Intel) und ein .dmg. Läuft auf einem Mac mit Xcode.
# Erwartet bondrd-arm64 und bondrd-amd64 in mac/kern/ (aus dem Go-Kern gebaut).
set -eu
cd "$(dirname "$0")"
FASSUNG="${1:-0.0.0}"
rm -rf bau && mkdir -p bau
for a in arm64 x86_64; do
  swiftc -O -parse-as-library -swift-version 5 -target "$a-apple-macos14" \
    -o "bau/Bondr-$a" Quellen/*.swift
done
APP=bau/Bondr.app
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
lipo -create -output "$APP/Contents/MacOS/Bondr" bau/Bondr-arm64 bau/Bondr-x86_64
lipo -create -output "$APP/Contents/MacOS/bondrd" kern/bondrd-arm64 kern/bondrd-amd64
sed "s/FASSUNG/$FASSUNG/g" Info.plist > "$APP/Contents/Info.plist"
# Symbol aus den PNGs in symbol/
mkdir -p bau/Bondr.iconset
for n in 16 32 128 256 512; do
  cp "symbol/$n.png" "bau/Bondr.iconset/icon_${n}x${n}.png"
  d=$((n * 2)); [ -f "symbol/$d.png" ] && cp "symbol/$d.png" "bau/Bondr.iconset/icon_${n}x${n}@2x.png"
done
iconutil -c icns -o "$APP/Contents/Resources/Bondr.icns" bau/Bondr.iconset
# Ad-hoc-Signatur: kein bezahltes Zertifikat, aber macOS merkt Veränderungen
codesign --force --sign - "$APP/Contents/MacOS/bondrd"
codesign --force --sign - "$APP"
codesign --verify --strict "$APP"
mkdir -p bau/dmg && cp -R "$APP" bau/dmg/ && ln -s /Applications bau/dmg/Programme
hdiutil create -volname Bondr -srcfolder bau/dmg -ov -format UDZO "bau/Bondr-$FASSUNG-mac.dmg" >/dev/null
echo "fertig: mac/bau/Bondr-$FASSUNG-mac.dmg"
