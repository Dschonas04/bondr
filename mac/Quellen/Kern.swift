import Foundation
import AppKit
import ServiceManagement

// Was der Kern (bondrd) über /stand meldet.
struct Stand: Decodable {
    var version: String
    var strategie: String
    var systemproxy: Bool
    var gewuenscht: Bool
    var proxy: String
    var ordner: String
    var mbit_rein: Double
    var mbit_raus: Double
    var wege: [Weg]
    var downloads: [Download]
}

struct Weg: Decodable, Identifiable {
    var schluessel: String
    var name: String
    var art: String
    var ip: String
    var gateway: String?
    var gewicht: Int
    var rolle: String
    var benutzt: Bool
    var gestoert: Bool
    var fehler: String?
    var offen: Int
    var mbit_rein: Double
    var mbit_raus: Double
    var mb_rein: Double
    var id: String { schluessel }
}

struct Anteil: Decodable, Hashable {
    var name: String
    var bytes: Int64
}

struct Download: Decodable, Identifiable {
    var id: Int
    var url: String
    var datei: String
    var groesse: Int64
    var geladen: Int64
    var status: String
    var fehler: String?
    var wege: [Anteil]?
    var mbit: Double
}

/// Startet bondrd aus dem App-Paket und spricht mit ihm über 127.0.0.1.
@MainActor
final class Kern: ObservableObject {
    @Published var stand: Stand?
    @Published var meldung: String?

    static let port = 47801
    private var prozess: Process?
    private var takt: Timer?
    private var schluessel: String?

    private var schluesselDatei: URL {
        FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("Bondr/api-schluessel")
    }

    func starten() {
        if let pfad = Bundle.main.url(forAuxiliaryExecutable: "bondrd") {
            let p = Process()
            p.executableURL = pfad
            p.arguments = ["--eltern", String(ProcessInfo.processInfo.processIdentifier)]
            p.standardOutput = FileHandle.nullDevice
            p.standardError = FileHandle.nullDevice
            do { try p.run(); prozess = p } catch { meldung = "Kern startet nicht: \(error.localizedDescription)" }
        } else {
            meldung = "bondrd fehlt im App-Paket"
        }
        takt = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.holen() }
        }
    }

    func beenden() {
        let sem = DispatchSemaphore(value: 0)
        anfrage("/beenden", [:]) { sem.signal() }
        _ = sem.wait(timeout: .now() + 1.5)
        if let p = prozess, p.isRunning { p.terminate() }
    }

    private func anfrageBauen(_ pfad: String) -> URLRequest? {
        if schluessel == nil {
            schluessel = (try? String(contentsOf: schluesselDatei, encoding: .utf8))?.trimmingCharacters(in: .whitespacesAndNewlines)
        }
        guard let s = schluessel, let url = URL(string: "http://127.0.0.1:\(Kern.port)\(pfad)") else { return nil }
        var r = URLRequest(url: url, timeoutInterval: 3)
        r.setValue(s, forHTTPHeaderField: "X-Bondr")
        return r
    }

    func holen() async {
        guard let r = anfrageBauen("/stand") else { return }
        do {
            let (daten, antwort) = try await URLSession.shared.data(for: r)
            if (antwort as? HTTPURLResponse)?.statusCode == 403 { schluessel = nil; return }
            let neu = try JSONDecoder().decode(Stand.self, from: daten)
            stand = neu
            meldung = nil
            // Ist der Systemproxy gewünscht, muss Bondr nach einem Neustart laufen, sonst
            // hätten Safari und Co. kein Internet: Anmeldeobjekt dann immer an.
            if neu.gewuenscht && SMAppService.mainApp.status != .enabled {
                try? SMAppService.mainApp.register()
            }
        } catch {
            if stand == nil { meldung = "Warte auf den Kern …" }
        }
    }

    func anfrage(_ pfad: String, _ inhalt: [String: Any], fertig: (() -> Void)? = nil) {
        guard var r = anfrageBauen(pfad) else { fertig?(); return }
        r.httpMethod = "POST"
        r.setValue("application/json", forHTTPHeaderField: "Content-Type")
        r.httpBody = try? JSONSerialization.data(withJSONObject: inhalt)
        URLSession.shared.dataTask(with: r) { daten, antwort, _ in
            let code = (antwort as? HTTPURLResponse)?.statusCode ?? 0
            let text = daten.flatMap { String(data: $0, encoding: .utf8) }?.trimmingCharacters(in: .whitespacesAndNewlines)
            Task { @MainActor in
                if code >= 400 { self.meldung = text ?? "Fehler \(code)" }
                await self.holen()
                fertig?()
            }
        }.resume()
    }

    func regel(_ w: Weg, gewicht: Int? = nil, rolle: String? = nil) {
        anfrage("/regel", ["name": w.name, "gewicht": gewicht ?? w.gewicht, "rolle": rolle ?? w.rolle])
    }
    func strategie(_ s: String) { anfrage("/strategie", ["strategie": s]) }
    func systemproxy(_ an: Bool) { anfrage("/systemproxy", ["an": an]) }
    func laden(_ url: String) { anfrage("/laden", ["url": url]) }
    func abbrechen(_ d: Download) { anfrage("/abbrechen", ["id": d.id]) }
    func ordner(_ o: String) { anfrage("/ordner", ["ordner": o]) }
}
