# Bondr

Bondr nutzt alle Internetzugänge eines Windows-Rechners gleichzeitig: Kabel, WLAN und das Handy per USB-Tethering.

## Was es macht

**Datei laden.** Eine Datei wird in Stücke von 4 MiB geteilt, die gleichzeitig über alle Wege geholt werden. Hier addiert sich die Bandbreite auch für einen einzelnen Download. Bremst der Server (429/503), nimmt Bondr Anfragen zurück und versucht es später wieder; hängt ein Weg, übernimmt ein anderer das Stück.

**Programme verteilen.** Bondr ist ein lokaler Proxy (SOCKS5 und HTTP auf `127.0.0.1:1080`). Mit dem Schalter „Windows-Proxy“ benutzen ihn Browser und alle Programme, die den Windows-Proxy beachten. Jede neue Verbindung geht über den Weg, der gerade am wenigsten zu tun hat, gemessen an dem, was er zuletzt geschafft hat. Eine einzelne Verbindung wird dabei nicht schneller, viele gleichzeitige schon (Webseiten, Steam, Updates). Adressen im lokalen Netz (10.x, 192.168.x, .local, .fritz.box) gehen am Proxy vorbei.

Echtes Bündeln einer einzelnen Verbindung braucht einen Server auf der anderen Seite (wie Speedify oder MPTCP); das macht Bondr nicht.

## Bedienung

`Bondr.exe` starten, die Oberfläche öffnet sich im Browser unter `http://127.0.0.1:8765`. Dort:

- Wege mit aktueller Rate ein- und ausschalten
- Windows-Proxy an oder aus
- Download-Adresse eingeben

Ist der Windows-Proxy an, trägt sich Bondr in den Autostart ein, damit der Browser nach einem Neustart weiter Internet hat. Beim Beenden schaltet Bondr den Proxy ab.

Einstellungen und Protokoll liegen in `%APPDATA%\Bondr`.

## Wie die Wege gebunden werden

Jede ausgehende Verbindung wird an die Adresse des Adapters gebunden und bekommt zusätzlich `IP_UNICAST_IF`, damit Windows sie wirklich über diesen Adapter schickt, auch wenn die Routentabelle einen anderen bevorzugt. Adapter werden alle 3 Sekunden neu erkannt, ein eingestecktes Handy taucht also von selbst auf.

## Bauen

```
CGO_ENABLED=0 GOOS=windows go build -trimpath -ldflags "-s -w -H windowsgui" -o Bondr.exe ./cmd/bondr
go test ./...
```

## Lizenz

MIT
