package io.github.dschonas04.bondr

import android.content.Context
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import org.json.JSONObject

/**
 * Startet den Go-Kern (libbondrd.so aus dem Bibliotheksordner) und spricht mit ihm über
 * 127.0.0.1:47801. Der Schlüssel liegt in einer nur für die App lesbaren Datei.
 */
class Kern(private val context: Context) {
    private var prozess: Process? = null
    private var schluessel: String? = null

    private val ordner get() = File(context.filesDir, ".config/Bondr")

    fun starten() {
        if (prozess?.isAlive == true) return
        val programm = File(context.applicationInfo.nativeLibraryDir, "libbondrd.so")
        File(ordner, "api-schluessel").delete()
        schluessel = null
        val pb = ProcessBuilder(programm.path, "--tunnel", "--eltern", android.os.Process.myPid().toString())
            .redirectErrorStream(true)
            .redirectOutput(File(context.filesDir, "bondrd-ausgabe.txt"))
        pb.environment()["HOME"] = context.filesDir.path
        pb.environment()["XDG_CONFIG_HOME"] = File(context.filesDir, ".config").path
        if (File(context.filesDir, "protokoll").exists()) pb.environment()["BONDR_PROTOKOLL"] = "1"
        prozess = pb.start()
    }

    fun laeuft(): Boolean = prozess?.isAlive == true

    fun beenden() {
        runCatching { anfrage("POST", "/beenden", "{}") }
        prozess?.let { p ->
            repeat(15) { if (p.isAlive) Thread.sleep(100) }
            if (p.isAlive) p.destroy()
        }
        prozess = null
    }

    /** Antwort als Text; wirft bei Fehlern mit der Meldung des Kerns. */
    fun anfrage(methode: String, pfad: String, inhalt: String? = null): String {
        val s = schluessel ?: File(ordner, "api-schluessel").takeIf { it.exists() }?.readText()?.trim()
            ?: throw IllegalStateException("Kern startet …")
        schluessel = s
        val c = URL("http://127.0.0.1:47801$pfad").openConnection() as HttpURLConnection
        c.requestMethod = methode
        c.connectTimeout = 2000
        c.readTimeout = 4000
        c.setRequestProperty("X-Bondr", s)
        if (inhalt != null) {
            c.doOutput = true
            c.setRequestProperty("Content-Type", "application/json")
            c.outputStream.use { it.write(inhalt.toByteArray()) }
        }
        val code = c.responseCode
        val text = (if (code >= 400) c.errorStream else c.inputStream)?.bufferedReader()?.use { it.readText() } ?: ""
        if (code == 403) schluessel = null
        if (code >= 400) throw IllegalStateException(text.trim().ifEmpty { "Fehler $code" })
        return text
    }

    fun stand(): JSONObject = JSONObject(anfrage("GET", "/stand"))

    /** Verbrauch aus der Datei, auch wenn der Kern nicht läuft. */
    fun verbrauchGespeichert(): JSONObject =
        runCatching { JSONObject(File(ordner, "verbrauch.json").readText()) }.getOrDefault(JSONObject())
}
