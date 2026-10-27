#!/bin/sh
set -eu

cd "$(dirname "$0")"
APP="WordSolver.app"

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp ui/Info.plist "$APP/Contents/Info.plist"
iconset="$APP/Contents/Resources/AppIcon.iconset"
mkdir "$iconset"
for size in 16 32 128 256 512; do
	sips -z "$size" "$size" ui/icon.png --out "$iconset/icon_${size}x${size}.png" >/dev/null
done
for size in 16 32 128 256 512; do
	double=$((size * 2))
	sips -z "$double" "$double" ui/icon.png --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$APP/Contents/Resources/AppIcon.icns"
rm -r "$iconset"
go build -trimpath -o "$APP/Contents/Resources/wordsolver" .
xcrun swiftc -parse-as-library -O -framework SwiftUI -framework AppKit \
	-o "$APP/Contents/MacOS/WordSolver" ui/main.swift
codesign --force --deep --sign - "$APP"
