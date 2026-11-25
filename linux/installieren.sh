#!/bin/sh
# Bondr für Linux ohne Paketverwaltung installieren (für den angemeldeten Benutzer).
# Braucht Python 3 mit PyGObject, GTK 4 und libadwaita.
set -e
cd "$(dirname "$0")"
ziel="$HOME/.local/lib/bondr"
mkdir -p "$ziel" "$HOME/.local/bin" "$HOME/.local/share/applications" "$HOME/.local/share/icons/hicolor/scalable/apps"
install -m 755 bondrd bondr.py "$ziel/"
ln -sf "$ziel/bondr.py" "$HOME/.local/bin/bondr"
sed "s|^Exec=bondr|Exec=$HOME/.local/bin/bondr|" io.github.dschonas04.bondr.desktop > "$HOME/.local/share/applications/io.github.dschonas04.bondr.desktop"
install -m 644 io.github.dschonas04.bondr.svg "$HOME/.local/share/icons/hicolor/scalable/apps/"
echo "Bondr ist installiert. Start über das Anwendungsmenü oder: $HOME/.local/bin/bondr"
