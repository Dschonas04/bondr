import SwiftUI
import AppKit
import ServiceManagement

@main
struct BondrApp: App {
    @NSApplicationDelegateAdaptor(Verwalter.self) var verwalter

    var body: some Scene {
        Window("Bondr", id: "haupt") {
            Hauptansicht().environmentObject(verwalter.kern)
                .frame(minWidth: 720, minHeight: 600)
        }
        .defaultSize(width: 860, height: 700)

        MenuBarExtra {
            Menue().environmentObject(verwalter.kern)
        } label: {
            Image(systemName: "arrow.triangle.merge")
        }
    }
}

final class Verwalter: NSObject, NSApplicationDelegate {
    @MainActor let kern = Kern()

    func applicationDidFinishLaunching(_ n: Notification) {
        MainActor.assumeIsolated {
            kern.starten()
            // Prüfmodus: Bondr --bild <datei.png> zeichnet das Fenster in eine Datei
            let a = CommandLine.arguments
            if let i = a.firstIndex(of: "--bild"), i + 1 < a.count { bildSpeichern(a[i + 1]) }
        }
    }

    @MainActor func bildSpeichern(_ pfad: String) {
        let ansicht = NSHostingView(rootView: Hauptansicht().environmentObject(kern).frame(width: 860, height: 700).background(Color(nsColor: .windowBackgroundColor)))
        ansicht.frame = NSRect(x: 0, y: 0, width: 860, height: 700)
        let f = NSWindow(contentRect: ansicht.frame, styleMask: [.titled], backing: .buffered, defer: false)
        f.contentView = ansicht
        f.setFrameOrigin(NSPoint(x: -3000, y: -3000))
        f.orderFrontRegardless()
        DispatchQueue.main.asyncAfter(deadline: .now() + 4) {
            if let rep = ansicht.bitmapImageRepForCachingDisplay(in: ansicht.bounds) {
                ansicht.cacheDisplay(in: ansicht.bounds, to: rep)
                try? rep.representation(using: .png, properties: [:])?.write(to: URL(fileURLWithPath: pfad))
            }
            NSApp.terminate(nil)
        }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ s: NSApplication) -> Bool { false }

    func applicationWillTerminate(_ n: Notification) {
        MainActor.assumeIsolated { kern.beenden() }
    }
}

struct Menue: View {
    @EnvironmentObject var kern: Kern
    @Environment(\.openWindow) var fensterOeffnen

    var body: some View {
        if let s = kern.stand {
            Text(String(format: "%.1f Mbit/s über %d %@", s.mbit_rein, s.wege.count, s.wege.count == 1 ? "Weg" : "Wege"))
            Toggle("Systemproxy", isOn: Binding(get: { s.systemproxy }, set: { kern.systemproxy($0) }))
        } else {
            Text(kern.meldung ?? "Startet …")
        }
        Divider()
        Button("Bondr öffnen") {
            fensterOeffnen(id: "haupt")
            NSApp.activate(ignoringOtherApps: true)
        }
        Button("Beenden") { NSApp.terminate(nil) }.keyboardShortcut("q")
    }
}

func zahl(_ x: Double, _ stellen: Int = 1) -> String {
    let f = NumberFormatter()
    f.locale = Locale(identifier: "de_DE")
    f.minimumFractionDigits = stellen
    f.maximumFractionDigits = stellen
    return f.string(from: NSNumber(value: x)) ?? "\(x)"
}

func groesse(_ b: Double) -> String {
    if b >= 1e9 { return zahl(b / 1e9, 2) + " GB" }
    if b >= 1e6 { return zahl(b / 1e6, 1) + " MB" }
    return zahl(b / 1e3, 0) + " KB"
}

