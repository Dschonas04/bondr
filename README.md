# Bondr

Bondr nutzt alle Internetzugänge eines Windows-Rechners gleichzeitig: Kabel, WLAN und das Handy per USB-Tethering. Ein Windows-Programm mit eigenem Fenster und Symbol im Infobereich, ohne Installation und ohne Zusatzsoftware (.NET Framework 4.8 ist in Windows 10 und 11 enthalten).

## Was es macht

**Datei laden.** Eine Datei wird in Abschnitte geteilt, die gleichzeitig über alle Wege kommen; hier addiert sich die Bandbreite auch für einen einzelnen Download. Je Weg laufen drei lange Verbindungen. Wird eine frei, übernimmt sie die hintere Hälfte des größten offenen Abschnitts, so bremst der langsamere Weg am Ende nicht. Antwortet der Server mit 429 oder 503, nimmt Bondr Verbindungen zurück.

**Programme verteilen.** Bondr ist ein lokaler Proxy (SOCKS5 und HTTP auf `127.0.0.1:1080`). Mit „Windows-Proxy verwenden“ benutzen ihn Browser und alle Programme, die den Windows-Proxy beachten. Jede neue Verbindung geht über den Weg, der gerade am wenigsten zu tun hat. Eine einzelne Verbindung wird dadurch nicht schneller, viele gleichzeitige schon. Adressen im LAN (10.x, 192.168.x, .local, .fritz.box) gehen direkt.

Echtes Bündeln einer einzelnen Verbindung braucht einen Server auf der anderen Seite (wie Speedify oder MPTCP); das macht Bondr nicht.

## Bedienung

- Fenster schließen legt Bondr in den Infobereich, beenden über „Beenden“ oder das Symbol.
- Haken bei einem Weg entfernen, um ihn nicht zu benutzen.
- Ist der Windows-Proxy an, trägt sich Bondr in den Autostart ein (startet dann still im Infobereich), damit der Browser nach einem Neustart weiter Internet hat. Beim Beenden schaltet Bondr den Proxy ab.
- Einstellungen unter `HKCU\Software\Bondr`, Protokoll in `%APPDATA%\Bondr\bondr.log`.

Für Messungen ohne Fenster: `Bondr.exe --messen <URL> <Ordner>` schreibt `messung.txt` in den Ordner.

## Wie die Wege gebunden werden

Jede ausgehende Verbindung wird an die Adresse des Adapters gebunden und bekommt `IP_UNICAST_IF`, damit Windows sie wirklich über diesen Adapter schickt. Downloads laufen über einen eigenen kleinen HTTP/1.1-Client, weil die eingebauten Klassen Verbindungen je Server teilen und sich nicht pro Anfrage an einen Adapter binden lassen. Adapter werden alle 3 Sekunden neu erkannt.

## Bauen

Unter Windows, ohne Visual Studio, mit dem C#-Compiler aus dem .NET Framework:

```
bauen.cmd
```

Ergebnis: `dist\Bondr.exe`.

## Lizenz

MIT
