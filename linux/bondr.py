#!/usr/bin/env python3
# Bondr für Linux: GTK4/libadwaita-Oberfläche. Startet den Kern bondrd und steuert ihn
# über 127.0.0.1:47801 mit dem Schlüssel aus ~/.config/Bondr/api-schluessel.

import json
import os
import shutil
import subprocess
import sys
import threading
import urllib.error
import urllib.request

import gi

gi.require_version("Gtk", "4.0")
gi.require_version("Adw", "1")
from gi.repository import Adw, Gdk, Gio, GLib, Gtk  # noqa: E402

APP_ID = "io.github.dschonas04.bondr"
PORT = 47801
ORDNER = os.path.join(GLib.get_user_config_dir(), "Bondr")
SCHLUESSEL = os.path.join(ORDNER, "api-schluessel")
AUTOSTART = os.path.join(GLib.get_user_config_dir(), "autostart", APP_ID + ".desktop")

ROLLEN = [("haupt", "Haupt"), ("reserve", "Reserve"), ("aus", "Aus")]
STRATEGIEN = [
    ("wenigste", "Wenigste Verbindungen",
     "Jede neue Verbindung geht an den Weg mit den wenigsten offenen Verbindungen im Verhältnis zu seinem Gewicht (wie nginx least_conn)."),
    ("reihum", "Gewichtet reihum",
     "Neue Verbindungen gehen reihum, jeder Weg so oft, wie sein Gewicht sagt (wie nginx ohne Zusatz)."),
    ("ausfall", "Nur bei Ausfall",
     "Alles läuft über den Hauptweg mit dem höchsten Gewicht. Die anderen springen nur ein, wenn er ausfällt."),
]
SYMBOLE = {"Kabel": "network-wired-symbolic", "WLAN": "network-wireless-symbolic",
           "Handy": "phone-symbolic"}

CSS = """
.gross { font-size: 30px; font-weight: 600; font-feature-settings: "tnum"; }
.titel { font-size: 26px; font-weight: 600; }
.zahl { font-feature-settings: "tnum"; }
.punkt { min-width: 8px; min-height: 8px; border-radius: 4px; }
.punkt.aktiv { background: @success_color; }
.punkt.bereit { background: @warning_color; }
.punkt.gestoert { background: @error_color; }
.punkt.aus { background: alpha(currentColor, 0.35); }
"""


def zahl(x, stellen=1):
    return f"{x:,.{stellen}f}".replace(",", " ").replace(".", ",").replace(" ", ".")


def groesse(b):
    if b >= 1e9:
        return zahl(b / 1e9, 2) + " GB"
    if b >= 1e6:
        return zahl(b / 1e6, 1) + " MB"
    return zahl(b / 1e3, 0) + " KB"


def kern_pfad():
    for p in (os.environ.get("BONDR_KERN"),
              os.path.join(os.path.dirname(os.path.realpath(__file__)), "bondrd"),
              "/usr/lib/bondr/bondrd", "/opt/bondr/bondrd"):
        if p and os.access(p, os.X_OK):
            return p
    return shutil.which("bondrd")