struct Hauptansicht: View {
    @EnvironmentObject var kern: Kern
    @State private var adresse = ""
    @State private var anmeldung = SMAppService.mainApp.status == .enabled

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                kopf
                if let m = kern.meldung { Text(m).foregroundStyle(.orange) }
                if let s = kern.stand {
                    wege(s)
                    verteilung(s)
                    downloads(s)
                }
            }
            .padding(22)
        }
    }

    var kopf: some View {
        HStack(alignment: .firstTextBaseline) {
            VStack(alignment: .leading, spacing: 2) {
                Text("Bondr").font(.largeTitle.weight(.semibold))
                Text("Alle Verbindungen gleichzeitig nutzen, wie ein Lastverteiler").foregroundStyle(.secondary)
            }
            Spacer()
            Text((kern.stand.map { zahl($0.mbit_rein) } ?? "0,0") + " Mbit/s")
                .font(.system(size: 30, weight: .semibold)).monospacedDigit()
        }
    }

    func wege(_ s: Stand) -> some View {
        GroupBox("Wege") {
            VStack(spacing: 0) {
                if s.wege.isEmpty { Text("Kein Adapter mit Internet gefunden.").foregroundStyle(.secondary).padding() }
                ForEach(s.wege) { w in
                    WegZeile(weg: w)
                    if w.id != s.wege.last?.id { Divider() }
                }
            }
        }
    }

    func verteilung(_ s: Stand) -> some View {
        GroupBox("Verteilung") {
            VStack(alignment: .leading, spacing: 10) {
                Picker("Strategie", selection: Binding(get: { s.strategie }, set: { kern.strategie($0) })) {
                    Text("Wenigste Verbindungen").tag("wenigste")
                    Text("Gewichtet reihum").tag("reihum")
                    Text("Nur bei Ausfall").tag("ausfall")
                }
                .pickerStyle(.segmented)
                Text(erklaerung(s.strategie)).font(.callout).foregroundStyle(.secondary)
                Toggle(isOn: Binding(get: { s.systemproxy }, set: { kern.systemproxy($0) })) {
                    VStack(alignment: .leading) {
                        Text("Systemproxy verwenden").bold()
                        Text("Safari, Chrome und andere Programme verteilen ihre Verbindungen nach diesen Regeln. Adressen im LAN gehen direkt. Für eigene Programme: SOCKS5 oder HTTP \(s.proxy)")
                            .font(.callout).foregroundStyle(.secondary)
                    }
                }
                Toggle(s.gewuenscht ? "Bei Anmeldung starten (nötig, solange der Systemproxy an ist)" : "Bei Anmeldung starten", isOn: $anmeldung)
                    .disabled(s.gewuenscht)
                    .onAppear { anmeldung = SMAppService.mainApp.status == .enabled }
                    .onChange(of: anmeldung) { _, an in
                        do {
                            if an { try SMAppService.mainApp.register() } else { try SMAppService.mainApp.unregister() }
                        } catch {
                            kern.meldung = "Anmeldeobjekt: \(error.localizedDescription)"
                            anmeldung = SMAppService.mainApp.status == .enabled
                        }
                    }
            }
            .padding(6)
        }
    }

    func erklaerung(_ s: String) -> String {
        switch s {
        case "reihum": return "Neue Verbindungen gehen reihum, jeder Weg so oft, wie sein Gewicht sagt (wie nginx ohne Zusatz)."
        case "ausfall": return "Alles läuft über den Hauptweg mit dem höchsten Gewicht. Die anderen springen nur ein, wenn er ausfällt."
        default: return "Jede neue Verbindung geht an den Weg mit den wenigsten offenen Verbindungen im Verhältnis zu seinem Gewicht (wie nginx least_conn)."
        }
    }

    func downloads(_ s: Stand) -> some View {
        GroupBox("Datei laden") {
            VStack(alignment: .leading, spacing: 10) {
                Text("Eine Datei kommt in Abschnitten gleichzeitig über die Hauptwege; hier addiert sich die Bandbreite auch für einen einzelnen Download.")
                    .font(.callout).foregroundStyle(.secondary)
                HStack {
                    TextField("Download-Adresse, z. B. https://…", text: $adresse).onSubmit(starten)
                    Button("Laden", action: starten).keyboardShortcut(.defaultAction)
                }
                HStack(spacing: 6) {
                    Text("Ziel: \(s.ordner)").font(.callout).foregroundStyle(.secondary)
                    Button("ändern") { ordnerWaehlen(s.ordner) }.buttonStyle(.link)
                }
                ForEach(s.downloads) { d in DownloadZeile(d: d) }
            }
            .padding(6)
        }
    }

    func starten() {
        guard !adresse.isEmpty else { return }
        kern.laden(adresse)
        adresse = ""
    }

    func ordnerWaehlen(_ jetzt: String) {
        let p = NSOpenPanel()
        p.canChooseDirectories = true
        p.canChooseFiles = false
        p.directoryURL = URL(fileURLWithPath: jetzt)
        if p.runModal() == .OK, let u = p.url { kern.ordner(u.path) }
    }
}

