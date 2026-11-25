# Änderungen

## 3.2.0

- Bondr für Linux: GTK-4-App mit Kern, Pakete .deb und .rpm (x86_64, ARM64) und Archiv mit Installationsskript. Bindung per SO_BINDTODEVICE, Systemproxy in GNOME und KDE, Namen der Verbindungen aus NetworkManager.
- Scheitert das Speichern einer Einstellung, sagt Bondr das und behält den alten Wert.

## 3.1.0

- Programme ohne Proxy (Steam) unter Windows per WinDivert über alle Wege.
- Systemproxy nur beim eigenen Proxy-Port, Explorer nur mit geprüftem Pfad.

## 3.0.0

- Lastverteiler mit Gewicht, Reserve, Strategien und Gesundheitsprüfung.
- Kern in Go, Windows-App mit Installer, Mac-App.