class Kern:
    """Spricht mit bondrd. Anfragen laufen in Threads, Antworten kommen im GTK-Thread an."""

    def __init__(self):
        self.prozess = None
        self.schluessel = None

    def starten(self):
        pfad = kern_pfad()
        if not pfad:
            return "bondrd nicht gefunden"
        try:
            self.prozess = subprocess.Popen([pfad, "--eltern", str(os.getpid())],
                                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                            start_new_session=True)
        except OSError as e:
            return f"Kern startet nicht: {e}"
        return None

    def _anfrage(self, pfad, inhalt=None):
        if self.schluessel is None:
            try:
                with open(SCHLUESSEL, encoding="utf-8") as f:
                    self.schluessel = f.read().strip()
            except OSError:
                raise ConnectionError("Warte auf den Kern …")
        daten = None if inhalt is None else json.dumps(inhalt).encode()
        r = urllib.request.Request(f"http://127.0.0.1:{PORT}{pfad}", data=daten,
                                   method="GET" if inhalt is None else "POST",
                                   headers={"X-Bondr": self.schluessel, "Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(r, timeout=3) as a:
                text = a.read()
        except urllib.error.HTTPError as e:
            if e.code == 403:
                self.schluessel = None
            raise RuntimeError(e.read().decode("utf-8", "replace").strip() or f"Fehler {e.code}")
        except (urllib.error.URLError, OSError):
            raise ConnectionError("Warte auf den Kern …")
        return json.loads(text) if text else None

    def im_hintergrund(self, pfad, inhalt, fertig):
        def lauf():
            try:
                erg, fehler = self._anfrage(pfad, inhalt), None
            except Exception as e:  # noqa: BLE001
                erg, fehler = None, e
            GLib.idle_add(fertig, erg, fehler)
        threading.Thread(target=lauf, daemon=True).start()

    def beenden(self):
        try:
            self._anfrage("/beenden", {})
        except Exception:  # noqa: BLE001
            pass
        if self.prozess and self.prozess.poll() is None:
            try:
                self.prozess.wait(1.5)
            except subprocess.TimeoutExpired:
                self.prozess.terminate()


def autostart_an():
    return os.path.exists(AUTOSTART)


def autostart_setzen(an):
    if an:
        os.makedirs(os.path.dirname(AUTOSTART), exist_ok=True)
        befehl = shutil.which("bondr") or os.path.realpath(sys.argv[0])
        with open(AUTOSTART, "w", encoding="utf-8") as f:
            f.write("[Desktop Entry]\nType=Application\nName=Bondr\n"
                    f"Exec={befehl} --hintergrund\nIcon={APP_ID}\n"
                    "X-GNOME-Autostart-enabled=true\nNoDisplay=true\n")
    elif os.path.exists(AUTOSTART):
        os.remove(AUTOSTART)


class WegZeile(Adw.ActionRow):
    def __init__(self, fenster, weg):
        super().__init__()
        self.fenster = fenster
        self.weg = weg
        self.sperre = False
        self.symbol = Gtk.Image(icon_size=Gtk.IconSize.LARGE)
        self.add_prefix(self.symbol)
        self.punkt = Gtk.Box(valign=Gtk.Align.CENTER, css_classes=["punkt"])
        self.add_prefix(self.punkt)

        rechts = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, valign=Gtk.Align.CENTER, width_request=110)
        self.rein = Gtk.Label(xalign=1, css_classes=["zahl", "heading"])
        self.raus = Gtk.Label(xalign=1, css_classes=["zahl", "caption", "dim-label"])
        rechts.append(self.rein)
        rechts.append(self.raus)
        self.add_suffix(rechts)

        self.rolle = Gtk.DropDown.new_from_strings([t for _, t in ROLLEN])
        self.rolle.set_valign(Gtk.Align.CENTER)
        self.rolle.connect("notify::selected", self.rolle_geaendert)
        self.add_suffix(self.rolle)

        box = Gtk.Box(spacing=6, valign=Gtk.Align.CENTER)
        box.append(Gtk.Label(label="Gewicht", css_classes=["dim-label"]))
        self.gewicht = Gtk.SpinButton.new_with_range(1, 10, 1)
        self.gewicht.connect("value-changed", self.gewicht_geaendert)
        box.append(self.gewicht)
        self.add_suffix(box)
        self.aktualisieren(weg)

    def aktualisieren(self, w):
        self.weg = w
        self.sperre = True
        self.symbol.set_from_icon_name(SYMBOLE.get(w["art"], "network-workgroup-symbolic"))
        if w["rolle"] == "aus":
            zustand = ("aus", "aus")
        elif w["gestoert"]:
            zustand = ("gestört", "gestoert")
        elif w["benutzt"]:
            zustand = ("aktiv", "aktiv")
        else:
            zustand = ("bereit", "bereit")
        self.set_title(GLib.markup_escape_text(f"{w['name']}  ·  {zustand[0]}"))
        unter = f"{w['art']} · {w.get('beschreibung') or ''} · {w['ip']} · {w['offen']} offen · {groesse(w['mb_rein'] * 1e6)} empfangen"
        if w["gestoert"] and w.get("fehler"):
            unter += "\n" + w["fehler"]
        self.set_subtitle(GLib.markup_escape_text(unter.replace(" ·  · ", " · ")))
        for k in ("aktiv", "bereit", "gestoert", "aus"):
            self.punkt.remove_css_class(k)
        self.punkt.add_css_class(zustand[1])
        self.rein.set_label(zahl(w["mbit_rein"]) + " Mbit/s")
        self.raus.set_label("↑ " + zahl(w["mbit_raus"]) + " Mbit/s")
        i = [k for k, _ in ROLLEN].index(w["rolle"]) if w["rolle"] in dict(ROLLEN) else 0
        if self.rolle.get_selected() != i:
            self.rolle.set_selected(i)
        if not self.gewicht.has_focus() and int(self.gewicht.get_value()) != w["gewicht"]:
            self.gewicht.set_value(w["gewicht"])
        self.gewicht.set_sensitive(w["rolle"] != "aus")
        self.set_opacity(0.55 if w["rolle"] == "aus" else 1)
        self.sperre = False

    def rolle_geaendert(self, *_):
        if not self.sperre:
            self.fenster.regel(self.weg, rolle=ROLLEN[self.rolle.get_selected()][0])

    def gewicht_geaendert(self, *_):
        if not self.sperre:
            self.fenster.regel(self.weg, gewicht=int(self.gewicht.get_value()))


class DownloadZeile(Gtk.Box):
    def __init__(self, fenster, d):
        super().__init__(orientation=Gtk.Orientation.VERTICAL, spacing=4,
                         margin_top=8, margin_bottom=8, margin_start=12, margin_end=12)
        self.fenster = fenster
        oben = Gtk.Box(spacing=8)
        self.name = Gtk.Label(xalign=0, hexpand=True, ellipsize=3, css_classes=["heading"])
        self.text = Gtk.Label(xalign=1, css_classes=["zahl", "caption", "dim-label"])
        oben.append(self.name)
        oben.append(self.text)
        self.append(oben)
        self.balken = Gtk.ProgressBar()
        self.append(self.balken)
        unten = Gtk.Box(spacing=8)
        self.anteile = Gtk.Label(xalign=0, hexpand=True, css_classes=["caption", "dim-label"])
        self.knopf = Gtk.Button(css_classes=["flat"])
        self.knopf.connect("clicked", self.geklickt)
        unten.append(self.anteile)
        unten.append(self.knopf)
        self.append(unten)
        self.fehler = Gtk.Label(xalign=0, wrap=True, css_classes=["caption", "error"])
        self.append(self.fehler)
        self.aktualisieren(d)

    def aktualisieren(self, d):
        self.d = d
        self.name.set_label(os.path.basename(d["datei"]) if d["datei"] else d["url"])
        t = groesse(d["geladen"])
        if d["groesse"] > 0:
            t += " von " + groesse(d["groesse"])
        t += f" · {zahl(d['mbit'], 0)} Mbit/s"
        if d["status"] != "läuft":
            t += f" · {d['status']}"
        self.text.set_label(t)
        self.balken.set_visible(d["groesse"] > 0)
        if d["groesse"] > 0:
            self.balken.set_fraction(min(1, d["geladen"] / d["groesse"]))
        geladen = d["geladen"] or 0
        self.anteile.set_label(", ".join(
            f"{a['name']} {zahl(100 * a['bytes'] / geladen if geladen else 0, 0)} %"
            for a in sorted(d.get("wege") or [], key=lambda a: -a["bytes"])))
        if d["status"] == "läuft":
            self.knopf.set_label("Abbrechen")
            self.knopf.set_visible(True)
        elif d["status"] == "fertig":
            self.knopf.set_label("Im Ordner zeigen")
            self.knopf.set_visible(True)
        else:
            self.knopf.set_visible(False)
        self.fehler.set_label(d.get("fehler") or "")
        self.fehler.set_visible(bool(d.get("fehler")))

    def geklickt(self, *_):
        if self.d["status"] == "läuft":
            self.fenster.senden("/abbrechen", {"id": self.d["id"]})
        else:
            datei = Gio.File.new_for_path(self.d["datei"])
            try:
                Gio.DBusProxy.new_for_bus_sync(
                    Gio.BusType.SESSION, Gio.DBusProxyFlags.NONE, None,
                    "org.freedesktop.FileManager1", "/org/freedesktop/FileManager1",
                    "org.freedesktop.FileManager1", None,
                ).call_sync("ShowItems", GLib.Variant("(ass)", ([datei.get_uri()], "")),
                            Gio.DBusCallFlags.NONE, 2000, None)
            except GLib.Error:
                Gio.AppInfo.launch_default_for_uri(datei.get_parent().get_uri(), None)


class Fenster(Adw.ApplicationWindow):
    def __init__(self, app):
        super().__init__(application=app, title="Bondr", default_width=900, default_height=720)
        self.app = app
        self.stand = None
        self.zeilen = {}
        self.dl_zeilen = {}
        self.sperre = False

        kopf = Adw.HeaderBar()
        menue = Gio.Menu()
        menue.append("Über Bondr", "app.ueber")
        menue.append("Beenden", "app.beenden")
        kopf.pack_end(Gtk.MenuButton(icon_name="open-menu-symbolic", menu_model=menue))

        inhalt = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=24,
                         margin_top=18, margin_bottom=24, margin_start=18, margin_end=18)
        clamp = Adw.Clamp(maximum_size=900, child=inhalt)
        rollen = Gtk.ScrolledWindow(vexpand=True, child=clamp)
        aussen = Gtk.Box(orientation=Gtk.Orientation.VERTICAL)
        aussen.append(kopf)
        self.banner = Gtk.Label(wrap=True, xalign=0, css_classes=["warning"], visible=False,
                                margin_start=18, margin_end=18, margin_top=6)
        aussen.append(self.banner)
        aussen.append(rollen)
        self.set_content(aussen)

        # Kopf
        zeile = Gtk.Box(spacing=12)
        links = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, hexpand=True)
        links.append(Gtk.Label(label="Bondr", xalign=0, css_classes=["titel"]))
        links.append(Gtk.Label(label="Alle Verbindungen gleichzeitig nutzen, wie ein Lastverteiler",
                               xalign=0, css_classes=["dim-label"]))
        zeile.append(links)
        self.gesamt = Gtk.Label(label="0,0 Mbit/s", css_classes=["gross"], valign=Gtk.Align.END)
        zeile.append(self.gesamt)
        inhalt.append(zeile)

        # Wege
        self.wege = Adw.PreferencesGroup(title="Wege")
        self.leer = Adw.ActionRow(title="Kein Adapter mit Internet gefunden.")
        self.wege.add(self.leer)
        inhalt.append(self.wege)

        # Verteilung
        vg = Adw.PreferencesGroup(title="Verteilung")
        self.strategie = Adw.ComboRow(title="Strategie", model=Gtk.StringList.new([t for _, t, _ in STRATEGIEN]))
        self.strategie.connect("notify::selected", self.strategie_geaendert)
        vg.add(self.strategie)
        self.proxy = Adw.ActionRow(title="Systemproxy verwenden")
        self.proxy_schalter = Gtk.Switch(valign=Gtk.Align.CENTER)
        self.proxy_schalter.connect("notify::active", self.proxy_geaendert)
        self.proxy.add_suffix(self.proxy_schalter)
        self.proxy.set_activatable_widget(self.proxy_schalter)
        vg.add(self.proxy)
        self.anmeldung = Adw.ActionRow(title="Bei Anmeldung starten")
        self.anmeldung_schalter = Gtk.Switch(valign=Gtk.Align.CENTER, active=autostart_an())
        self.anmeldung_schalter.connect("notify::active", self.anmeldung_geaendert)
        self.anmeldung.add_suffix(self.anmeldung_schalter)
        self.anmeldung.set_activatable_widget(self.anmeldung_schalter)
        vg.add(self.anmeldung)
        inhalt.append(vg)

        # Downloads
        dg = Adw.PreferencesGroup(
            title="Datei laden",
            description="Eine Datei kommt in Abschnitten gleichzeitig über die Hauptwege; hier addiert sich die Bandbreite auch für einen einzelnen Download.")
        self.adresse = Adw.EntryRow(title="Download-Adresse, z. B. https://…", show_apply_button=False)
        self.adresse.connect("entry-activated", self.laden)
        knopf = Gtk.Button(label="Laden", valign=Gtk.Align.CENTER, css_classes=["suggested-action"])
        knopf.connect("clicked", self.laden)
        self.adresse.add_suffix(knopf)
        dg.add(self.adresse)
        self.ziel = Adw.ActionRow(title="Ziel")
        aendern = Gtk.Button(label="Ändern", valign=Gtk.Align.CENTER, css_classes=["flat"])
        aendern.connect("clicked", self.ordner_waehlen)
        self.ziel.add_suffix(aendern)
        dg.add(self.ziel)
        self.dl_liste = Gtk.ListBox(selection_mode=Gtk.SelectionMode.NONE, css_classes=["boxed-list"], visible=False)
        dg.add(self.dl_liste)
        inhalt.append(dg)

        self.connect("close-request", self.schliessen)

    def schliessen(self, *_):
        # Bondr läuft weiter (der Proxy braucht den Kern); über das Menü „Beenden" endet es
        self.set_visible(False)
        return True

    def meldung(self, text):
        self.banner.set_label(text or "")
        self.banner.set_visible(bool(text))

    def zeigen(self, s):
        self.stand = s
        self.sperre = True
        self.gesamt.set_label(zahl(s["mbit_rein"]) + " Mbit/s")

        # Wege: Zeilen behalten, nur neue anlegen und verschwundene entfernen
        neu = {w["schluessel"]: w for w in s["wege"]}
        for k in list(self.zeilen):
            if k not in neu:
                self.wege.remove(self.zeilen.pop(k))
        for k, w in neu.items():
            if k in self.zeilen:
                self.zeilen[k].aktualisieren(w)
            else:
                self.zeilen[k] = WegZeile(self, w)
                self.wege.add(self.zeilen[k])
        self.leer.set_visible(not neu)

        keys = [k for k, _, _ in STRATEGIEN]
        i = keys.index(s["strategie"]) if s["strategie"] in keys else 0
        if self.strategie.get_selected() != i:
            self.strategie.set_selected(i)
        self.strategie.set_subtitle(STRATEGIEN[i][2])
        if self.proxy_schalter.get_active() != s["systemproxy"]:
            self.proxy_schalter.set_active(s["systemproxy"])
        self.proxy.set_subtitle("Firefox, Chrome und andere Programme, die den GNOME- oder KDE-Proxy beachten, "
                                f"verteilen ihre Verbindungen nach diesen Regeln. Adressen im LAN gehen direkt. "
                                f"Für eigene Programme: SOCKS5 oder HTTP {s['proxy']}")
        # Ist der Systemproxy gewünscht, muss Bondr nach einer Anmeldung laufen
        if s["gewuenscht"] and not autostart_an():
            autostart_setzen(True)
        self.anmeldung.set_title("Bei Anmeldung starten (nötig, solange der Systemproxy an ist)"
                                 if s["gewuenscht"] else "Bei Anmeldung starten")
        self.anmeldung_schalter.set_active(autostart_an())
        self.anmeldung_schalter.set_sensitive(not s["gewuenscht"])
        self.ziel.set_subtitle(GLib.markup_escape_text(s["ordner"]))

        ids = {d["id"]: d for d in s["downloads"]}
        for k in list(self.dl_zeilen):
            if k not in ids:
                self.dl_liste.remove(self.dl_zeilen.pop(k).get_parent())
        for k, d in sorted(ids.items()):
            if k in self.dl_zeilen:
                self.dl_zeilen[k].aktualisieren(d)
            else:
                self.dl_zeilen[k] = DownloadZeile(self, d)
                self.dl_liste.append(self.dl_zeilen[k])
        self.dl_liste.set_visible(bool(ids))
        self.sperre = False

    def senden(self, pfad, inhalt):
        def fertig(_, fehler):
            if isinstance(fehler, RuntimeError):
                self.meldung(str(fehler))
            self.app.holen()
        self.app.kern.im_hintergrund(pfad, inhalt, fertig)

    def regel(self, w, gewicht=None, rolle=None):
        self.senden("/regel", {"name": w["name"], "gewicht": gewicht or w["gewicht"], "rolle": rolle or w["rolle"]})

    def strategie_geaendert(self, *_):
        if not self.sperre:
            self.senden("/strategie", {"strategie": STRATEGIEN[self.strategie.get_selected()][0]})

    def proxy_geaendert(self, *_):
        if not self.sperre:
            self.senden("/systemproxy", {"an": self.proxy_schalter.get_active()})

    def anmeldung_geaendert(self, *_):
        if self.sperre:
            return
        try:
            autostart_setzen(self.anmeldung_schalter.get_active())
        except OSError as e:
            self.meldung(f"Autostart: {e}")

    def laden(self, *_):
        url = self.adresse.get_text().strip()
        if url:
            self.senden("/laden", {"url": url})
            self.adresse.set_text("")

    def ordner_waehlen(self, *_):
        jetzt = Gio.File.new_for_path(self.stand["ordner"] if self.stand else GLib.get_home_dir())
        if hasattr(Gtk, "FileDialog"):
            d = Gtk.FileDialog(title="Zielordner", initial_folder=jetzt)

            def fertig(dlg, erg):
                try:
                    f = dlg.select_folder_finish(erg)
                except GLib.Error:
                    return
                if f and f.get_path():
                    self.senden("/ordner", {"ordner": f.get_path()})
            d.select_folder(self, None, fertig)
        else:
            d = Gtk.FileChooserNative(title="Zielordner", transient_for=self,
                                      action=Gtk.FileChooserAction.SELECT_FOLDER)
            d.set_current_folder(jetzt)

            def antwort(dlg, r):
                if r == Gtk.ResponseType.ACCEPT and dlg.get_file():
                    self.senden("/ordner", {"ordner": dlg.get_file().get_path()})
            d.connect("response", antwort)
            d.show()
            self._waehler = d