struct WegZeile: View {
    @EnvironmentObject var kern: Kern
    let weg: Weg

    var symbol: String {
        switch weg.art {
        case "Kabel": return "cable.connector"
        case "WLAN": return "wifi"
        case "Handy": return "iphone"
        default: return "network"
        }
    }

    var zustand: (String, Color) {
        if weg.rolle == "aus" { return ("aus", .secondary) }
        if weg.gestoert { return ("gestört", .red) }
        if weg.benutzt { return ("aktiv", .green) }
        return ("bereit", .orange)
    }

    var body: some View {
        HStack(spacing: 14) {
            Image(systemName: symbol).font(.title2).frame(width: 30)
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Circle().fill(zustand.1).frame(width: 7, height: 7)
                    Text(weg.name).bold()
                    Text(zustand.0).font(.caption).foregroundStyle(.secondary)
                }
                Text("\(weg.art) · \(weg.ip) · \(weg.offen) offen · \(groesse(weg.mb_rein * 1e6)) empfangen")
                    .font(.caption).foregroundStyle(.secondary)
                if weg.gestoert, let f = weg.fehler { Text(f).font(.caption).foregroundStyle(.red).lineLimit(1) }
            }
            Spacer()
            VStack(alignment: .trailing) {
                Text(zahl(weg.mbit_rein) + " Mbit/s").monospacedDigit().bold()
                Text("↑ " + zahl(weg.mbit_raus) + " Mbit/s").font(.caption).monospacedDigit().foregroundStyle(.secondary)
            }
            .frame(width: 110, alignment: .trailing)
            Picker("", selection: Binding(get: { weg.rolle }, set: { kern.regel(weg, rolle: $0) })) {
                Text("Haupt").tag("haupt")
                Text("Reserve").tag("reserve")
                Text("Aus").tag("aus")
            }
            .labelsHidden().frame(width: 100)
            Stepper(value: Binding(get: { weg.gewicht }, set: { kern.regel(weg, gewicht: $0) }), in: 1...10) {
                Text("Gewicht \(weg.gewicht)").monospacedDigit().frame(width: 72, alignment: .trailing)
            }
            .disabled(weg.rolle == "aus")
        }
        .padding(.vertical, 8)
        .opacity(weg.rolle == "aus" ? 0.55 : 1)
    }
}

struct DownloadZeile: View {
    @EnvironmentObject var kern: Kern
    let d: Download

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(d.datei.isEmpty ? d.url : (d.datei as NSString).lastPathComponent).bold().lineLimit(1)
                Spacer()
                Text(text).font(.caption).monospacedDigit().foregroundStyle(.secondary)
            }
            if d.groesse > 0 { ProgressView(value: Double(d.geladen), total: Double(d.groesse)) }
            HStack {
                Text((d.wege ?? []).sorted { $0.bytes > $1.bytes }.map { a in
                    "\(a.name) \(zahl(d.geladen > 0 ? 100 * Double(a.bytes) / Double(d.geladen) : 0, 0)) %"
                }.joined(separator: ", ")).font(.caption).foregroundStyle(.secondary)
                Spacer()
                if d.status == "läuft" { Button("Abbrechen") { kern.abbrechen(d) }.buttonStyle(.link) }
                if d.status == "fertig" {
                    Button("Im Finder zeigen") { NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: d.datei)]) }.buttonStyle(.link)
                }
            }
            if let f = d.fehler, !f.isEmpty { Text(f).font(.caption).foregroundStyle(.red) }
        }
        .padding(.vertical, 4)
    }

    var text: String {
        var t = groesse(Double(d.geladen))
        if d.groesse > 0 { t += " von " + groesse(Double(d.groesse)) }
        t += " · \(zahl(d.mbit, 0)) Mbit/s"
        if d.status != "läuft" { t += " · \(d.status)" }
        return t
    }
}
