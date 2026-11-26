#!/usr/bin/env bash
# Baut die Android-App: erst den Go-Kern für arm64 (als libbondrd.so), dann das APK in
# einem Android-Container auf einem Docker-Host (BAUHOST oder Datei bauhost.lokal) oder,
# mit ANDROID_HOME gesetzt, direkt hier.
#   ./bauen.sh [debug|release]
# Signiert wird ein Release mit BONDR_KEYSTORE (Pfad) und BONDR_KEYSTORE_PASSWORT.
set -euo pipefail
VARIANTE="${1:-debug}"
HIER="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FASSUNG="${BONDR_FASSUNG:-0.0.0}"
mkdir -p "$HIER/app/src/main/jniLibs/arm64-v8a"
(cd "$HIER/.." && CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -trimpath \
  -ldflags "-s -w -X main.Version=$FASSUNG" -o "$HIER/app/src/main/jniLibs/arm64-v8a/libbondrd.so" ./cmd/bondrd)
if [ -n "${ANDROID_HOME:-}" ]; then
  (cd "$HIER" && BONDR_FASSUNG="$FASSUNG" ./gradlew --no-daemon -q testDebugUnitTest "assemble${VARIANTE^}")
  mkdir -p "$HIER/ausgabe"
  cp "$HIER"/app/build/outputs/apk/"$VARIANTE"/app-"$VARIANTE"*.apk "$HIER/ausgabe/Bondr-$FASSUNG-android.apk"
  exit 0
fi
if [ -z "${BAUHOST:-}" ] && [ -f "$HIER/bauhost.lokal" ]; then BAUHOST="$(cat "$HIER/bauhost.lokal")"; fi
: "${BAUHOST:?BAUHOST setzen, bauhost.lokal anlegen oder ANDROID_HOME setzen}"
FERN="/tmp/bondr-android"
ssh "$BAUHOST" "rm -rf $FERN/app/build/outputs"
rsync -az --delete --exclude '.git' --exclude 'build' --exclude '.gradle' --exclude '.kotlin' --exclude 'ausgabe' --exclude 'bauhost.lokal' "$HIER/" "$BAUHOST:$FERN/"
SIGNIEREN=""
if [ "$VARIANTE" = release ] && [ -n "${BONDR_KEYSTORE:-}" ]; then
  # Schlüssel und Passwort nur für diesen Bau auf den Host, nie in der Befehlszeile
  ssh "$BAUHOST" "mkdir -p -m 700 $FERN-schluessel"
  scp -q "$BONDR_KEYSTORE" "$BAUHOST:$FERN-schluessel/bondr.jks"
  printf 'BONDR_KEYSTORE=/schluessel/bondr.jks\nBONDR_KEYSTORE_PASSWORT=%s\n' "$BONDR_KEYSTORE_PASSWORT" | ssh "$BAUHOST" "umask 077; cat > $FERN-schluessel/env"
  SIGNIEREN="-v $FERN-schluessel/bondr.jks:/schluessel/bondr.jks:ro --env-file $FERN-schluessel/env"
  trap 'ssh "$BAUHOST" "rm -rf $FERN-schluessel"' EXIT
fi
ssh "$BAUHOST" "cd $FERN && docker run --rm $SIGNIEREN \
    -v $FERN:/build -v hecht-gradle-cache:/build/.gradle -w /build -e BONDR_FASSUNG=$FASSUNG \
    nexora-android:1 bash -c 'export ANDROID_USER_HOME=/build/.gradle/android-bondr; ./gradlew --no-daemon -q testDebugUnitTest assemble${VARIANTE^}; r=\$?; chown -R \$(stat -c %u:%g /build/bauen.sh) /build/app /build/.kotlin 2>/dev/null; exit \$r'"
mkdir -p "$HIER/ausgabe"; rm -f "$HIER/ausgabe/Bondr-$FASSUNG-android.apk"
scp -q "$BAUHOST:$FERN/app/build/outputs/apk/$VARIANTE/app-$VARIANTE*.apk" "$HIER/ausgabe/Bondr-$FASSUNG-android.apk"
ls -lh "$HIER/ausgabe/Bondr-$FASSUNG-android.apk"