class App(Adw.Application):
    def __init__(self):
        super().__init__(application_id=APP_ID, flags=Gio.ApplicationFlags.HANDLES_COMMAND_LINE)
        self.kern = Kern()
        self.fenster = None
        self.version = ""
        self.bild = None
        self.hintergrund = False
        for name, f in (("beenden", lambda *_: self.quit()), ("ueber", self.ueber)):
            a = Gio.SimpleAction.new(name, None)
            a.connect("activate", f)
            self.add_action(a)
        self.set_accels_for_action("app.beenden", ["<Ctrl>q"])

    def do_startup(self):
        Adw.Application.do_startup(self)
        css = Gtk.CssProvider()
        css.load_from_data(CSS.encode())
        Gtk.StyleContext.add_provider_for_display(Gdk.Display.get_default(), css,
                                                  Gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
        self.fenster = Fenster(self)
        fehler = self.kern.starten()
        if fehler:
            self.fenster.meldung(fehler)
        self.hold()  # läuft ohne Fenster weiter, bis „Beenden"
        GLib.timeout_add_seconds(1, self.holen)
        self.holen()

    def do_command_line(self, befehle):
        a = befehle.get_arguments()
        if "--bild" in a and a.index("--bild") + 1 < len(a):
            self.bild = a[a.index("--bild") + 1]
            self.fenster.present()
            GLib.timeout_add_seconds(4, self.bild_speichern)
        elif "--hintergrund" not in a:  # Autostart: nur der Kern, Fenster bleibt zu
            self.fenster.present()
        return 0

    def holen(self):
        def fertig(s, fehler):
            if s:
                self.version = s.get("version", "")
                self.fenster.zeigen(s)
                if self.fenster.banner.get_label() == "Warte auf den Kern …":
                    self.fenster.meldung(None)
            elif fehler and self.fenster.stand is None:
                self.fenster.meldung(str(fehler))
        self.kern.im_hintergrund("/stand", None, fertig)
        return True

    def ueber(self, *_):
        Adw.AboutWindow(transient_for=self.fenster, application_name="Bondr", application_icon=APP_ID,
                        version=self.version, developer_name="Jonas Groll", license_type=Gtk.License.MIT_X11,
                        website="https://github.com/Dschonas04/bondr",
                        comments="Alle Internetzugänge eines Rechners gleichzeitig.").present()

    def bild_speichern(self):
        f = self.fenster
        p = Gtk.WidgetPaintable.new(f)
        b, h = f.get_width(), f.get_height()
        s = Gtk.Snapshot()
        p.snapshot(s, b, h)
        knoten = s.to_node()
        if knoten:
            f.get_renderer().render_texture(knoten, None).save_to_png(self.bild)
        self.quit()
        return False

    def do_shutdown(self):
        self.kern.beenden()
        Adw.Application.do_shutdown(self)


if __name__ == "__main__":
    sys.exit(App().run(sys.argv))
